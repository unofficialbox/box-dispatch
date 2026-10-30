import { expect, test, type Page, type APIRequestContext } from '@playwright/test'
import type { ConnectionSummary, DeploymentPlan, DispatchRun } from '../src/types'

async function fixtures(page: Page, request: APIRequestContext) {
  const connections = await (await request.get('/api/connections')).json() as ConnectionSummary[]
  const plan: DeploymentPlan = { exists: true, name: 'Recovery fixture', templateId: 'clm', template: 'CLM deployment', repository: 'example/mock-package', strategy: 'reuse', components: ['box', 'salesforce'].map((id) => ({ id, name: id === 'box' ? 'Box' : 'Salesforce', configured: true, verified: true, ready: true })) }
  const original: DispatchRun = { id: 'failed-original', deployment: plan.name, action: 'deploy', status: 'failed', providers: [{ name: 'box', status: 'present' }, { name: 'salesforce', status: 'failed' }] }
  const state = { unavailable: new Set<string>(), connections, plan, run: original, original, validations: 0, deployments: 0, diagnosticFails: true, oauthStatus: 'pending', oauthStarts: 0, oauthPolls: 0 }
  await page.route('**/api/**', async (route) => {
    const path = new URL(route.request().url()).pathname
    const json = (value: unknown, status = 200) => route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(value) })
    if (state.unavailable.has(path)) return json({ error: 'Local service unavailable' }, 503)
    if (path === '/api/plan') return json(state.plan)
    if (path === '/api/connections') return json(state.connections)
    if (/\/connections\/(box|salesforce)\/check$/.test(path)) return json(state.connections.find((connection) => connection.name.toLowerCase() === path.split('/')[3]))
    if (path === '/api/deployments') return json([])
    if (path === '/api/runs') {
      if (route.request().method() === 'POST') { state.validations++; state.run = { ...original, id: `fresh-validation-${state.validations}`, action: 'validate', status: 'completed' }; return json(state.run, 202) }
      return json([state.run])
    }
    if (path.endsWith('/diagnostics')) return state.diagnosticFails ? json({ error: 'Store unavailable' }, 503) : json({ title: 'Recovery guidance', summary: 'Review provider state before retrying.', nextSteps: ['Confirm connections, then validate again.'] })
    if (path.endsWith('/events')) return json({ error: 'Stream unavailable' }, 503)
    if (path.endsWith('/deploy')) {
      state.deployments++
      if (state.deployments === 1) return json({ error: 'Deployment could not be recorded; check local storage and try again.' }, 503)
      state.run = { ...original, id: 'accepted-recovery-deployment', status: 'running' }
      return json(state.run, 202)
    }
    if (path.startsWith('/api/runs/')) return json(state.run)
    if (path.endsWith('/oauth/start')) { state.oauthStarts++; return json({ id: `login-${state.oauthStarts}`, authorizeUrl: 'about:blank' }) }
    if (/\/oauth\/login-/.test(path)) { state.oauthPolls++; return json({ id: path.split('/').at(-1), status: state.oauthStatus, message: state.oauthStatus === 'pending' ? 'Finish login in the window.' : 'Login complete.' }) }
    return route.continue()
  })
  return state
}

for (const colorScheme of ['light', 'dark'] as const) for (const width of [390, 1440]) {
  test(`recovers unavailable data and keeps long connection actions visible at ${width}px in ${colorScheme}`, async ({ page, request }, info) => {
    await page.setViewportSize({ width, height: 1000 }); await page.emulateMedia({ colorScheme })
    const state = await fixtures(page, request)
    const longName = 'Northstar EMEA Regulatory Contract Administration — Disposable Validation Environment'
    for (const connection of state.connections) for (const record of [...(connection.connections ?? []), ...(connection.orgs ?? [])]) { record.alias = longName; record.domain = 'northstar-emea-regulatory-validation.sandbox.my.salesforce.com' }
    state.unavailable.add('/api/deployments')
    await page.goto('/#history')
    await expect(page.getByRole('heading', { name: 'Workspace data unavailable' })).toBeVisible()
    await expect(page.getByText('No deployments yet.')).toHaveCount(0)
    await page.screenshot({ path: info.outputPath('history-error.png'), fullPage: true })
    state.unavailable.clear()
    await page.getByRole('button', { name: 'Retry loading' }).click()
    await expect(page.getByRole('heading', { name: 'Deployment history', exact: true })).toBeVisible()
    await page.getByRole('link', { name: 'Settings', exact: true }).click()
    await expect(page.locator('.settings-connection-remove').first()).toBeVisible()
    for (const viewportWidth of width === 390 ? [390, 320] : [1440]) {
      await page.setViewportSize({ width: viewportWidth, height: 1000 })
      const bounds = await page.locator('.settings-connection-row').evaluateAll((rows) => rows.map((row) => {
        const card = row.closest('.settings-provider')!.getBoundingClientRect()
        const actions = row.querySelector('.settings-connection-actions')!.getBoundingClientRect()
        return { fits: actions.left >= card.left && actions.right <= card.right && actions.right <= document.documentElement.clientWidth, width: actions.width, cardLeft: card.left, cardRight: card.right, actionsLeft: actions.left, actionsRight: actions.right, viewport: document.documentElement.clientWidth }
      }))
      expect(bounds.length).toBeGreaterThan(0)
      expect(bounds.filter((bound) => !bound.fits || bound.width <= 0)).toEqual([])
      await page.screenshot({ path: info.outputPath(`long-actions-${viewportWidth}.png`), fullPage: true })
    }
    await page.setViewportSize({ width, height: 1000 })
    for (const provider of ['Box', 'Salesforce'] as const) {
      const panel = page.getByRole('region', { name: provider === 'Box' ? 'Box connections' : 'Salesforce environments', exact: true })
      await panel.getByRole('button', { name: 'Manage', exact: true }).click()
      const drawer = page.locator('box-drawer.connection-drawer')
      const drawerRows = drawer.locator('box-resource-row.connection-option-row')
      await expect(drawerRows.first()).toBeVisible()
      const drawerLayout = await drawer.evaluate((element) => {
        const drawerPart = element.shadowRoot!.querySelector<HTMLElement>('[part="drawer"]')!
        const bodyPart = element.shadowRoot!.querySelector<HTMLElement>('[part="body"]')!
        const footerPart = element.shadowRoot!.querySelector<HTMLElement>('[part="footer"]')!
        const closeButton = element.querySelector<HTMLElement>('[slot="footer"] box-button')!
        const drawerBounds = drawerPart.getBoundingClientRect()
        const footerBounds = footerPart.getBoundingClientRect()
        const closeBounds = closeButton.getBoundingClientRect()
        return {
          viewportHeight: window.innerHeight,
          drawerTop: drawerBounds.top,
          drawerBottom: drawerBounds.bottom,
          bodyClientHeight: bodyPart.clientHeight,
          bodyScrollHeight: bodyPart.scrollHeight,
          footerTop: footerBounds.top,
          footerBottom: footerBounds.bottom,
          closeTop: closeBounds.top,
          closeBottom: closeBounds.bottom,
        }
      })
      expect(drawerLayout.drawerTop).toBeGreaterThanOrEqual(-1)
      expect(drawerLayout.drawerBottom).toBeLessThanOrEqual(drawerLayout.viewportHeight + 1)
      expect(drawerLayout.footerBottom).toBeLessThanOrEqual(drawerLayout.viewportHeight + 1)
      expect(drawerLayout.closeTop).toBeGreaterThanOrEqual(0)
      expect(drawerLayout.closeBottom).toBeLessThanOrEqual(drawerLayout.viewportHeight + 1)
      if (width === 390) {
        expect(drawerLayout.bodyScrollHeight).toBeGreaterThan(drawerLayout.bodyClientHeight)
      }
      const drawerBounds = await drawerRows.evaluateAll((rows) => rows.map((row) => {
        const host = row.getBoundingClientRect()
        const trailing = row.shadowRoot!.querySelector<HTMLElement>('[part="trailing"]')!.getBoundingClientRect()
        const actions = row.querySelector<HTMLElement>('[slot="actions"]')!.getBoundingClientRect()
        return {
          fits: trailing.left >= host.left && trailing.right <= host.right && actions.left >= host.left && actions.right <= host.right && actions.right <= document.documentElement.clientWidth,
          hostLeft: host.left,
          hostRight: host.right,
          trailingLeft: trailing.left,
          trailingRight: trailing.right,
          actionsLeft: actions.left,
          actionsRight: actions.right,
          viewport: document.documentElement.clientWidth,
        }
      }))
      expect(drawerBounds.length).toBeGreaterThan(0)
      expect(drawerBounds.filter((bound) => !bound.fits)).toEqual([])
      await page.screenshot({ path: info.outputPath(`${provider.toLowerCase()}-drawer-actions.png`) })
      if (width === 390) {
        await drawer.evaluate((element) => {
          const bodyPart = element.shadowRoot!.querySelector<HTMLElement>('[part="body"]')!
          bodyPart.scrollTop = bodyPart.scrollHeight
        })
        const scrolledLayout = await drawer.evaluate((element) => {
          const bodyPart = element.shadowRoot!.querySelector<HTMLElement>('[part="body"]')!
          const footerBounds = element.shadowRoot!.querySelector<HTMLElement>('[part="footer"]')!.getBoundingClientRect()
          return { bodyScrollTop: bodyPart.scrollTop, footerTop: footerBounds.top, footerBottom: footerBounds.bottom }
        })
        expect(scrolledLayout.bodyScrollTop).toBeGreaterThan(0)
        expect(Math.abs(scrolledLayout.footerTop - drawerLayout.footerTop)).toBeLessThanOrEqual(1)
        expect(scrolledLayout.footerBottom).toBeLessThanOrEqual(drawerLayout.viewportHeight + 1)
        await page.screenshot({ path: info.outputPath(`${provider.toLowerCase()}-drawer-scroll-end.png`) })
      }
      await drawer.getByRole('button', { name: 'Close', exact: true }).click()
    }
    await page.getByRole('link', { name: 'Help', exact: true }).click()
    await expect(page.getByRole('heading', { name: 'Help', exact: true })).toBeVisible()
    await expect(page.getByRole('link', { name: 'Help', exact: true })).toHaveAttribute('aria-current', 'page')
    await page.screenshot({ path: info.outputPath('help.png'), fullPage: true })
    await page.reload()
    await expect(page.getByRole('heading', { name: 'Help', exact: true })).toBeVisible()
    await page.goBack()
    await expect(page.getByRole('heading', { name: 'Settings', exact: true })).toBeVisible()
  })

  test(`recovers diagnostics, retries with fresh validation, and reconciles broken SSE at ${width}px in ${colorScheme}`, async ({ page, request }, info) => {
    await page.setViewportSize({ width, height: 1000 }); await page.emulateMedia({ colorScheme })
    const state = await fixtures(page, request)
    await page.goto('/#workspace')
    await page.getByRole('button', { name: 'View diagnostic guidance' }).click()
    await expect(page.getByRole('button', { name: 'Retry diagnostics' })).toBeEnabled()
    await expect(page.locator('box-drawer.diagnostic-drawer')).not.toHaveAttribute('busy')
    await page.screenshot({ path: info.outputPath('diagnostics-error.png'), fullPage: true })
    state.diagnosticFails = false
    await page.getByRole('button', { name: 'Retry diagnostics' }).click()
    await expect(page.getByRole('heading', { name: 'Recovery guidance', exact: true })).toBeVisible()
    await page.keyboard.press('Escape')
    await expect(page.getByRole('button', { name: 'View diagnostic guidance' })).toBeFocused()
    await page.getByRole('button', { name: 'Edit connections' }).click()
    await expect(page.getByRole('heading', { name: 'Confirm connections' })).toBeVisible()
    await page.goBack()
    await page.reload()
    await page.getByRole('button', { name: 'Retry validation' }).click()
    await page.getByRole('button', { name: 'Continue to deployment' }).waitFor()
    expect(state.validations).toBe(1)
    expect(state.original.id).toBe('failed-original'); expect(state.original.status).toBe('failed')
    await page.getByRole('button', { name: 'Continue to deployment' }).click()
    await page.getByRole('button', { name: 'Start deployment', exact: true }).click()
    await expect(page.getByText('Deployment could not be recorded; check local storage and try again.', { exact: true })).toBeVisible()
    await expect(page.getByRole('button', { name: 'Continue to deployment' })).toBeEnabled()
    await page.getByRole('button', { name: 'Continue to deployment' }).click()
    await page.getByRole('button', { name: 'Start deployment', exact: true }).click()
    await expect(page.getByText(/Live updates are disconnected/)).toBeVisible()
    await page.screenshot({ path: info.outputPath('stream-disconnected.png'), fullPage: true })
    state.run = { ...state.run, status: 'completed', providers: [{ name: 'box', status: 'present' }, { name: 'salesforce', status: 'present' }] }
    await expect(page.getByRole('heading', { name: 'Recovery fixture is ready' })).toBeVisible()
    expect(state.validations).toBe(1); expect(state.deployments).toBe(2)
    await page.screenshot({ path: info.outputPath('stream-reconciled.png'), fullPage: true })
  })

  test(`cancels and restarts local login watchers at ${width}px in ${colorScheme}`, async ({ page, request }, info) => {
    await page.setViewportSize({ width, height: 1000 }); await page.emulateMedia({ colorScheme })
    const state = await fixtures(page, request)
    for (const connection of state.connections) { connection.connections = []; connection.orgs = []; connection.oauthConfigured = true }
    await page.goto('/#settings')
    for (const provider of ['Box', 'Salesforce']) {
      const panel = page.getByRole('region', { name: provider === 'Box' ? 'Box connections' : 'Salesforce environments', exact: true })
      await panel.getByRole('button', { name: 'Manage', exact: true }).click()
      const label = provider === 'Box' ? 'Connect Box user' : 'Connect org'
      const popupPromise = page.waitForEvent('popup')
      await page.getByRole('button', { name: label, exact: true }).click()
      const popup = await popupPromise
      await expect(page.getByRole('button', { name: `Waiting for ${provider}…` })).toBeDisabled()
      await popup.close()
      await expect(page.getByRole('button', { name: label, exact: true })).toBeEnabled()
      await expect(page.getByText('Login window closed. Start login again when you are ready.', { exact: true })).toBeVisible()
      await page.screenshot({ path: info.outputPath(`${provider}-canceled.png`), fullPage: true })
      const secondPopupPromise = page.waitForEvent('popup')
      await page.getByRole('button', { name: label, exact: true }).click()
      const secondPopup = await secondPopupPromise
      await page.getByRole('button', { name: 'Close', exact: true }).click()
      const pollsAtClose = state.oauthPolls
      await page.waitForTimeout(1200)
      expect(state.oauthPolls).toBe(pollsAtClose)
      await secondPopup.close()
      await panel.getByRole('button', { name: 'Manage', exact: true }).click()
      await expect(page.getByRole('button', { name: label, exact: true })).toBeEnabled()
      await page.evaluate(() => {
        const browserWindow = window as Window & { dispatchOriginalOpen?: typeof window.open }
        browserWindow.dispatchOriginalOpen = window.open
        window.open = () => null
      })
      await page.getByRole('button', { name: label, exact: true }).click()
      await expect(page.getByText(`The browser blocked the ${provider} login window. Allow popups for Dispatch, then try again.`, { exact: true })).toBeVisible()
      await expect(page.getByRole('button', { name: label, exact: true })).toBeEnabled()
      await page.evaluate(() => {
        const browserWindow = window as Window & { dispatchOriginalOpen?: typeof window.open }
        window.open = browserWindow.dispatchOriginalOpen!
      })
      await page.getByRole('button', { name: 'Close', exact: true }).click()
    }
    expect(state.oauthStarts).toBe(6)
  })
}

for (const [endpoint, hash] of [
  ['/api/plan', '#workspace'], ['/api/runs', '#workspace'], ['/api/templates', '#workspace'], ['/api/defaults', '#settings'], ['/api/connections', '#settings'],
]) {
  test(`distinguishes ${endpoint} failure from an empty workspace`, async ({ page, request }) => {
    const state = await fixtures(page, request)
    state.unavailable.add(endpoint)
    await page.goto(`/${hash}`)
    await expect(page.getByRole('heading', { name: 'Workspace data unavailable' })).toBeVisible()
    await expect(page.getByRole('button', { name: 'Prepare package' })).toHaveCount(0)
    await expect(page.getByText(/No saved (Box|Salesforce) connections/)).toHaveCount(0)
    state.unavailable.clear()
    await page.getByRole('button', { name: 'Retry loading' }).click()
    await expect(page.getByRole('heading', { name: 'Workspace data unavailable' })).toHaveCount(0)
    await expect(page.getByRole('heading', { name: hash === '#settings' ? 'Settings' : 'Deployment', exact: true })).toBeVisible()
    expect(state.validations).toBe(0); expect(state.deployments).toBe(0)
  })
}

test('closing a drawer while login starts prevents a late popup', async ({ page, request }) => {
  const state = await fixtures(page, request)
  state.connections.find((connection) => connection.name === 'Box')!.connections = []
  state.connections.find((connection) => connection.name === 'Box')!.oauthConfigured = true
  let release!: () => void
  const delayed = new Promise<void>((resolve) => { release = resolve })
  await page.route('**/api/connections/box/oauth/start', async (route) => {
    await delayed
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ id: 'old-start', authorizeUrl: 'about:blank' }) }).catch(() => undefined)
  })
  let popupCount = 0
  page.on('popup', () => { popupCount++ })
  await page.goto('/#settings')
  const panel = page.getByRole('region', { name: 'Box connections', exact: true })
  await panel.getByRole('button', { name: 'Manage', exact: true }).click()
  await page.getByRole('button', { name: 'Connect Box user', exact: true }).click()
  await expect(page.locator('box-drawer.connection-drawer')).toHaveAttribute('busy')
  await page.getByRole('button', { name: 'Close', exact: true }).click()
  release()
  await panel.getByRole('button', { name: 'Manage', exact: true }).click()
  await expect(page.getByRole('button', { name: 'Connect Box user', exact: true })).toBeEnabled()
  await page.waitForTimeout(500)
  expect(popupCount).toBe(0)
})

test('a delayed running-status response cannot overwrite a terminal stream event', async ({ page, request }) => {
  const state = await fixtures(page, request)
  state.run = { ...state.original, status: 'running', providers: [{ name: 'box', status: 'present' }, { name: 'salesforce', status: 'present' }] }
  let release!: () => void
  const delayed = new Promise<void>((resolve) => { release = resolve })
  let details = 0
  await page.route('**/api/runs/failed-original', async (route) => {
    details++
    const snapshot = { ...state.run }
    if (details === 1) await delayed
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify(snapshot) })
  })
  let eventRequests = 0
  await page.route('**/api/runs/failed-original/events', async (route) => {
    eventRequests++
    if (eventRequests === 1) return route.fulfill({ contentType: 'text/event-stream', body: 'retry: 500\nevent: dispatch\ndata: ' + JSON.stringify({ sequence: 0, type: 'status', at: '2026-10-11T18:00:00Z', status: 'running', message: 'Running' }) + '\n\n' })
    state.run = { ...state.run, status: 'completed' }
    return route.fulfill({ contentType: 'text/event-stream', body: `event: dispatch\ndata: ${JSON.stringify({ sequence: 1, type: 'status', at: '2026-10-11T18:00:00Z', status: 'completed', message: 'Complete' })}\n\n` })
  })
  await page.goto('/#workspace')
  await expect.poll(() => details).toBe(1)
  await expect(page.getByRole('heading', { name: 'Recovery fixture is ready' })).toBeVisible()
  release()
  await page.waitForTimeout(100)
  expect(await page.locator('.deployment-header box-badge.meta-status').getAttribute('label')).toBe('Deployment complete')
  await expect(page.getByRole('heading', { name: 'Recovery fixture is ready' })).toBeVisible()
  await expect.poll(() => details).toBeGreaterThan(1)
  await expect(page.getByRole('heading', { name: 'Recovery fixture is ready' })).toBeVisible()
  expect(state.deployments).toBe(0); expect(state.validations).toBe(0)
})

test('reopens a stream closed by an HTTP failure without starting provider work', async ({ page, request }) => {
  const state = await fixtures(page, request)
  state.run = { ...state.original, status: 'running', providers: [{ name: 'box', status: 'present' }, { name: 'salesforce', status: 'present' }] }
  let attempts = 0
  await page.route('**/api/runs/failed-original/events', async (route) => {
    attempts++
    if (attempts === 1) return route.fulfill({ status: 503, body: 'Unavailable' })
    state.run = { ...state.run, status: 'completed' }
    return route.fulfill({ contentType: 'text/event-stream', body: `event: dispatch\ndata: ${JSON.stringify({ sequence: 1, type: 'status', at: '2026-10-11T18:00:00Z', status: 'completed', message: 'Complete' })}\n\n` })
  })
  await page.goto('/#workspace')
  await expect.poll(() => attempts).toBeGreaterThan(1)
  await expect(page.getByRole('heading', { name: 'Recovery fixture is ready' })).toBeVisible()
  expect(state.validations).toBe(0); expect(state.deployments).toBe(0)
})

for (const provider of ['Box', 'Salesforce'] as const) for (const operation of ['select', 'remove'] as const) {
  test(`${provider} ${operation} remains serialized across drawer close and reopen`, async ({ page, request }) => {
    const state = await fixtures(page, request)
    const connection = state.connections.find((entry) => entry.name === provider)!
    const records = ['A', 'B', 'C'].map((alias) => ({ id: alias, alias, selected: alias === 'A', status: 'Ready', kind: 'Org', identity: `${alias}@example.com`, username: `${alias}@example.com` }))
    if (provider === 'Box') connection.connections = []
    else connection.orgs = []
    connection.oauthConfigured = true
    let release!: () => void
    const held = new Promise<void>((resolve) => { release = resolve })
    let calls = 0
    const endpoint = provider === 'Box' ? '/api/connections/box/selection' : '/api/connections/salesforce'
    await page.route(`**${operation === 'select' ? endpoint : '/api/connections/' + provider.toLowerCase() + '/B'}`, async (route) => {
      calls++
      await held
      if (operation === 'select') for (const record of records) record.selected = record.id === 'B'
      else if (provider === 'Box') connection.connections = records.filter((record) => record.id !== 'B')
      else connection.orgs = records.filter((record) => record.id !== 'B')
      await route.fulfill({ contentType: 'application/json', body: JSON.stringify(connection) })
    })
    await page.goto('/#settings')
    const panel = page.getByRole('region', { name: provider === 'Box' ? 'Box connections' : 'Salesforce environments', exact: true })
    await panel.getByRole('button', { name: 'Manage', exact: true }).click()
    const popupPromise = page.waitForEvent('popup')
    await page.getByRole('button', { name: provider === 'Box' ? 'Connect Box user' : 'Connect org', exact: true }).click()
    const popup = await popupPromise
    if (provider === 'Box') connection.connections = records
    else connection.orgs = records
    state.oauthStatus = 'active'
    await expect(page.locator('box-resource-row[label="B"]')).toBeVisible()
    await expect(page.locator('box-drawer.connection-drawer')).not.toHaveAttribute('busy')
    await popup.close()
    await page.getByRole('button', { name: 'Close', exact: true }).click()
    await panel.getByRole('button', { name: 'Manage', exact: true }).click()
    if (operation === 'select') await page.locator('box-resource-row[label="B"]').locator('button').first().click()
    else await page.locator('box-drawer.connection-drawer').getByRole('button', { name: 'Remove B', exact: true }).click()
    await expect.poll(() => calls).toBe(1)
    await page.getByRole('button', { name: 'Close', exact: true }).click()
    await panel.getByRole('button', { name: 'Manage', exact: true }).click()
    await expect(page.locator('box-resource-row[label="C"]')).toHaveAttribute('disabled')
    await expect(page.locator('box-drawer.connection-drawer').getByRole('button', { name: 'Remove C', exact: true })).toBeDisabled()
    release()
    await expect(page.locator('box-drawer.connection-drawer').getByRole('button', { name: 'Remove C', exact: true })).toBeEnabled()
    if (operation === 'select') await expect(page.locator('box-resource-row[label="B"]')).toHaveAttribute('selected')
    else await expect(page.locator('box-resource-row[label="B"]')).toHaveCount(0)
    expect(calls).toBe(1)
    expect(state.deployments).toBe(0); expect(state.validations).toBe(0)
  })
}

test('terminal REST status keeps incomplete validation activity explicitly qualified', async ({ page, request }) => {
  const state = await fixtures(page, request)
  state.run = { ...state.run, action: 'validate', status: 'running', providers: [] }
  let events = 0
  let release!: () => void
  const held = new Promise<void>((resolve) => { release = resolve })
  await page.route('**/api/runs/failed-original', async (route) => {
    await held
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify({ ...state.run, status: 'completed', providers: [{ name: 'box', status: 'present' }, { name: 'salesforce', status: 'present' }] }) })
  })
  await page.route('**/api/runs/failed-original/events', async (route) => {
    events++
    if (events > 1) return route.fulfill({ status: 503, body: 'Unavailable' })
    await route.fulfill({ contentType: 'text/event-stream', body: `event: dispatch\ndata: ${JSON.stringify({ sequence: 1, type: 'activity', provider: 'salesforce', component: 'ApexClass:Held', progressState: 'running', status: 'running', message: 'Checking Held', at: '2026-10-11T18:00:00Z' })}\n\n` })
  })
  await page.goto('/#workspace')
  await expect(page.locator('box-run-trace.dispatch-run-trace')).toContainText('Checking Held')
  release()
  await expect(page.getByRole('button', { name: 'Continue to deployment' })).toBeVisible()
  await expect(page.getByText(/Run status is confirmed, but activity history is incomplete/)).toBeVisible()
  await expect(page.getByText('Provider progress updates automatically.', { exact: true })).toHaveCount(0)
  expect(state.deployments).toBe(0); expect(state.validations).toBe(0)
})

test('a history retry cannot restore its saved workflow over a newly requested draft', async ({ page, request }) => {
  const state = await fixtures(page, request)
  state.unavailable.add('/api/deployments')
  await page.goto('/#history')
  await expect(page.getByRole('button', { name: 'Retry loading' })).toBeVisible()
  state.unavailable.clear()
  let release!: () => void
  const held = new Promise<void>((resolve) => { release = resolve })
  let requested = false
  await page.route('**/api/plan', async (route) => {
    requested = true
    await held
    await route.fulfill({ contentType: 'application/json', body: JSON.stringify(state.plan) })
  })
  await page.getByRole('button', { name: 'Retry loading' }).click()
  await expect.poll(() => requested).toBe(true)
  await page.locator('box-nav-sidebar').getByRole('link', { name: 'Deployments', exact: true }).click()
  release()
  await expect(page.getByRole('button', { name: 'Prepare package' })).toBeVisible()
  await expect(page.getByRole('textbox', { name: 'Name this deployment' })).toHaveValue('')
  await page.getByRole('textbox', { name: 'Name this deployment' }).fill('New draft after history retry')
  await expect(page.getByRole('heading', { name: 'New draft after history retry', exact: true })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Retry validation' })).toHaveCount(0)
  expect(state.validations).toBe(0); expect(state.deployments).toBe(0)
})
