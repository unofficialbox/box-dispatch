import { expect, test } from '@playwright/test'

test('configures, validates, and deploys against the mock backend', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByRole('heading', { name: 'Overview' })).toBeVisible()

  await page.getByRole('button', { name: 'New deployment' }).click()
  await expect(page.getByRole('heading', { name: 'Choose a solution' })).toBeVisible()
  await page.getByRole('button', { name: 'Next' }).click()

  await expect(page.getByRole('heading', { name: 'Connect systems' })).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Box', exact: true })).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Salesforce', exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Next' }).click()

  await expect(page.getByRole('heading', { name: 'Configure deployment' })).toBeVisible()
  await page.getByRole('button', { name: 'Next' }).click()

  await expect(page.getByRole('heading', { name: 'Ready to validate' })).toBeVisible()
  await page.getByRole('button', { name: 'Start validation' }).click()
  await expect(page.getByText('All selected systems finished successfully.')).toBeVisible()
  await page.getByRole('button', { name: 'Apply validated changes' }).click()

  await expect(page.getByRole('heading', { name: 'Deployment' })).toBeVisible()
  await expect(page.getByText('All selected systems finished successfully.')).toBeVisible()
  await expect(page.getByText('2 of 2 systems complete')).toBeVisible()
})
