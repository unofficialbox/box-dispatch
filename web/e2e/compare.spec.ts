import { expect, test } from '@playwright/test'

test('compares two Box enterprises live without changing the deployment selection', async ({ page }) => {
  let comparisonRequests = 0
  let folderComparisonRequests = 0
  let aiAgentComparisonRequests = 0
  page.on('request', (request) => {
    if (request.method() === 'POST' && request.url().endsWith('/api/box/comparisons')) comparisonRequests += 1
    if (request.method() === 'POST' && request.url().endsWith('/api/box/comparisons/folders')) folderComparisonRequests += 1
    if (request.method() === 'POST' && request.url().endsWith('/api/box/comparisons/ai-agents')) aiAgentComparisonRequests += 1
  })
  await page.goto('/')
  const initialSelection = await page.request.put('/api/connections/box/selection', { data: { id: 'box-mock-production' } })
  expect(initialSelection.ok()).toBe(true)
  await page.reload()
  const selectedBefore = await page.evaluate(async () => {
    const connections = await fetch('/api/connections').then((response) => response.json()) as Array<{ name: string; connections?: Array<{ id: string; selected: boolean }> }>
    return connections.find((connection) => connection.name === 'Box')?.connections?.find((connection) => connection.selected)?.id
  })

  await page.getByRole('link', { name: 'Compare Box environments' }).click()
  await expect(page.getByRole('heading', { name: 'Compare Box environments' })).toBeVisible()
  await expect(page.locator('box-select[label="Source Box connection"]')).toHaveAttribute('value', 'box-mock-production')
  await expect(page.locator('box-select[label="Destination Box connection"]')).toHaveAttribute('value', 'box-mock-sandbox')
  await page.locator('box-button[label="Run live comparison"]').click()

  await expect(page.getByRole('heading', { name: 'Metadata templates' })).toBeVisible()
  await expect(page.locator('box-metric-card[heading="Changed"]')).toHaveAttribute('value', '1')
  await expect(page.locator('box-metric-card[heading="Source only"]')).toHaveAttribute('value', '1')
  await expect(page.locator('box-metric-card[heading="Destination only"]')).toHaveAttribute('value', '1')
  const diff = page.locator('box-diff-viewer[heading="contracts.json"]')
  await expect(diff).toBeVisible()
  await expect(diff).toHaveAttribute('before-label', 'Acme Production')
  await expect(diff).toHaveAttribute('after-label', 'Acme Sandbox')
  await expect(page.getByText('fields.status.options')).toBeVisible()
  await expect(page.getByText('Enterprise EID-mock', { exact: false })).toBeVisible()
  await expect(page.getByText('Enterprise EID-sandbox', { exact: false })).toBeVisible()

  await page.locator('box-button[label="Run live comparison"]').click()
  await expect.poll(() => comparisonRequests).toBe(2)

  await page.locator('box-select[label="Box object type"]').evaluate((element) => {
    element.setAttribute('value', 'folders')
    element.dispatchEvent(new CustomEvent('value-changed', { bubbles: true, composed: true, detail: { value: 'folders' } }))
  })
  await expect(page.getByText(/All Files hierarchy to 4 levels/)).toBeVisible()
  await page.locator('box-button[label="Run live comparison"]').click()
  await expect(page.getByRole('heading', { name: 'Folders', exact: true })).toBeVisible()
  await expect(page.locator('box-metric-card[heading="Changed"]')).toHaveAttribute('value', '1')
  await expect(page.locator('box-metric-card[heading="Source only"]')).toHaveAttribute('value', '2')
  await expect(page.locator('box-metric-card[heading="Destination only"]')).toHaveAttribute('value', '2')
  await expect(page.locator('box-diff-viewer[heading="folder-structure.json"]')).toBeVisible()
  await expect(page.getByText('items.files')).toBeVisible()
  await expect.poll(() => folderComparisonRequests).toBe(1)

  await page.locator('box-select[label="Box object type"]').evaluate((element) => {
    element.setAttribute('value', 'ai_agents')
    element.dispatchEvent(new CustomEvent('value-changed', { bubbles: true, composed: true, detail: { value: 'ai_agents' } }))
  })
  await expect(page.getByText(/current custom AI Studio agents/)).toBeVisible()
  await page.locator('box-button[label="Run live comparison"]').click()
  await expect(page.getByRole('heading', { name: 'AI agents and extraction configurations' })).toBeVisible()
  await expect(page.locator('box-metric-card[heading="Changed"]')).toHaveAttribute('value', '1')
  await expect(page.locator('box-metric-card[heading="Source only"]')).toHaveAttribute('value', '1')
  await expect(page.locator('box-metric-card[heading="Destination only"]')).toHaveAttribute('value', '1')
  await expect(page.locator('box-diff-viewer[heading="contract-extractor.json"]')).toBeVisible()
  await expect(page.getByText('configuration.extract.long_text.model')).toBeVisible()
  await expect(page.getByText('Extraction configuration', { exact: true })).toBeVisible()
  await expect.poll(() => aiAgentComparisonRequests).toBe(1)

  const selectedAfter = await page.evaluate(async () => {
    const connections = await fetch('/api/connections').then((response) => response.json()) as Array<{ name: string; connections?: Array<{ id: string; selected: boolean }> }>
    return connections.find((connection) => connection.name === 'Box')?.connections?.find((connection) => connection.selected)?.id
  })
  expect(selectedAfter).toBe(selectedBefore)

  await page.setViewportSize({ width: 390, height: 844 })
  expect(await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)).toBeLessThanOrEqual(1)

  await page.setViewportSize({ width: 1440, height: 1000 })
  await expect(page.getByText('1 of 1 deployable agents selected for Acme Sandbox.')).toBeVisible()
  const removeFromPlan = page.locator('box-icon-button[icon="cart-1"][label="Remove Legal Assistant from deployment plan"]')
  await expect(removeFromPlan).toBeVisible()
  await expect(page.locator('box-button[label="Remove from plan"]')).toHaveCount(0)
  await removeFromPlan.click()
  await expect(page.getByText('0 of 1 deployable agents selected for Acme Sandbox.')).toBeVisible()
  await expect(page.locator('box-button[label="Prepare deployment"]')).toHaveAttribute('disabled', '')
  await page.locator('box-button[label="Add to plan"]').click()
  await expect(page.getByText('1 of 1 deployable agents selected for Acme Sandbox.')).toBeVisible()
  await page.locator('box-button[label="Prepare deployment"]').click()
  await expect(page).toHaveURL(/#workspace$/)
  await expect(page.getByRole('heading', { name: 'Confirm connections' })).toBeVisible()
  await expect(page.getByText('AI agent promotion package prepared. Verify the destination connection, then review the deployment.')).toBeVisible()
  const selectedForPromotion = await page.evaluate(async () => {
    const connections = await fetch('/api/connections').then((response) => response.json()) as Array<{ name: string; connections?: Array<{ id: string; selected: boolean }> }>
    return connections.find((connection) => connection.name === 'Box')?.connections?.find((connection) => connection.selected)?.id
  })
  expect(selectedForPromotion).toBe('box-mock-sandbox')
  const restoredPlan = await page.request.put('/api/plan', { data: {
    name: 'Mock deployment', templateId: 'clm', template: 'Contract Lifecycle Management',
    repository: 'https://github.com/unofficialbox/box-bedrock-for-clm', strategy: 'reuse', components: ['box', 'salesforce'],
  } })
  expect(restoredPlan.ok()).toBe(true)
  const restoredSelection = await page.request.put('/api/connections/box/selection', { data: { id: selectedBefore } })
  expect(restoredSelection.ok()).toBe(true)
})
