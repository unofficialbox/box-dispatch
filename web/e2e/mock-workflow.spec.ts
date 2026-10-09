import { expect, test } from '@playwright/test'

test('configures, validates, and deploys against the mock backend', async ({ page }) => {
  const connectionChecks: string[] = []
  page.on('request', (request) => {
    if (request.method() === 'POST' && /\/api\/connections\/(box|salesforce)\/check$/.test(request.url())) connectionChecks.push(request.url())
  })
  await page.goto('/')
  await expect(page.getByRole('heading', { name: 'Overview' })).toBeVisible()
  await expect(page.locator('box-metric-card[heading="Connections"]')).not.toHaveAttribute('value', 'Checking')
  await expect(page.getByRole('link', { name: 'Overview', exact: true })).toHaveAttribute('aria-current', 'page')
  await expect(page.locator('box-app-shell')).toHaveAttribute('heading', 'Box Dispatch')
  await expect(page.getByRole('navigation')).toHaveCount(1)
  const sidebar = page.locator('box-nav-sidebar')
  await expect(sidebar).not.toHaveAttribute('collapsed', '')
  const expandedWidth = await sidebar.evaluate((element) => element.getBoundingClientRect().width)
  const expandedNavigationRows = await sidebar.locator('.nav-route').evaluateAll((rows) => rows.map((row) => {
    const icon = row.querySelector<HTMLElement>('[data-nav-icon]')!.getBoundingClientRect()
    const label = row.querySelector<HTMLElement>('[data-nav-label]')!.getBoundingClientRect()
    return {
      iconBeforeLabel: icon.right < label.left,
      centerDelta: Math.abs((icon.top + icon.height / 2) - (label.top + label.height / 2)),
      rowTextAlign: getComputedStyle(row).textAlign,
      labelTextAlign: getComputedStyle(row.querySelector<HTMLElement>('[data-nav-label]')!).textAlign,
    }
  }))
  expect(expandedNavigationRows.every(({ iconBeforeLabel, centerDelta, rowTextAlign, labelTextAlign }) => iconBeforeLabel && centerDelta < 0.5 && rowTextAlign === 'left' && labelTextAlign === 'left')).toBe(true)
  const activeNavigationStyle = await sidebar.locator('.nav-route.active').evaluate((row) => ({
    beforeContent: getComputedStyle(row, '::before').content,
    boxShadow: getComputedStyle(row).boxShadow,
  }))
  expect(activeNavigationStyle.beforeContent).toBe('none')
  expect(activeNavigationStyle.boxShadow).not.toContain('3px 0px')
  await page.locator('box-sidebar-toggle-button').getByRole('button', { name: 'Collapse navigation' }).click()
  await expect(sidebar).toHaveAttribute('collapsed', '')
  await expect.poll(() => sidebar.evaluate((element) => element.getBoundingClientRect().width)).toBeLessThan(expandedWidth)
  await page.locator('box-sidebar-toggle-button').getByRole('button', { name: 'Expand navigation' }).click()
  await expect(sidebar).not.toHaveAttribute('collapsed', '')
  await expect(page.getByText('acme.app.box.com')).toBeVisible()
  await expect(page.getByText('example.my.salesforce.com', { exact: true })).toBeVisible()
  await page.getByRole('link', { name: 'Settings', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Settings' })).toBeVisible()
  await expect(page.getByText(/\d+ saved/).first()).toBeVisible()
  await expect.poll(() => connectionChecks.filter((url) => url.endsWith('/box/check')).length).toBeGreaterThanOrEqual(2)
  await expect.poll(() => connectionChecks.filter((url) => url.endsWith('/salesforce/check')).length).toBeGreaterThanOrEqual(2)
  await expect(page.getByRole('link', { name: 'Settings', exact: true })).toHaveAttribute('aria-current', 'page')
  await expect(page.getByRole('link', { name: 'Overview', exact: true })).not.toHaveAttribute('aria-current')
  await expect(page.getByText('acme.app.box.com')).toBeVisible()
  await expect(page.getByText('example.my.salesforce.com', { exact: true })).toBeVisible()
  await page.goBack()
  await expect(page.getByRole('heading', { name: 'Overview' })).toBeVisible()
  await page.goForward()
  await expect(page.getByRole('heading', { name: 'Settings' })).toBeVisible()
  await page.getByRole('link', { name: 'Overview', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Overview' })).toBeVisible()

  const overviewGutter = await page.evaluate(() => {
    const progress = document.querySelector('box-progress-bar')!.getBoundingClientRect()
    const providerRail = document.querySelector('.overview-secondary')!.getBoundingClientRect()
    const history = document.querySelector('.overview-history-table')!.getBoundingClientRect()
    return {
      progressToDivider: providerRail.left - progress.right,
      historyToDivider: providerRail.left - history.right,
    }
  })
  expect(overviewGutter.progressToDivider).toBeGreaterThanOrEqual(29)
  expect(overviewGutter.historyToDivider).toBeGreaterThanOrEqual(29)

  await page.getByRole('button', { name: 'New deployment' }).click()
  await expect(page.getByRole('heading', { name: 'Choose a solution' })).toBeVisible()
  const workflow = page.locator('box-path')
  await expect(workflow).toHaveAttribute('current', 'Choose')
  await expect(workflow.locator('[part="stage"][data-stage-id="Deploy"]')).toHaveAttribute('data-state', 'upcoming')
  await expect(workflow.locator('[part="stage"][data-stage-id="Summary"]')).toHaveAttribute('data-state', 'upcoming')
  const nameFieldLayout = await page.evaluate(() => {
    const section = document.querySelector('.deployment-name-field')!.getBoundingClientRect()
    const field = document.querySelector('.deployment-name-field box-text-field')!.getBoundingClientRect()
    return {
      leftEdgeDelta: Math.abs(section.left - field.left),
      duplicateHelperVisible: document.body.innerText.includes('Shown in Overview and deployment history.'),
    }
  })
  expect(nameFieldLayout.leftEdgeDelta).toBeLessThan(0.01)
  expect(nameFieldLayout.duplicateHelperVisible).toBe(false)
  const providerAlignment = await page.evaluate(() => [...document.querySelectorAll<HTMLElement>('[data-system-provider]')].map((provider) => {
    const logo = provider.querySelector<HTMLElement>('.provider-logo')!.getBoundingClientRect()
    const copy = provider.querySelector<HTMLElement>('.system-provider-copy')!.getBoundingClientRect()
    return {
      provider: provider.dataset.systemProvider,
      centerDelta: Math.abs((logo.top + logo.height / 2) - (copy.top + copy.height / 2)),
    }
  }))
  expect(providerAlignment.map(({ provider }) => provider)).toEqual(['box', 'salesforce', 'databricks', 'amazon bedrock'])
  expect(providerAlignment.every(({ centerDelta }) => centerDelta < 0.01)).toBe(true)
  expect(await page.locator('[data-system-provider] img[data-provider-logo]').evaluateAll((logos) => logos.map((logo) => ({ provider: logo.getAttribute('data-provider-logo'), loaded: (logo as HTMLImageElement).naturalWidth > 0 })))).toEqual([
    { provider: 'box', loaded: true },
    { provider: 'salesforce', loaded: true },
    { provider: 'databricks', loaded: true },
    { provider: 'bedrock', loaded: true },
  ])
  await page.getByRole('textbox', { name: 'Name this deployment' }).fill('Northstar CLM rollout')
  await expect(page.getByRole('heading', { name: 'Northstar CLM rollout' })).toBeVisible()
  await page.getByRole('button', { name: 'Prepare package' }).click()

  await expect(page.getByRole('heading', { name: 'Confirm connections' })).toBeVisible()
  await expect(workflow).toHaveAttribute('current', 'Connect')
  await expect(workflow.locator('[part="stage"][data-stage-id="Choose"]')).toHaveAttribute('data-state', 'complete')
  await expect(page.locator('box-resource-row.connection-resource-row[label="Box"]')).toHaveAttribute('selected', '')
  await expect(page.locator('box-resource-row.connection-resource-row[label="Box"] box-badge')).toHaveAttribute('label', 'Ready')
  await expect(page.locator('box-resource-row.connection-resource-row[label="Salesforce"] box-badge')).toHaveAttribute('label', 'Ready')
  await expect(page.locator('.connection-buttons box-button[label="Open"][aria-label="Open Box"]')).toBeVisible()
  await expect(page.locator('.connection-buttons box-button[label="Open"][aria-label="Open Salesforce"]')).toBeVisible()
  await expect(page.locator('.connection-resource-row img[data-provider-logo]')).toHaveCount(2)
  expect(await page.locator('.connection-buttons box-button[aria-label^="Open "]').evaluateAll((buttons) => buttons.map((button) => button.getAttribute('label')))).toEqual(['Open', 'Open'])
  expect(await page.locator('button button, button box-button, button box-switch').count()).toBe(0)
  await page.getByRole('button', { name: 'Continue to configure' }).click()

  await expect(page.getByRole('heading', { name: 'Configure deployment' })).toBeVisible()
  const providerControls = await page.locator('.configuration-resource-row box-switch').evaluateAll((switches) => switches.map((control) => {
    const bounds = control.getBoundingClientRect()
    return {
      label: control.getAttribute('label'),
      description: control.getAttribute('description'),
      left: bounds.left,
      right: bounds.right,
      center: bounds.top + bounds.height / 2,
    }
  }))
  expect(providerControls.map(({ label, description }) => ({ label, description }))).toEqual([
    { label: 'Included', description: 'Required' },
    { label: 'Included', description: 'Optional' },
  ])
  expect(Math.max(...providerControls.map(({ left }) => left)) - Math.min(...providerControls.map(({ left }) => left))).toBeLessThan(0.01)
  expect(Math.max(...providerControls.map(({ right }) => right)) - Math.min(...providerControls.map(({ right }) => right))).toBeLessThan(0.01)
  await page.getByRole('button', { name: 'Review plan' }).click()

  await expect(page.getByRole('heading', { name: 'Review and validate' })).toBeVisible()
  await page.getByRole('button', { name: 'Validate deployment' }).click()
  await expect.poll(() => page.locator('box-run-trace').evaluate((trace) => {
    const runningMarker = trace.shadowRoot?.querySelector<HTMLElement>('[part="step"][data-step-id="salesforce"][data-status="running"] [part="marker"]')
    return runningMarker ? getComputedStyle(runningMarker).animationName : ''
  })).toContain('dispatch-provider-pulse')
  await page.reload()
  await expect(page.getByRole('heading', { name: 'Validation', exact: true })).toBeVisible()
  await expect(page.getByText('All selected systems finished successfully.')).toBeVisible()
  // Completed runs must replay their activity too, not only runs still in flight.
  await page.reload()
  await expect(page.getByText('All selected systems finished successfully.')).toBeVisible()
  await expect.poll(() => page.getByText('Authentication verified').count()).toBeGreaterThanOrEqual(2)
  const liveActivity = page.locator('box-timeline[aria-label="Live validation log"]')
  await expect(liveActivity).toBeVisible()
  await liveActivity.focus()
  await page.keyboard.press('PageDown')
  const liveActivityState = await liveActivity.evaluate((element) => ({ focused: document.activeElement === element, tabIndex: (element as HTMLElement).tabIndex, events: (element as HTMLElement & { events: unknown[] }).events.length }))
  expect(liveActivityState).toMatchObject({ focused: true, tabIndex: 0 })
  expect(liveActivityState.events).toBeGreaterThan(0)

  await page.getByRole('button', { name: 'Review changes' }).click()
  await expect(page.getByRole('heading', { name: 'Review validation changes' })).toBeVisible()
  await expect(page.getByText('2 files')).toBeVisible()
  await expect(page.locator('box-diff-viewer[heading="settings/Communities.settings-meta.xml"]')).toBeVisible()
  expect(await page.locator('box-diff-viewer').evaluate((viewer) => ({
    before: viewer.getAttribute('before-label'),
    after: viewer.getAttribute('after-label'),
    mode: viewer.getAttribute('mode'),
    hasCurrent: viewer.getAttribute('before-text')?.includes('false'),
    hasPackaged: viewer.getAttribute('after-text')?.includes('true'),
  }))).toEqual({ before: 'Current org', after: 'Validated package', mode: 'split', hasCurrent: true, hasPackaged: true })
  await page.locator('box-table.validation-change-table').locator('[part="row"][data-index="1"]').click()
  await expect(page.locator('box-diff-viewer[heading="objects/Contract__c/fields/Amount__c.field-meta.xml"]')).toBeVisible()
  await page.locator('box-table.validation-change-table').locator('[part="row"][data-index="0"]').click()
  await expect(page.locator('box-diff-viewer[heading="settings/Communities.settings-meta.xml"]')).toBeVisible()
  await page.getByRole('button', { name: 'Close drawer' }).click()

  for (const details of await page.getByRole('button', { name: 'Details' }).all()) await details.click()
  const childAlignment = await page.locator('box-run-trace').evaluate((trace) => {
    const rows = [...trace.shadowRoot!.querySelectorAll<HTMLElement>('[part="child"]')]
    const bars = rows.map((row) => row.querySelector<HTMLElement>('box-progress-bar')?.getBoundingClientRect()).filter((rect): rect is DOMRect => Boolean(rect))
    return {
      count: bars.length,
      widthDelta: Math.max(...bars.map((rect) => rect.width)) - Math.min(...bars.map((rect) => rect.width)),
      rightEdgeDelta: Math.max(...bars.map((rect) => rect.right)) - Math.min(...bars.map((rect) => rect.right)),
    }
  })
  expect(childAlignment.count).toBeGreaterThanOrEqual(2)
  expect(childAlignment.widthDelta).toBeLessThan(0.01)
  expect(childAlignment.rightEdgeDelta).toBeLessThan(0.01)

  const traceAlignment = await page.locator('box-run-trace').evaluate((trace) => {
    const steps = [...trace.shadowRoot!.querySelectorAll<HTMLElement>('[part="step"]')]
    const markers = steps.map((step) => step.querySelector<HTMLElement>('[part="marker"]')!.getBoundingClientRect())
    const connector = getComputedStyle(steps[0], '::after')
    const step = steps[0].getBoundingClientRect()
    const lineCenter = step.left + Number.parseFloat(connector.left) + Number.parseFloat(connector.width) / 2
    const lineTop = step.top + Number.parseFloat(connector.top)
    const lineBottom = step.bottom - Number.parseFloat(connector.bottom)
    return {
      markerCenterDelta: Math.abs((markers[0].left + markers[0].width / 2) - (markers[1].left + markers[1].width / 2)),
      connectorCenterDelta: Math.abs(lineCenter - (markers[0].left + markers[0].width / 2)),
      firstEdgeGap: Math.abs(lineTop - markers[0].bottom),
      secondEdgeGap: Math.abs(markers[1].top - lineBottom),
    }
  })
  expect(traceAlignment.markerCenterDelta).toBeLessThan(0.01)
  expect(traceAlignment.connectorCenterDelta).toBeLessThan(0.01)
  expect(traceAlignment.firstEdgeGap).toBeLessThan(0.01)
  expect(traceAlignment.secondEdgeGap).toBeLessThan(0.01)

  await page.getByRole('button', { name: 'Continue to deployment' }).click()
  await expect(page.getByRole('heading', { name: 'Start deployment?' })).toBeVisible()
  const confirmationHost = page.locator('box-dialog[heading="Start deployment?"]')
  const confirmation = page.getByRole('dialog', { name: 'Start deployment?' })
  await expect(confirmation).toBeVisible()
  expect(await confirmation.evaluate((dialog) => dialog.matches(':modal'))).toBe(true)
  await page.getByRole('link', { name: 'Overview', exact: true, includeHidden: true }).evaluate((link) => link.focus())
  expect(await confirmationHost.evaluate((host) => document.activeElement === host && host.shadowRoot?.activeElement?.getAttribute('part') === 'dialog')).toBe(true)
  for (let index = 0; index < 4; index++) {
    await page.keyboard.press('Tab')
    expect(await confirmationHost.evaluate((host) => {
      const surface = host.shadowRoot?.querySelector('[part="dialog"]')
      const active = host.shadowRoot?.activeElement
      return document.activeElement === host && Boolean(active && surface?.contains(active))
    })).toBe(true)
  }
  await page.keyboard.press('Escape')
  await expect(confirmation).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'Continue to deployment' })).toBeFocused()
  await page.getByRole('button', { name: 'Continue to deployment' }).click()
  await page.getByRole('button', { name: 'Start deployment' }).click()

  await expect(page.getByRole('heading', { name: 'Northstar CLM rollout is ready' })).toBeVisible()
  await expect(page.getByText('Every selected system finished successfully. Open a destination to review the deployed experience.')).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Open your deployment' })).toBeVisible()
  await expect(page.getByText('Box workspace')).toBeVisible()
  await expect(page.getByRole('link', { name: 'Open Box App & Settings' })).toBeVisible()
  await expect(page.getByRole('link', { name: 'Open Contract Lifecycle Management' })).toBeVisible()
  await page.getByRole('button', { name: 'Review changes' }).click()
  await expect(page.getByRole('heading', { name: 'Review deployed changes' })).toBeVisible()
  expect(await page.locator('box-diff-viewer').evaluate((viewer) => ({ before: viewer.getAttribute('before-label'), after: viewer.getAttribute('after-label') }))).toEqual({ before: 'Before deployment', after: 'After deployment' })
  await page.getByRole('button', { name: 'Close drawer' }).click()

  await page.getByRole('link', { name: 'Overview', exact: true }).click()
  await expect(page.getByText('Northstar CLM rollout').first()).toBeVisible()
  const recentDeployments = page.getByRole('table', { name: 'Recent deployments' })
  await recentDeployments.getByRole('link', { name: 'Northstar CLM rollout' }).click()
  await expect(page.getByRole('heading', { name: 'Northstar CLM rollout' })).toBeVisible()
  await page.reload()
  await expect(page.getByRole('heading', { name: 'Northstar CLM rollout' })).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Provider summary' })).toBeVisible()
  await expect(page.getByText('The immutable audit record for this run.')).toBeVisible()
  await expect(page.locator('.history-provider-card').filter({ hasText: 'Box' }).locator('header box-button[label="Open Box"]')).toBeVisible()
  await expect(page.locator('.history-provider-card').filter({ hasText: 'Salesforce' }).locator('header box-button[label="Open Salesforce"]')).toBeVisible()
  await expect(page.locator('box-button[label*="EID-mock"], box-button[label*="00D-mock"]')).toHaveCount(0)
  await expect(page.getByRole('table', { name: 'Components deployed by this deployment' })).toBeVisible()
  await expect(page.getByText('Sample Content:northstar-msa-redline-v3.pdf')).toBeVisible()
  await expect(page.getByText('UIBundle:clmreactapp')).toBeVisible()
  await page.getByRole('button', { name: 'Review changes' }).click()
  await expect(page.getByRole('heading', { name: 'Review deployed changes' })).toBeVisible()
  await expect(page.getByText('2 files')).toBeVisible()
  expect(await page.locator('box-diff-viewer').evaluate((viewer) => ({ before: viewer.getAttribute('before-label'), after: viewer.getAttribute('after-label') }))).toEqual({ before: 'Before deployment', after: 'After deployment' })
  await page.getByRole('button', { name: 'Close drawer' }).click()
  await page.getByRole('link', { name: 'Back to deployment history' }).click()
  await expect(page.getByRole('heading', { name: 'Deployment history' })).toBeVisible()
  await expect(page.getByLabel('Search')).toBeVisible()
  await expect(page.getByLabel('System', { exact: true })).toBeVisible()
  await expect(page.getByLabel('Result', { exact: true })).toBeVisible()
  await expect(page.getByLabel('Strategy', { exact: true })).toBeVisible()
  await page.setViewportSize({ width: 390, height: 844 })
  await page.getByRole('link', { name: 'Northstar CLM rollout' }).click()
  await expect(page.getByRole('heading', { name: 'Deployment details' })).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)).toBeLessThanOrEqual(1)
})

test('keeps the primary workflow usable on a mobile viewport', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto('/')
  expect(await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)).toBeLessThanOrEqual(1)
  await page.getByRole('button', { name: 'New deployment' }).click()

  await expect(page.getByRole('heading', { name: 'Choose a solution' })).toBeVisible()
  const mobileProviderAlignment = await page.evaluate(() => [...document.querySelectorAll<HTMLElement>('[data-system-provider]')].map((provider) => {
    const logo = provider.querySelector<HTMLElement>('.provider-logo')!.getBoundingClientRect()
    const copy = provider.querySelector<HTMLElement>('.system-provider-copy')!.getBoundingClientRect()
    return Math.abs((logo.top + logo.height / 2) - (copy.top + copy.height / 2))
  }))
  expect(mobileProviderAlignment).toHaveLength(4)
  expect(mobileProviderAlignment.every((centerDelta) => centerDelta < 0.01)).toBe(true)
  await page.getByRole('textbox', { name: 'Name this deployment' }).fill('Mobile CLM rollout')
  await expect(page.getByRole('button', { name: 'Prepare package' })).toBeVisible()
  const layout = await page.evaluate(() => {
    const rect = (element: Element) => element.getBoundingClientRect()
    const title = rect(document.querySelector('.title-row h1')!)
    const status = rect(document.querySelector('.meta-status')!)
    const solutionGroup = document.querySelector<HTMLElement>('box-tile-group.solution-list')!
    const group = rect(solutionGroup)
    const options = [...solutionGroup.shadowRoot!.querySelectorAll<HTMLElement>('[part="tile"]')].map((option) => {
      const row = rect(option)
      return {
        inset: row.left - group.left,
        clipped: row.left < group.left || row.right > group.right,
      }
    })
    return {
      overflow: document.documentElement.scrollWidth - document.documentElement.clientWidth,
      titleStatusCenterDelta: Math.abs((title.top + title.height / 2) - (status.top + status.height / 2)),
      options,
    }
  })
  expect(layout.overflow).toBeLessThanOrEqual(1)
  expect(layout.titleStatusCenterDelta).toBeLessThan(0.01)
  expect(layout.options.length).toBeGreaterThan(0)
  expect(layout.options.every((option) => option.inset >= 0 && !option.clipped)).toBe(true)

  await page.evaluate(() => window.scrollTo(0, document.body.scrollHeight))
  expect(await page.evaluate(() => window.scrollY)).toBeGreaterThan(0)
  await page.getByRole('button', { name: 'Prepare package' }).click()
  await expect(page.getByRole('heading', { name: 'Confirm connections' })).toBeVisible()
  expect(await page.evaluate(() => window.scrollY)).toBe(0)

  await page.evaluate(() => window.scrollTo(0, document.body.scrollHeight))
  await page.getByRole('button', { name: 'Continue to configure' }).click()
  await expect(page.getByRole('heading', { name: 'Configure deployment' })).toBeVisible()
  expect(await page.evaluate(() => window.scrollY)).toBe(0)
})

test('edits workspace defaults and applies them to a new deployment', async ({ page }) => {
  await page.goto('/#settings')
  const defaults = page.locator('.defaults-settings')

  await expect(defaults.getByRole('heading', { name: 'Defaults' })).toBeVisible()
  await expect(defaults.getByText('Starting configuration for new deployments. Each deployment can override these choices.')).toBeVisible()
  await expect(defaults.getByText('Readiness')).toHaveCount(0)
  await expect(page.getByText('Package configuration')).toHaveCount(0)
  await expect(defaults.locator('box-select[label="Default solution"]')).toHaveCount(0)
  await expect(defaults.getByRole('radio', { name: /Contract Lifecycle Management \(CLM\)/ })).toBeChecked()
  await expect(defaults.getByRole('radio', { name: /Citizen Services/ })).toBeDisabled()
  await expect(defaults.getByRole('radio', { name: /Life Sciences eTMF/ })).toBeDisabled()
  await expect(defaults.getByRole('radio', { name: /Insurance Claims Management/ })).toBeDisabled()
  await expect(defaults.locator('box-tile-group.settings-default-solution-list').getByText('Coming soon', { exact: true })).toHaveCount(3)
  await expect(defaults.getByText('Strategy', { exact: true })).toHaveCount(0)
  await expect(defaults.getByText('Systems', { exact: true })).toHaveCount(0)
  await expect(defaults.getByText('Source', { exact: true })).toHaveCount(0)

  await defaults.locator('box-button[label="Edit defaults"]').click()
  await expect(defaults.locator('box-select[label="Default solution"]')).toHaveCount(0)
  await expect(defaults.getByRole('radio', { name: /Contract Lifecycle Management \(CLM\)/ })).toBeChecked()
  await expect(defaults.getByRole('radio', { name: /Citizen Services/ })).toBeDisabled()
  await expect(defaults.getByRole('radio', { name: /Life Sciences eTMF/ })).toBeDisabled()
  await expect(defaults.getByRole('radio', { name: /Insurance Claims Management/ })).toBeDisabled()
  await expect(defaults.locator('box-tile-group.settings-default-solution-list').getByText('Coming soon', { exact: true })).toHaveCount(3)
  await expect(defaults.getByText('Source', { exact: true })).toBeVisible()
  await expect(defaults.getByRole('link', { name: 'https://github.com/unofficialbox/box-bedrock-for-clm' })).toBeVisible()
  await defaults.locator('box-tile-group.strategy-picker').locator('[part="tile"][data-option-id="create_new"]').click()
  await defaults.locator('box-switch[label="Salesforce"]').evaluate((control) => {
    control.dispatchEvent(new CustomEvent('checked-changed', { detail: { checked: false } }))
  })
  await defaults.locator('box-button[label="Save defaults"]').click()
  await expect(page.locator('box-toast').getByText('Deployment defaults saved.')).toBeVisible()

  await expect(defaults.getByText('Contract Lifecycle Management')).toBeVisible()
  await expect(defaults.getByText('Create new')).toHaveCount(0)
  await expect(defaults.getByText('Source', { exact: true })).toHaveCount(0)

  await defaults.locator('box-button[label="Edit defaults"]').click()
  await expect(defaults.getByRole('radio', { name: /Create new/ })).toBeChecked()
  await expect(defaults.locator('box-switch[label="Salesforce"]')).not.toHaveAttribute('checked')
  await expect(defaults.getByText('Source', { exact: true })).toBeVisible()
  await defaults.locator('box-button[label="Cancel"]').click()

  await page.getByRole('link', { name: 'Deployments' }).click()
  await expect(page.getByRole('heading', { name: 'Choose a solution' })).toBeVisible()
  await expect(page.locator('box-tile-group.solution-list').locator('[part="tile"][data-selected="true"]')).toContainText('Contract Lifecycle Management')
  await expect(page.locator('box-switch[label="Salesforce"]')).not.toHaveAttribute('checked')

  await page.request.put('/api/defaults', { data: { templateId: 'clm', strategy: 'reuse', components: ['box', 'salesforce'] } })
})

test('returns an authentication failure to a resumable connection step and recovers', async ({ page }) => {
  await page.route('**/api/connections/salesforce/check', async (route) => {
    await route.fulfill({ status: 503, contentType: 'application/json', body: JSON.stringify({ error: 'Salesforce session expired. Reconnect the selected Salesforce org.' }) })
  })
  await page.request.post('/api/packages', { data: { name: 'Recovery CLM rollout', templateId: 'clm', components: ['box', 'salesforce'], strategy: 'reuse' } })

  await page.goto('/')
  await expect(page.getByRole('heading', { name: 'Overview' })).toBeVisible()
  const salesforcePanel = page.locator('.provider-connection-panel--compact[aria-label="Salesforce"]')
  const notReadyBadge = salesforcePanel.locator('box-badge[label="Not ready"]')
  await expect(notReadyBadge).toBeVisible()
  const statusBounds = await salesforcePanel.evaluate((panel) => {
    const badge = panel.querySelector('box-badge[label="Not ready"]')!.getBoundingClientRect()
    const row = panel.querySelector('.settings-connection-row')!.getBoundingClientRect()
    return {
      insetRight: row.right - badge.right,
      insideLeftEdge: badge.left >= row.left,
    }
  })
  expect(statusBounds.insetRight).toBeGreaterThanOrEqual(12)
  expect(statusBounds.insideLeftEdge).toBe(true)

  await page.goto('/#workspace')
  await expect(page.getByRole('heading', { name: 'Confirm connections' })).toBeVisible()
  await expect(page.locator('box-resource-row.connection-resource-row[label="Salesforce"] box-badge')).toHaveAttribute('label', 'Needs attention')

  await page.unroute('**/api/connections/salesforce/check')
  await page.getByRole('button', { name: 'Continue to configure' }).click()
  await expect(page.getByRole('button', { name: 'Review plan' })).toBeEnabled()
  await page.getByRole('button', { name: 'Review plan' }).click()
  await expect(page.getByRole('heading', { name: 'Review and validate' })).toBeVisible()
  await page.getByRole('button', { name: 'Validate deployment' }).click()
  await expect(page.getByText('All selected systems finished successfully.')).toBeVisible()
})
