import { expect, test } from '@playwright/test'

test('promotes a selected live folder node through validation, deployment, and history', async ({ page }) => {
  const originalPlan = {
    name: 'Mock deployment', templateId: 'clm', template: 'Contract Lifecycle Management',
    repository: 'https://github.com/unofficialbox/box-bedrock-for-clm', strategy: 'reuse', components: ['box', 'salesforce'],
  }
  let promotionBody: Record<string, unknown> | undefined
  page.on('request', (request) => {
    if (request.method() === 'POST' && request.url().endsWith('/api/box/promotions/folders')) {
      promotionBody = request.postDataJSON() as Record<string, unknown>
    }
  })

  try {
    await page.goto('/#compare')
    await page.locator('box-select[label="Box object type"]').evaluate((element) => {
      element.setAttribute('value', 'folders')
      element.dispatchEvent(new CustomEvent('value-changed', { bubbles: true, composed: true, detail: { value: 'folders' } }))
    })
    await page.locator('box-button[label="Run live comparison"]').click()

    const tree = page.locator('box-folder-tree-diff')
    await expect(tree).toBeVisible()
    await expect(page.getByText('0 of 5 selected')).toBeVisible()
    await tree.evaluate((element) => {
      const node = element.nodes.find((candidate) => candidate.id === 'folder:/Legal')!
      element.dispatchEvent(new CustomEvent('selected-changed', {
        bubbles: true, composed: true, detail: { id: node.id, node, selectedIds: [node.id] },
      }))
    })
    await expect(page.getByText('1 of 5 selected')).toBeVisible()
    await page.locator('box-text-field[label="Deployment name"]').evaluate((element) => {
      element.setAttribute('value', 'Folder QA promotion')
      element.dispatchEvent(new CustomEvent('value-changed', { bubbles: true, composed: true, detail: { value: 'Folder QA promotion' } }))
    })
    await page.locator('box-button[label="Prepare deployment"]').click()

    await expect(page).toHaveURL(/#workspace$/)
    expect(promotionBody).toEqual({
      name: 'Folder QA promotion', leftConnectionId: 'box-mock-production', rightConnectionId: 'box-mock-sandbox',
      leftRootId: '0', rightRootId: '0', nodes: [{ path: '/Legal', type: 'folder' }],
    })
    await expect(page.getByRole('heading', { name: 'Confirm connections' })).toBeVisible()
    await page.locator('box-button[label="Continue to configure"]').click()
    await page.locator('box-button[label="Review plan"]').click()
    await page.locator('box-button[label="Validate deployment"]').click()
    await expect(page.locator('box-badge[label="Validation complete"]')).toBeVisible()
    await page.locator('box-button[label="Review changes"]').click()
    const validationDiff = page.locator('box-diff-viewer[heading="folder-promotion/Legal.json"]')
    await expect(validationDiff).toBeVisible()
    await expect(validationDiff).toHaveAttribute('after-text', /"path": "\/Legal"/)
    await expect(validationDiff).toHaveAttribute('after-text', /"type": "folder"/)
    await expect(validationDiff).not.toHaveAttribute('after-text', /source|url/i)
    await page.getByRole('button', { name: 'Close drawer' }).click()

    await page.getByRole('button', { name: 'Continue to deployment' }).click()
    await page.getByRole('button', { name: 'Start deployment' }).click()
    await expect(page.getByRole('heading', { name: 'Folder QA promotion is ready' })).toBeVisible()
    await page.getByRole('button', { name: 'Review changes' }).click()
    await expect(page.locator('box-diff-viewer[heading="folder-promotion/Legal.json"]')).toHaveAttribute('after-text', /"path": "\/Legal"/)
    await page.getByRole('button', { name: 'Close drawer' }).click()

    await page.getByRole('link', { name: 'Overview', exact: true }).click()
    const recent = page.getByRole('table', { name: 'Recent deployments' })
    await recent.getByRole('link', { name: 'Folder QA promotion' }).click()
    await expect(page.getByRole('heading', { name: 'Folder QA promotion' })).toBeVisible()
    await expect(page.getByText('Folder:/Legal', { exact: true })).toBeVisible()
    await page.getByRole('button', { name: 'Review changes' }).click()
    await expect(page.locator('box-diff-viewer[heading="folder-promotion/Legal.json"]')).toHaveAttribute('after-text', /"type": "folder"/)
  } finally {
    await page.request.put('/api/plan', { data: originalPlan })
    await page.request.put('/api/connections/box/selection', { data: { id: 'box-mock-production' } })
  }
})

