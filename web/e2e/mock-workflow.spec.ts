import { expect, test } from '@playwright/test'

test('configures, validates, and deploys against the mock backend', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByRole('heading', { name: 'Overview' })).toBeVisible()

  await page.getByRole('button', { name: 'New deployment' }).click()
  await expect(page.getByRole('heading', { name: 'Choose a solution' })).toBeVisible()
  await page.getByRole('button', { name: 'Prepare package' }).click()

  await expect(page.getByRole('heading', { name: 'Confirm connections' })).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Box', exact: true })).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Salesforce', exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Continue to configure' }).click()

  await expect(page.getByRole('heading', { name: 'Configure deployment' })).toBeVisible()
  await page.getByRole('button', { name: 'Review plan' }).click()

  await expect(page.getByRole('heading', { name: 'Review and validate' })).toBeVisible()
  await page.getByRole('button', { name: 'Validate deployment' }).click()
  await expect(page.getByText('All selected systems finished successfully.')).toBeVisible()
  await page.getByRole('button', { name: 'Apply validated changes' }).click()

  await expect(page.getByRole('heading', { name: 'Deployment' })).toBeVisible()
  await expect(page.getByText('All selected systems finished successfully.')).toBeVisible()
  await expect(page.getByText('2 of 2 systems complete')).toBeVisible()
})

test('keeps the primary workflow usable on a mobile viewport', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto('/')
  await page.getByRole('button', { name: 'New deployment' }).click()

  await expect(page.getByRole('heading', { name: 'Choose a solution' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Prepare package' })).toBeVisible()
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)
  expect(overflow).toBeLessThanOrEqual(1)
})
