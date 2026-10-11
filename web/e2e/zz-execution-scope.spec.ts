import { expect, test } from '@playwright/test'

for (const colorScheme of ['light', 'dark'] as const) {
  for (const width of [390, 1440]) {
    test(`guards a delayed deployment start and explains package scope at ${width}px in ${colorScheme}`, async ({ page, request }, testInfo) => {
      await page.setViewportSize({ width, height: 1000 })
      await page.emulateMedia({ colorScheme })
      await page.goto('/')
      await page.getByRole('button', { name: 'New deployment' }).click()
      const name = `Scope safeguard ${colorScheme} ${width}`
      await page.getByRole('textbox', { name: 'Name this deployment' }).fill(name)
      await page.getByRole('button', { name: 'Prepare package' }).click()
      await page.getByRole('button', { name: 'Continue to configure' }).click()
      await expect(page.locator('.component-scope')).toContainText('Individual component exclusions are not supported here.')
      await expect(page.locator('.component-scope box-switch')).toHaveCount(0)
      await expect(page.locator('box-switch[description="Required"]').getByRole('switch')).toBeDisabled()
      await expect(page.locator('box-switch[description="Optional"]').getByRole('switch')).toBeEnabled()
      await page.getByRole('button', { name: 'Review plan' }).click()
      await expect(page.getByText('Prepared package', { exact: true })).toBeVisible()
      await page.getByRole('button', { name: 'Validate deployment' }).click()
      await page.getByRole('button', { name: 'Continue to deployment' }).waitFor()
      let releaseResponse!: () => void
      const holdResponse = new Promise<void>((resolve) => { releaseResponse = resolve })
      let responseBody: { id: string } | undefined
      let deploymentURL = ''
      let requests = 0
      await page.route('**/api/runs/*/deploy', async (route) => {
        requests += 1
        deploymentURL = route.request().url()
        const response = await route.fetch()
        responseBody = await response.json()
        await holdResponse
        await route.fulfill({ response })
      })
      await page.getByRole('button', { name: 'Continue to deployment' }).click()
      await page.getByRole('button', { name: 'Start deployment' }).click()
      try {
        await expect(page.getByRole('button', { name: 'Starting deployment…' })).toBeDisabled()
        await expect(page.getByRole('dialog')).toHaveCount(0)
        await expect.poll(() => responseBody?.id).toBeTruthy()
        const replay = await request.post(deploymentURL)
        expect(replay.status()).toBe(202)
        expect((await replay.json()).id).toBe(responseBody!.id)
        await page.screenshot({ path: testInfo.outputPath('pending-start.png'), fullPage: true })
        expect(requests).toBe(1)
      } finally {
        releaseResponse()
      }
      await expect(page.getByRole('heading', { name: `${name} is ready` })).toBeVisible()
      const runs = await (await request.get('/api/runs')).json() as { deployment: string; action: string }[]
      expect(runs.filter((run) => run.deployment === name && run.action === 'deploy')).toHaveLength(1)
      await page.screenshot({ path: testInfo.outputPath('completed.png'), fullPage: true })
    })
  }
}
