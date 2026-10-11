import { expect, test } from '@playwright/test'
import { resolve } from 'node:path'
import { writeFile } from 'node:fs/promises'

for (const colorScheme of ['light', 'dark'] as const) {
  for (const width of [390, 1440]) {
    test(`requires fresh validation after plan drift and guards active inputs at ${width}px in ${colorScheme}`, async ({ page, request }, info) => {
      await page.setViewportSize({ width, height: 1000 })
      await page.emulateMedia({ colorScheme })
      await page.goto('/')
      await page.getByRole('button', { name: 'New deployment' }).click()
      const original = `Context guard ${colorScheme} ${width}`
      const current = `${original} revised`
      await page.getByRole('textbox', { name: 'Name this deployment' }).fill(original)
      await page.getByRole('button', { name: 'Prepare package' }).click()
      await page.getByRole('button', { name: 'Continue to configure' }).click()
      await page.getByRole('button', { name: 'Review plan' }).click()
      await page.getByRole('button', { name: 'Validate deployment' }).click()
      await page.getByRole('button', { name: 'Continue to deployment' }).waitFor()
      const plan = await (await request.get('/api/plan')).json()
      const update = await request.put('/api/plan', { data: {
        name: current, templateId: plan.templateId, template: plan.template,
        repository: plan.repository, components: plan.components.map((component: { id: string }) => component.id), strategy: plan.strategy,
      } })
      expect(update.status()).toBe(200)
      await page.getByRole('button', { name: 'Continue to deployment' }).click()
      await page.getByRole('button', { name: 'Start deployment' }).click()
      await expect(page.getByText('The package or selected connection changed or could not be verified. Validate again before starting deployment.', { exact: true })).toBeVisible()
      await expect(page.getByRole('heading', { name: current, exact: true })).toBeVisible()
      await expect(page.getByRole('button', { name: 'Validate deployment' })).toBeEnabled()
      await expect(page.getByRole('button', { name: 'Continue to deployment' })).toHaveCount(0)
      const rejectedHistory = await (await request.get('/api/runs')).json() as { deployment: string; action: string }[]
      expect(rejectedHistory.filter((run) => run.deployment === original && run.action === 'deploy')).toHaveLength(0)
      await page.screenshot({ path: info.outputPath('stale-validation-review.png'), fullPage: true })
      await page.addScriptTag({ path: resolve('node_modules/axe-core/axe.min.js') })
      const accessibility = await page.evaluate(async () => {
        const axe = (window as unknown as { axe: { run: () => Promise<unknown> } }).axe
        return await axe.run()
      })
      const accessibilityPath = info.outputPath('stale-review-accessibility.json')
      await writeFile(accessibilityPath, JSON.stringify(accessibility, null, 2))
      await info.attach('stale-review-accessibility', { path: accessibilityPath, contentType: 'application/json' })
      const validate = page.getByRole('button', { name: 'Validate deployment' })
      await validate.focus()
      await expect(validate).toBeFocused()
      await validate.press('Enter')
      await page.getByRole('button', { name: 'Continue to deployment' }).waitFor()
      const connections = await (await request.get('/api/connections')).json() as { name: string; connections?: { id: string; selected?: boolean }[] }[]
      const options = connections.find((connection) => connection.name === 'Box')?.connections ?? []
      expect(options.length).toBeGreaterThan(0)
      let acceptedID = ''
      let release!: () => void
      const held = new Promise<void>((resolve) => { release = resolve })
      let checked = false
      await page.route('**/api/runs/*/deploy', async (route) => {
        const response = await route.fetch()
        expect(response.status()).toBe(202)
        acceptedID = (await response.json()).id
        const conflicting = await Promise.all([
          request.put('/api/plan', { data: { name: 'Forbidden change', templateId: plan.templateId, template: plan.template, repository: plan.repository, components: ['box', 'salesforce'], strategy: plan.strategy } }),
          request.post('/api/packages', { data: { name: 'Forbidden package', templateId: 'clm', components: ['box', 'salesforce'] } }),
          request.delete(`/api/connections/box/${(options.find((option) => option.selected) ?? options[0]).id}`),
          request.post('/api/connections/box/oauth/start'),
          request.post('/api/connections/salesforce/oauth/start', { data: {} }),
          request.post('/api/salesforce/scratch-orgs', { data: {} }),
          request.post('/api/runs'),
        ])
        for (const result of conflicting) {
          expect(result.status()).toBe(409)
          expect((await result.json()).error).toContain('Validation or deployment is active')
        }
        const replay = await request.post(route.request().url())
        expect(replay.status()).toBe(202)
        expect((await replay.json()).id).toBe(acceptedID)
        checked = true
        await held
        await route.fulfill({ response })
      })
      await page.getByRole('button', { name: 'Continue to deployment' }).click()
      await page.getByRole('button', { name: 'Start deployment' }).click()
      try {
        await expect.poll(() => checked).toBe(true)
        await expect(page.getByRole('button', { name: 'Starting deployment…' })).toBeDisabled()
        expect((await (await request.get('/api/plan')).json()).name).toBe(current)
        await page.screenshot({ path: info.outputPath('active-context-lease.png'), fullPage: true })
      } finally { release() }
      await expect(page.getByRole('heading', { name: `${current} is ready` })).toBeVisible()
      const history = await (await request.get('/api/runs')).json() as { id: string; deployment: string; action: string }[]
      expect(history.filter((run) => run.deployment === current && run.action === 'deploy').map((run) => run.id)).toEqual([acceptedID])
      const overflow = await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth)
      expect(overflow).toBe(false)
      await page.screenshot({ path: info.outputPath('fresh-validation-completed.png'), fullPage: true })
    })
  }
}


test('stale validation recovers when the current plan reload fails', async ({ page, request }, info) => {
  await page.goto('/')
  await page.getByRole('button', { name: 'New deployment' }).click()
  const name = 'Stale plan reload recovery'
  await page.getByRole('textbox', { name: 'Name this deployment' }).fill(name)
  await page.getByRole('button', { name: 'Prepare package' }).click()
  await page.getByRole('button', { name: 'Continue to configure' }).click()
  await page.getByRole('button', { name: 'Review plan' }).click()
  await page.getByRole('button', { name: 'Validate deployment' }).click()
  await page.getByRole('button', { name: 'Continue to deployment' }).waitFor()
  const plan = await (await request.get('/api/plan')).json()
  const update = await request.put('/api/plan', { data: { name, templateId: plan.templateId, template: plan.template, repository: plan.repository, components: plan.components.map((component: { id: string }) => component.id), strategy: plan.strategy } })
  expect(update.status()).toBe(200)
  await page.route('**/api/plan', (route) => route.fulfill({ status: 503, contentType: 'application/json', body: JSON.stringify({ error: 'Plan unavailable' }) }))
  await page.getByRole('button', { name: 'Continue to deployment' }).click()
  await page.getByRole('button', { name: 'Start deployment' }).click()
  await expect(page.getByRole('button', { name: 'Retry loading' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Continue to deployment' })).toHaveCount(0)
  await page.screenshot({ path: info.outputPath('stale-plan-reload-failed.png'), fullPage: true })
  await page.unroute('**/api/plan')
  await page.getByRole('button', { name: 'Retry loading' }).click()
  await expect(page.getByRole('button', { name: 'Validate deployment' })).toBeEnabled()
  await expect(page.getByRole('button', { name: 'Continue to deployment' })).toHaveCount(0)
  await page.getByRole('button', { name: 'Validate deployment' }).click()
  await page.getByRole('button', { name: 'Continue to deployment' }).waitFor()
  await page.screenshot({ path: info.outputPath('fresh-validation-after-reload-recovery.png'), fullPage: true })
})
