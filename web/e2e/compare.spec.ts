import { expect, test } from '@playwright/test'

test('compares two Box enterprises live without changing the deployment selection', async ({ page }) => {
  let comparisonRequests = 0
  let folderComparisonRequests = 0
  let aiAgentComparisonRequests = 0
  const folderComparisonBodies: Array<Record<string, string>> = []
  page.on('request', (request) => {
    if (request.method() === 'POST' && request.url().endsWith('/api/box/comparisons')) comparisonRequests += 1
    if (request.method() === 'POST' && request.url().endsWith('/api/box/comparisons/folders')) {
      folderComparisonRequests += 1
      folderComparisonBodies.push(request.postDataJSON() as Record<string, string>)
    }
    if (request.method() === 'POST' && request.url().endsWith('/api/box/comparisons/ai-agents')) aiAgentComparisonRequests += 1
  })
  await page.goto('/')
  const selectedBefore = await page.evaluate(async () => {
    const connections = await fetch('/api/connections').then((response) => response.json()) as Array<{ name: string; connections?: Array<{ id: string; selected: boolean }> }>
    return connections.find((connection) => connection.name === 'Box')?.connections?.find((connection) => connection.selected)?.id
  })
  expect(selectedBefore).toBe('box-mock-production')

  await page.getByRole('link', { name: 'Compare Box environments' }).click()
  await expect(page.getByRole('heading', { name: 'Compare Box environments' })).toBeVisible()
  await expect(page.getByText('Comparison direction')).toHaveCount(0)
  await expect(page.getByText(/Query two Box enterprises now/)).toHaveCount(0)
  await expect(page.locator('box-select[label="Source Box connection"]')).toHaveAttribute('value', 'box-mock-production')
  await expect(page.locator('box-select[label="Destination Box connection"]')).toHaveAttribute('value', 'box-mock-sandbox')
  await page.locator('box-button[label="Run live comparison"]').click()

  await expect(page.getByRole('heading', { name: 'Metadata templates', exact: true })).toBeVisible()
  await expect(page.locator('box-metric-card[heading="Changed"]')).toHaveAttribute('value', '1')
  await expect(page.locator('box-metric-card[heading="Missing"]')).toHaveAttribute('value', '1')
  await expect(page.locator('box-metric-card[heading="Extra"]')).toHaveAttribute('value', '1')
  await expect(page.locator('box-metric-card[heading="Equal"]')).toHaveAttribute('value', '0')
  const diff = page.locator('box-diff-viewer[heading="contracts.json"]')
  await expect(diff).toBeVisible()
  await expect(diff).toHaveAttribute('before-label', 'Acme Production')
  await expect(diff).toHaveAttribute('after-label', 'Acme Sandbox')
  await expect(page.getByText('fields.status.options')).toHaveCount(0)
  await expect(page.getByText('Showing 2 of 3')).toBeVisible()
  await expect(page.locator('box-icon-button[icon="plus"][label="Add Contracts to deployment plan"]')).toBeVisible()
  await expect(page.locator('box-icon-button[icon="plus"][label="Add Retention to deployment plan"]')).toBeVisible()
  await page.locator('box-icon-button[icon="plus"][label="Add Contracts to deployment plan"]').click()
  await expect(page.getByText('1 of 2 selected')).toBeVisible()
  const removeContract = page.locator('box-icon-button[label="Remove Contracts from deployment plan"]')
  await removeContract.hover()
  const errorColor = await page.evaluate(() => {
    const probe = document.createElement('span')
    probe.style.color = 'var(--boe-token-surface-status-surface-error, #ed3757)'
    document.body.append(probe)
    const color = getComputedStyle(probe).color
    probe.remove()
    return color
  })
  await expect.poll(() => removeContract.evaluate((host) => getComputedStyle(host.shadowRoot!.querySelector<HTMLElement>('[part="button"]')!).color)).toBe(errorColor)
  const addRetention = page.locator('box-icon-button[label="Add Retention to deployment plan"]')
  await addRetention.hover()
  await expect.poll(() => addRetention.evaluate((host) => getComputedStyle(host.shadowRoot!.querySelector<HTMLElement>('[part="button"]')!).color)).not.toBe(errorColor)
  await removeContract.click()
  await expect(page.getByText('0 of 2 selected')).toBeVisible()
  await expect(page.getByText('acme.app.box.com', { exact: true })).toBeVisible()
  await expect(page.getByText('acme-sandbox.app.box.com', { exact: true })).toBeVisible()

  await page.locator('box-button[label="Run live comparison"]').click()
  await expect.poll(() => comparisonRequests).toBe(2)

  await page.locator('box-select[label="Box object type"]').evaluate((element) => {
    element.setAttribute('value', 'folders')
    element.dispatchEvent(new CustomEvent('value-changed', { bubbles: true, composed: true, detail: { value: 'folders' } }))
  })
  const scopeTrees = page.locator('box-folder-tree')
  await expect(scopeTrees).toHaveCount(2)
  await scopeTrees.evaluateAll((trees) => trees.forEach((tree) => { tree.expandedIds = ['0'] }))
  await expect(scopeTrees.nth(0).locator('[data-node-id="contracts-production"]')).toBeVisible()
  await expect(scopeTrees.nth(1).locator('[data-node-id="contracts-sandbox"]')).toBeVisible()

  let releaseComparison: (() => void) | undefined
  await page.route('**/api/box/comparisons/folders', async (route) => {
    await new Promise<void>((resolve) => { releaseComparison = resolve })
    await route.continue().catch(() => undefined)
  })
  await page.locator('box-button[label="Run live comparison"]').click()
  await expect(page.locator('box-button[label="Cancel comparison"]')).toBeVisible()
  await page.locator('box-button[label="Cancel comparison"]').click()
  await expect(page.locator('box-alert[heading="Comparison stopped"]')).toHaveAttribute('message', 'Comparison canceled. No provider changes were made.')
  releaseComparison?.()
  await page.unroute('**/api/box/comparisons/folders')

  await page.locator('box-button[label="Run live comparison"]').click()
  await expect(page.getByRole('heading', { name: 'Folders', exact: true })).toBeVisible()
  await expect(page.locator('box-metric-card[heading="Changed"]')).toHaveAttribute('value', '1')
  await expect(page.locator('box-metric-card[heading="Missing"]')).toHaveAttribute('value', '4')
  await expect(page.locator('box-metric-card[heading="Extra"]')).toHaveAttribute('value', '4')
  await expect(page.locator('box-metric-card[heading="Equal"]')).toHaveAttribute('value', '1')
  const folderTree = page.locator('box-folder-tree-diff')
  await expect(folderTree).toBeVisible()
  await expect(folderTree).toHaveAttribute('mode', 'side-by-side')
  await expect(folderTree).toHaveAttribute('left-label', 'Acme Production')
  await expect(folderTree).toHaveAttribute('right-label', 'Acme Sandbox')
  await expect(folderTree).toHaveAttribute('changes-only', '')
  await expect(folderTree.locator('[part="counts"]')).toContainText('1 equal · 1 changed · 4 added · 4 removed · 0 moved')
  await expect(folderTree.locator('[part~="item"]')).toHaveCount(9)
  await expect(folderTree.locator('[part~="item"][data-status="changed"]')).toContainText('files 2 → 3')
  await expect(folderTree.locator('[part~="item"][data-node-id="entry:/Contracts/Archive/legacy.txt"]')).toContainText('legacy.txt')
  await expect(folderTree.locator('[part~="item"][data-node-id="entry:/Contracts/Templates/template.docx"]')).toContainText('template.docx')
  const folderFilter = folderTree.locator('button', { hasText: 'Show unchanged' })
  await expect(folderFilter).toBeVisible()
  await folderFilter.click()
  await expect(folderTree).not.toHaveAttribute('changes-only', '')
  await expect(folderTree.locator('[part~="item"]')).toHaveCount(10)
  await expect(folderTree.locator('[part~="item"][data-status="equal"]')).toContainText('agreement.pdf')
  await folderTree.locator('button', { hasText: 'Changes only' }).click()
  await expect(folderTree).toHaveAttribute('changes-only', '')
  await folderTree.locator('button', { hasText: 'Next difference' }).click()
  await expect.poll(() => folderTree.evaluate((tree) => tree.activeId)).toBe('folder:/Contracts/Archive')
  const folderSearch = page.locator('box-text-field[label="Filter folders and files"]')
  await folderSearch.evaluate((element) => {
    element.setAttribute('value', 'legacy.txt')
    element.dispatchEvent(new CustomEvent('value-changed', { bubbles: true, composed: true, detail: { value: 'legacy.txt' } }))
  })
  await expect(folderTree.locator('[part~="item"]')).toHaveCount(3)
  await expect(folderTree.locator('[data-node-id="entry:/Contracts/Archive/legacy.txt"]')).toBeVisible()
  await folderSearch.evaluate((element) => {
    element.setAttribute('value', '')
    element.dispatchEvent(new CustomEvent('value-changed', { bubbles: true, composed: true, detail: { value: '' } }))
  })
  await expect(folderTree.locator('[part~="item"]')).toHaveCount(9)
  await expect(page.locator('box-diff-viewer[heading="folder-structure.json"]')).toHaveCount(0)
  await expect(page.getByText('items.files')).toHaveCount(0)
  await expect.poll(() => folderComparisonRequests).toBe(2)
  expect(folderComparisonBodies.at(-1)).toMatchObject({ leftRootId: '0', rightRootId: '0' })

  const sourceFolderSearch = page.locator('.folder-scope-picker').nth(0).locator('box-text-field[label="Find a folder"]')
  await sourceFolderSearch.evaluate((element) => {
    element.setAttribute('value', 'archive')
    element.dispatchEvent(new CustomEvent('value-changed', { bubbles: true, composed: true, detail: { value: 'archive' } }))
  })
  await expect(scopeTrees.nth(0).locator('[data-node-id="archive-production"]')).toBeVisible()
  await sourceFolderSearch.evaluate((element) => {
    element.setAttribute('value', '')
    element.dispatchEvent(new CustomEvent('value-changed', { bubbles: true, composed: true, detail: { value: '' } }))
  })
  await scopeTrees.nth(0).evaluate((tree) => {
    const node = tree.nodes.find((candidate) => candidate.id === 'contracts-production')!
    tree.dispatchEvent(new CustomEvent('selected-changed', { bubbles: true, composed: true, detail: { id: node.id, node, selectedIds: [node.id] } }))
  })
  await scopeTrees.nth(1).evaluate((tree) => {
    const node = tree.nodes.find((candidate) => candidate.id === 'contracts-sandbox')!
    tree.dispatchEvent(new CustomEvent('selected-changed', { bubbles: true, composed: true, detail: { id: node.id, node, selectedIds: [node.id] } }))
  })
  await expect(page.locator('.folder-scope-picker').nth(0).locator('header strong')).toHaveText('Contracts')
  await expect(page.locator('.folder-scope-picker').nth(1).locator('header strong')).toHaveText('Contracts')
  await page.locator('box-button[label="Run live comparison"]').click()
  await expect(page.getByText('Contracts → Contracts')).toBeVisible()
  await expect(page.locator('box-metric-card[heading="Changed"]')).toHaveAttribute('value', '0')
  await expect(page.locator('box-metric-card[heading="Missing"]')).toHaveAttribute('value', '2')
  await expect(page.locator('box-metric-card[heading="Extra"]')).toHaveAttribute('value', '2')
  await expect(page.locator('box-metric-card[heading="Equal"]')).toHaveAttribute('value', '1')
  await expect.poll(() => folderComparisonRequests).toBe(3)
  expect(folderComparisonBodies.at(-1)).toMatchObject({ leftRootId: 'contracts-production', rightRootId: 'contracts-sandbox' })

  await page.route('**/api/box/comparisons/folders', async (route) => {
    const response = await route.fetch()
    const body = await response.json() as { scope: { leftTruncated: boolean } }
    body.scope.leftTruncated = true
    await route.fulfill({ response, json: body })
  })
  await page.locator('box-button[label="Run live comparison"]').click()
  await expect(page.locator('box-alert[heading="Partial folder comparison"]')).toBeVisible()
  await expect(page.locator('box-alert[heading="Partial folder comparison"]')).toHaveAttribute('message', /Items beyond that boundary are not represented/)
  await page.unroute('**/api/box/comparisons/folders')
  await expect.poll(() => folderComparisonRequests).toBe(4)

  await page.setViewportSize({ width: 390, height: 844 })
  await expect(folderTree).toHaveAttribute('mode', 'unified')
  expect(await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)).toBeLessThanOrEqual(1)
  await page.setViewportSize({ width: 1440, height: 1000 })
  await expect(folderTree).toHaveAttribute('mode', 'side-by-side')

  await page.locator('box-select[label="Box object type"]').evaluate((element) => {
    element.setAttribute('value', 'ai_agents')
    element.dispatchEvent(new CustomEvent('value-changed', { bubbles: true, composed: true, detail: { value: 'ai_agents' } }))
  })
  await page.locator('box-button[label="Run live comparison"]').click()
  await expect(page.getByRole('heading', { name: 'AI Studio custom agents' })).toBeVisible()
  await expect(page.locator('box-metric-card[heading="Changed"]')).toHaveAttribute('value', '1')
  await expect(page.locator('box-metric-card[heading="Missing"]')).toHaveAttribute('value', '1')
  await expect(page.locator('box-metric-card[heading="Extra"]')).toHaveAttribute('value', '1')
  const comparisonAgentDiff = page.locator('box-diff-viewer[heading="contract-extractor.json"]')
  await expect(comparisonAgentDiff).toBeVisible()
  await expect(page.getByText('configuration.extract.long_text.model')).toHaveCount(0)
  await expect(comparisonAgentDiff).toHaveAttribute('before-text', /AI Studio custom agent/)
  await expect(comparisonAgentDiff).toHaveAttribute('before-text', /Extract/)
  await expect.poll(() => aiAgentComparisonRequests).toBe(1)

  const desktopShellGeometry = await page.locator('box-app-shell').evaluate((shell) => {
    const navigation = shell.shadowRoot?.querySelector<HTMLElement>('[part="nav"]')?.getBoundingClientRect()
    const main = shell.shadowRoot?.querySelector<HTMLElement>('[part="main"]')?.getBoundingClientRect()
    const sidebar = shell.querySelector<HTMLElement>('box-nav-sidebar')?.getBoundingClientRect()
    const sidebarPanel = shell.querySelector<HTMLElement>('box-nav-sidebar')?.shadowRoot
      ?.querySelector<HTMLElement>('[part="sidebar"]')?.getBoundingClientRect()
    return {
      mainBottom: main?.bottom ?? 0,
      navigationBottom: navigation?.bottom ?? 0,
      sidebarBottom: sidebar?.bottom ?? 0,
      sidebarPanelBottom: sidebarPanel?.bottom ?? 0,
    }
  })
  expect(desktopShellGeometry.navigationBottom).toBeCloseTo(desktopShellGeometry.mainBottom, 0)
  expect(desktopShellGeometry.sidebarBottom).toBeCloseTo(desktopShellGeometry.navigationBottom, 0)
  expect(desktopShellGeometry.sidebarPanelBottom).toBeCloseTo(desktopShellGeometry.navigationBottom, 0)

  const selectedAfter = await page.evaluate(async () => {
    const connections = await fetch('/api/connections').then((response) => response.json()) as Array<{ name: string; connections?: Array<{ id: string; selected: boolean }> }>
    return connections.find((connection) => connection.name === 'Box')?.connections?.find((connection) => connection.selected)?.id
  })
  expect(selectedAfter).toBe(selectedBefore)

  await page.setViewportSize({ width: 390, height: 844 })
  expect(await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)).toBeLessThanOrEqual(1)
  const mobileShellGeometry = await page.locator('box-app-shell').evaluate((shell) => {
    const bounds = (element: Element) => {
      const rect = element.getBoundingClientRect()
      return { left: rect.left, right: rect.right, width: rect.width }
    }
    const frame = shell.shadowRoot!.querySelector<HTMLElement>('[part="frame"]')!
    const navigation = shell.shadowRoot!.querySelector<HTMLElement>('[part="nav"]')!
    const main = shell.shadowRoot!.querySelector<HTMLElement>('[part="main"]')!
    const workspace = shell.querySelector<HTMLElement>('.workspace')!
    const pageContent = shell.querySelector<HTMLElement>('.compare-page')!
    const content = bounds(pageContent)
    return {
      viewportWidth: window.innerWidth,
      frame: bounds(frame),
      navigation: bounds(navigation),
      main: bounds(main),
      workspace: bounds(workspace),
      leftInset: content.left,
      rightInset: window.innerWidth - content.right,
    }
  })
  expect(mobileShellGeometry.frame).toEqual({ left: 0, right: 390, width: 390 })
  expect(mobileShellGeometry.navigation).toEqual(mobileShellGeometry.frame)
  expect(mobileShellGeometry.main).toEqual(mobileShellGeometry.frame)
  expect(mobileShellGeometry.workspace).toEqual(mobileShellGeometry.frame)
  expect(mobileShellGeometry.leftInset).toBeGreaterThanOrEqual(16)
  expect(Math.abs(mobileShellGeometry.leftInset - mobileShellGeometry.rightInset)).toBeLessThan(0.01)
  await expect(comparisonAgentDiff).toHaveAttribute('mode', 'inline')
  const mobileDiffGeometry = await comparisonAgentDiff.evaluate((viewer) => {
    const wrapper = viewer.parentElement!.getBoundingClientRect()
    const bounds = viewer.getBoundingClientRect()
    return {
      wrapperWidth: wrapper.width,
      viewerWidth: bounds.width,
      viewerRight: bounds.right,
      viewportWidth: window.innerWidth,
    }
  })
  expect(mobileDiffGeometry.viewerWidth).toBeLessThanOrEqual(mobileDiffGeometry.wrapperWidth + 1)
  expect(mobileDiffGeometry.viewerRight).toBeLessThanOrEqual(mobileDiffGeometry.viewportWidth)

  await page.setViewportSize({ width: 1440, height: 1000 })
  await expect(comparisonAgentDiff).toHaveAttribute('mode', 'split')
  await expect(page.getByText('0 of 1 selected')).toBeVisible()
  await expect(page.locator('box-button[label="Prepare deployment"]')).toHaveAttribute('disabled', '')
  await page.locator('box-icon-button[icon="plus"][label="Add Legal Assistant to deployment plan"]').click()
  await expect(page.getByText('1 of 1 selected')).toBeVisible()
  await expect(page.locator('box-icon-button[icon="cart-1"][label="Remove Legal Assistant from deployment plan"]')).toBeVisible()
  await expect(page.locator('box-button[label="Remove from plan"]')).toHaveCount(0)
  await page.locator('box-button[label="Prepare deployment"]').click()
  await expect(page).toHaveURL(/#workspace$/)
  await expect(page.getByRole('heading', { name: 'Confirm connections' })).toBeVisible()
  await expect(page.getByText('Promotion package prepared. Verify the destination connection, then review the deployment.')).toBeVisible()
  const selectedForPromotion = await page.evaluate(async () => {
    const connections = await fetch('/api/connections').then((response) => response.json()) as Array<{ name: string; connections?: Array<{ id: string; selected: boolean }> }>
    return connections.find((connection) => connection.name === 'Box')?.connections?.find((connection) => connection.selected)?.id
  })
  expect(selectedForPromotion).toBe('box-mock-sandbox')
  await page.locator('box-button[label="Continue to configure"]').click()
  await page.locator('box-button[label="Review plan"]').click()
  await page.locator('box-button[label="Validate deployment"]').click()
  await expect(page.locator('box-badge[label="Validation complete"]')).toBeVisible()
  await page.locator('box-button[label="Review changes"]').click()
  await expect(page.getByRole('heading', { name: 'Review validation changes' })).toBeVisible()
  await expect(page.getByText('1 file', { exact: true })).toBeVisible()
  const agentDiff = page.locator('box-diff-viewer[heading="ai-agents/legal-assistant.json"]')
  await expect(agentDiff).toBeVisible()
  await expect(agentDiff).toHaveAttribute('before-text', '')
  await expect(agentDiff).toHaveAttribute('after-text', /Legal Assistant/)
  await page.getByRole('button', { name: 'Close drawer' }).click()
  const restoredPlan = await page.request.put('/api/plan', { data: {
    name: 'Mock deployment', templateId: 'clm', template: 'Contract Lifecycle Management',
    repository: 'https://github.com/unofficialbox/box-bedrock-for-clm', strategy: 'reuse', components: ['box', 'salesforce'],
  } })
  expect(restoredPlan.ok()).toBe(true)
  const restoredSelection = await page.request.put('/api/connections/box/selection', { data: { id: selectedBefore } })
  expect(restoredSelection.ok()).toBe(true)
})
