import { expect, test } from '@playwright/test'

test('compares two Box enterprises live without changing the deployment selection', async ({ page }) => {
  let comparisonRequests = 0
  page.on('request', (request) => {
    if (request.method() === 'POST' && request.url().endsWith('/api/box/comparisons')) comparisonRequests += 1
  })
  await page.goto('/')
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
  const selectedAfter = await page.evaluate(async () => {
    const connections = await fetch('/api/connections').then((response) => response.json()) as Array<{ name: string; connections?: Array<{ id: string; selected: boolean }> }>
    return connections.find((connection) => connection.name === 'Box')?.connections?.find((connection) => connection.selected)?.id
  })
  expect(selectedAfter).toBe(selectedBefore)

  await page.setViewportSize({ width: 390, height: 844 })
  expect(await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)).toBeLessThanOrEqual(1)
})
