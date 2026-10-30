import { expect, test, type Page } from '@playwright/test'
import axe from 'axe-core'

// Scoped audits retain all violations and incompletes, plus color-contrast passes.
// Full-page review evidence covers residuals outside these contexts.
async function expectAccessible(page: Page, selectors: (string | string[])[], rules: string[]) {
  await page.addScriptTag({ content: axe.source })
  const audit = await page.evaluate(async ({ selectors, rules }) => {
    const result = await (window as Window & { axe: typeof axe }).axe.run(
      { include: selectors },
      { runOnly: { type: 'rule', values: rules } },
    )
    const summarize = (results: typeof result.violations) => results.map(({ id, nodes }) => ({
      id,
      nodes: nodes.map(({ target, failureSummary, any, all, none }) => ({
        target,
        failureSummary,
        checks: [...any, ...all, ...none].map(({ id, data, message }) => ({ id, data, message })),
      })),
    }))
    return {
      violations: summarize(result.violations),
      incomplete: summarize(result.incomplete),
      contrastPasses: summarize(result.passes.filter(({ id }) => id === 'color-contrast')),
    }
  }, { selectors, rules })
  await test.info().attach('owned-accessibility-audit', {
    body: JSON.stringify({ selectors, rules, ...audit }, null, 2),
    contentType: 'application/json',
  })
  expect(audit.violations).toEqual([])
  if (rules.includes('color-contrast')) {
    // These contexts contain visible owned copy with known theme backgrounds.
    // An indeterminate contrast result must be assessed rather than passing.
    expect(audit.incomplete.filter(({ id }) => id === 'color-contrast')).toEqual([])
    expect(audit.contrastPasses.flatMap(({ nodes }) => nodes).length).toBeGreaterThan(0)
  }
}

// Settle native theme transitions before assessing the rendered label colors.
async function expectSharedAccessibilityInBothThemes(page: Page, selectors: (string | string[])[], rules = ['color-contrast', 'label-content-name-mismatch']) {
  for (const colorScheme of ['light', 'dark'] as const) {
    await page.emulateMedia({ colorScheme })
    await expect(page.locator('html')).toHaveAttribute('data-theme', colorScheme)
    const upcomingStage = page.locator('box-path').locator('[part="stage"][data-state="upcoming"]').first()
    if (await upcomingStage.count()) {
      await expect(upcomingStage).toHaveCSS('background-color', colorScheme === 'light' ? 'rgb(244, 244, 244)' : 'rgb(29, 33, 31)')
    }
    await expectAccessible(page, selectors, rules)
  }
}

async function expectDiffGutterContrast(page: Page) {
  for (const colorScheme of ['light', 'dark'] as const) {
    await page.emulateMedia({ colorScheme })
    await expect(page.locator('html')).toHaveAttribute('data-theme', colorScheme)
    const measurements = await page.locator('box-diff-viewer').evaluate((viewer) => {
      const rgb = (color: string) => {
        const match = color.match(/^rgb\((\d+), (\d+), (\d+)\)$/)
        if (!match) throw new Error(`Expected opaque RGB gutter color: ${color}`)
        return match.slice(1).map(Number)
      }
      const luminance = (color: number[]) => color.map((value) => {
        const channel = value / 255
        return channel <= 0.04045 ? channel / 12.92 : ((channel + 0.055) / 1.055) ** 2.4
      }).reduce((sum, value, index) => sum + value * [0.2126, 0.7152, 0.0722][index], 0)
      const background = getComputedStyle(viewer.shadowRoot!.querySelector('[part="panel"]')!).backgroundColor
      return [...viewer.shadowRoot!.querySelectorAll<HTMLElement>('[part="line-number"]')].filter((cell) => cell.textContent?.trim()).map((cell) => {
        const style = getComputedStyle(cell)
        // Numeric-only content can be inapplicable to axe's text heuristic.
        // Measure the visible native text instead of accepting an empty audit.
        const values = [luminance(rgb(style.color)), luminance(rgb(background))].sort((a, b) => a - b)
        return { text: cell.textContent, color: style.color, background, opacity: style.opacity, visible: cell.checkVisibility({ checkOpacity: true, checkVisibilityCSS: true }), ratio: (values[1] + 0.05) / (values[0] + 0.05) }
      })
    })
    await test.info().attach(`diff-gutter-${colorScheme}`, { body: JSON.stringify(measurements, null, 2), contentType: 'application/json' })
    expect(measurements.length).toBeGreaterThan(0)
    expect(measurements.every(({ visible, opacity, ratio }) => visible && opacity === '1' && ratio >= 4.5)).toBe(true)
  }
}

async function expectStackedWorkspaceRail(page: Page, workspaceSelector: string, primarySelector: string, expectFlexiblePrimary = false) {
  await page.setViewportSize({ width: 1024, height: 768 })
  const layout = await page.locator(workspaceSelector).evaluate((workspace, selector) => {
    const primary = workspace.querySelector<HTMLElement>(selector)!.getBoundingClientRect()
    const rail = workspace.querySelector<HTMLElement>('.details-rail')!.getBoundingClientRect()
    return {
      pageOverflow: document.documentElement.scrollWidth - document.documentElement.clientWidth,
      railBelowPrimary: rail.top >= primary.bottom - 1,
      leftEdgeDelta: Math.abs(rail.left - primary.left),
      rightEdgeDelta: Math.abs(rail.right - primary.right),
      primaryMinHeight: getComputedStyle(workspace.querySelector<HTMLElement>(selector)!).minHeight,
    }
  }, primarySelector)
  expect(layout.pageOverflow).toBeLessThanOrEqual(1)
  expect(layout.railBelowPrimary).toBe(true)
  expect(layout.leftEdgeDelta).toBeLessThan(1)
  expect(layout.rightEdgeDelta).toBeLessThan(1)
  if (expectFlexiblePrimary) expect(layout.primaryMinHeight).toBe('0px')
  await page.setViewportSize({ width: 1280, height: 720 })
}

test('follows the system color preference across primary pages', async ({ page }) => {
  await page.emulateMedia({ colorScheme: 'dark' })
  await page.goto('/')

  const root = page.locator('html')
  await expect(root).toHaveAttribute('data-theme', 'dark')
  expect(await page.evaluate(() => ({
    page: getComputedStyle(document.querySelector('.workspace')!).backgroundColor,
    heading: getComputedStyle(document.querySelector('.overview-heading h1')!).color,
    token: getComputedStyle(document.documentElement).getPropertyValue('--boe-token-surface-surface').trim(),
  }))).toEqual({ page: 'rgb(18, 21, 19)', heading: 'rgb(244, 244, 244)', token: '#121513' })
  await expect(page.locator('.overview-heading h1')).toHaveCSS('color', 'rgb(244, 244, 244)')
  await expectAccessible(page, ['.overview-heading h1'], ['color-contrast'])

  await page.getByRole('link', { name: 'Settings', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Settings' })).toBeVisible()
  expect(await page.locator('.settings-provider').first().evaluate((panel) => ({
    border: getComputedStyle(panel).borderTopColor,
    header: getComputedStyle(panel.querySelector('header')!).backgroundColor,
  }))).toEqual({ border: 'rgb(52, 58, 55)', header: 'rgb(12, 15, 13)' })

  await page.getByRole('link', { name: 'Deployments', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Choose a solution' })).toBeVisible()
  await expect(root).toHaveAttribute('data-theme', 'dark')

  await page.getByRole('link', { name: 'Deployment history', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Deployment history' })).toBeVisible()
  await page.setViewportSize({ width: 390, height: 844 })
  expect(await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)).toBeLessThanOrEqual(1)

  await page.emulateMedia({ colorScheme: 'light' })
  await expect(root).toHaveAttribute('data-theme', 'light')
  expect(await page.evaluate(() => ({
    page: getComputedStyle(document.querySelector('.workspace')!).backgroundColor,
    token: getComputedStyle(document.documentElement).getPropertyValue('--boe-token-surface-surface').trim(),
  }))).toEqual({ page: 'rgb(255, 255, 255)', token: '#ffffff' })
  await page.getByRole('link', { name: 'Overview', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Overview' })).toBeVisible()
  await expectAccessible(page, ['.overview-heading h1'], ['color-contrast'])
})

test('configures, validates, and deploys against the mock backend', async ({ page }) => {
  await page.emulateMedia({ colorScheme: 'dark' })
  const connectionChecks: string[] = []
  page.on('request', (request) => {
    if (request.method() === 'POST' && /\/api\/connections\/(box|salesforce)\/check$/.test(request.url())) connectionChecks.push(request.url())
  })
  await page.goto('/')
  await expect(page.getByRole('heading', { name: 'Overview' })).toBeVisible()
  await expect(page.locator('box-metric-card[heading="Connections"]')).not.toHaveAttribute('value', 'Checking')
  await expect(page.getByRole('link', { name: 'Overview', exact: true })).toHaveAttribute('aria-current', 'page')
  await expect(page.getByRole('link', { name: 'Dispatch Solution delivery overview', exact: true })).toBeVisible()
  await expect(page.locator('box-sidebar-toggle-button').getByRole('button', { name: 'Collapse navigation' })).not.toHaveAttribute('aria-controls')
  await expectAccessible(page, ['.brand', 'box-sidebar-toggle-button'], ['label-content-name-mismatch', 'aria-valid-attr-value'])
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
    backgroundColor: getComputedStyle(row).backgroundColor,
    beforeContent: getComputedStyle(row, '::before').content,
    boxShadow: getComputedStyle(row).boxShadow,
  }))
  expect(activeNavigationStyle.backgroundColor).toBe('rgb(48, 51, 48)')
  expect(activeNavigationStyle.beforeContent).toBe('none')
  expect(activeNavigationStyle.boxShadow).not.toContain('3px 0px')
  await page.locator('box-sidebar-toggle-button').getByRole('button', { name: 'Collapse navigation' }).click()
  await expect(sidebar).toHaveAttribute('collapsed', '')
  await expect.poll(() => sidebar.evaluate((element) => element.getBoundingClientRect().width)).toBeLessThan(expandedWidth)
  await page.locator('box-sidebar-toggle-button').getByRole('button', { name: 'Expand navigation' }).click()
  await expect(sidebar).not.toHaveAttribute('collapsed', '')
  await expect(page.getByText('acme.app.box.com')).toBeVisible()
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
    const providerRail = document.querySelector('.overview-secondary')!.getBoundingClientRect()
    const history = document.querySelector('.overview-history-table')!.getBoundingClientRect()
    return {
      historyToDivider: providerRail.left - history.right,
    }
  })
  expect(overviewGutter.historyToDivider).toBeGreaterThanOrEqual(29)

  await page.getByRole('button', { name: 'New deployment' }).click()
  await expect(page.getByRole('heading', { name: 'Choose a solution' })).toBeVisible()
  const workflow = page.locator('box-path')
  await expect(workflow).toHaveAttribute('current', 'Choose')
  await expect(workflow).toHaveAttribute('variant', 'chevron')
  await expect(workflow.locator('[part="stage"][data-stage-id="Deploy"]')).toHaveAttribute('data-state', 'upcoming')
  await expect(workflow.locator('[part="stage"][data-stage-id="Summary"]')).toHaveAttribute('data-state', 'upcoming')
  const solutionGroup = page.locator('box-tile-group.solution-list')
  const selectedSolutionBorder = await solutionGroup.evaluate((group) => {
    const selected = group.shadowRoot!.querySelector<HTMLElement>('[part="tile"][data-selected="true"]')!
    const unselected = group.shadowRoot!.querySelector<HTMLElement>('[part="tile"][data-selected="false"]')!
    const selectedStyle = getComputedStyle(selected)
    const unselectedStyle = getComputedStyle(unselected)
    return {
      selectedID: selected.dataset.optionId,
      selectedColors: [selectedStyle.borderTopColor, selectedStyle.borderRightColor, selectedStyle.borderBottomColor, selectedStyle.borderLeftColor],
      selectedWidths: [selectedStyle.borderTopWidth, selectedStyle.borderRightWidth, selectedStyle.borderBottomWidth, selectedStyle.borderLeftWidth],
      unselectedColor: unselectedStyle.borderTopColor,
    }
  })
  expect(selectedSolutionBorder.selectedWidths).toEqual(['1px', '1px', '1px', '1px'])
  expect(new Set(selectedSolutionBorder.selectedColors).size).toBe(1)
  expect(selectedSolutionBorder.selectedColors[0]).not.toBe(selectedSolutionBorder.unselectedColor)
  await solutionGroup.evaluate((group) => group.shadowRoot!.querySelector<HTMLInputElement>('[part="tile"][data-selected="false"] [part="control"]')!.click())
  await expect.poll(() => solutionGroup.evaluate((group) => group.shadowRoot!.querySelector<HTMLElement>('[part="tile"][data-selected="true"]')?.dataset.optionId)).not.toBe(selectedSolutionBorder.selectedID)
  await solutionGroup.evaluate((group, selectedID) => group.shadowRoot!.querySelector<HTMLInputElement>(`[part="tile"][data-option-id="${selectedID}"] [part="control"]`)!.click(), selectedSolutionBorder.selectedID)
  await expect.poll(() => solutionGroup.evaluate((group) => group.shadowRoot!.querySelector<HTMLElement>('[part="tile"][data-selected="true"]')?.dataset.optionId)).toBe(selectedSolutionBorder.selectedID)
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
  const salesforceSource = page.locator('box-switch[label="Salesforce"]')
  await expect(salesforceSource).not.toHaveAttribute('checked')
  await salesforceSource.evaluate((control) => control.dispatchEvent(new CustomEvent('checked-changed', { detail: { checked: true } })))
  await expect(salesforceSource).toHaveAttribute('checked', '')
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
  await page.setViewportSize({ width: 1024, height: 768 })
  const narrowConnectionLayout = await page.evaluate(() => {
    const primary = document.querySelector<HTMLElement>('.connect-workspace [slot="primary"]')!.getBoundingClientRect()
    const rail = document.querySelector<HTMLElement>('.connect-workspace .details-rail')!.getBoundingClientRect()
    const actions = [...document.querySelectorAll<HTMLElement>('.connect-workspace .connection-buttons box-button')].map((button) => button.getBoundingClientRect())
    const breadcrumbLinks = [...document.querySelector('box-breadcrumb')!.shadowRoot!.querySelectorAll<HTMLElement>('[part="link"]')]
    const stages = [...document.querySelector('box-path')!.shadowRoot!.querySelectorAll<HTMLElement>('[part="stage"]')]
    const path = document.querySelector('box-path')!.getBoundingClientRect()
    return {
      railBelowPrimary: rail.top >= primary.bottom - 1,
      actionsInsidePrimary: actions.every((action) => action.left >= primary.left - 1 && action.right <= primary.right + 1),
      breadcrumbClipped: breadcrumbLinks.some((link) => link.scrollWidth > link.clientWidth + 1),
      stageClipped: stages.some((stage) => stage.getBoundingClientRect().left < path.left - 1 || stage.getBoundingClientRect().right > path.right + 1),
    }
  })
  expect(narrowConnectionLayout).toEqual({ railBelowPrimary: true, actionsInsidePrimary: true, breadcrumbClipped: false, stageClipped: false })
  await page.setViewportSize({ width: 1280, height: 720 })
  await expectSharedAccessibilityInBothThemes(page, ['box-resource-row', 'box-badge', ['box-path', '[part="stage-label"]']])
  await page.getByRole('button', { name: 'Continue to configure' }).click()

  await expect(page.getByRole('heading', { name: 'Configure deployment' })).toBeVisible()
  await expectStackedWorkspaceRail(page, '.configure-workspace', '[slot="primary"]')
  const requiredSwitch = page.locator('box-switch[description="Required"]').getByRole('switch')
  const optionalSwitch = page.locator('box-switch[description="Optional"]').getByRole('switch')
  await expect(requiredSwitch).toHaveAccessibleName('Included')
  await expect(requiredSwitch).toHaveAccessibleDescription('Required')
  await expect(requiredSwitch).toBeDisabled()
  await expect(requiredSwitch).toHaveAttribute('aria-checked', 'true')
  await expect(optionalSwitch).toHaveAccessibleName('Included')
  await expect(optionalSwitch).toHaveAccessibleDescription('Optional')
  await expect(optionalSwitch).toBeEnabled()
  await expect(page.locator('.component-scope p').first()).toHaveCSS('color', 'rgb(188, 188, 188)')
  await expectAccessible(page, ['.component-scope p'], ['color-contrast'])
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
  await expectStackedWorkspaceRail(page, '.review-layout.review-layout-single', '.review-surface', true)
  await page.getByRole('button', { name: 'Validate deployment' }).click()
  await expect(page.locator('box-run-trace.dispatch-run-trace').locator('[part="step"][data-step-id="salesforce"]')).toHaveAttribute('data-status', 'running')
  await expect(page.locator('box-run-trace.dispatch-run-trace').locator('[part="step"][data-step-id="salesforce"] [part="status"]')).toHaveText('Running')
  expect(await page.locator('box-run-trace.dispatch-run-trace').locator('[part="step"][data-step-id="salesforce"] [part="marker"]').evaluate((marker) => ({
    background: getComputedStyle(marker).backgroundColor,
    injectedStyle: Boolean((marker.getRootNode() as ShadowRoot).querySelector('[data-dispatch-running-pulse]')),
  }))).toEqual({ background: 'rgb(0, 229, 129)', injectedStyle: false })
  await page.reload()
  await expect(page.getByRole('heading', { name: 'Validation', exact: true })).toBeVisible()
  await expect(page.getByText('All selected systems finished successfully.')).toBeVisible()
  // Completed runs must replay their activity too, not only runs still in flight.
  await page.reload()
  await expect(page.getByText('All selected systems finished successfully.')).toBeVisible()
  await expect.poll(() => page.getByText('Authentication verified').count()).toBeGreaterThanOrEqual(2)
  await page.getByRole('link', { name: 'Overview', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Overview' })).toBeVisible()
  await page.evaluate(() => { window.location.hash = '#workspace' })
  await expect(page.getByRole('heading', { name: 'Validation', exact: true })).toBeVisible()
  await expect(page.locator('box-path')).toHaveAttribute('current', 'Review')
  await expect(page.getByRole('button', { name: 'Continue to deployment' })).toBeVisible()
  await expect(page.getByText('Validation results restored. Review changes or continue to deployment.')).toBeVisible()
  await expectStackedWorkspaceRail(page, '.deploy-workspace', '.validation-surface')
  const runSummary = page.locator('box-run-summary.dispatch-run-summary')
  await expect(runSummary).toBeVisible()
  const runSummaryState = await runSummary.evaluate((element) => ({ open: (element as HTMLElement & { open: boolean }).open, steps: (element as HTMLElement & { turn: { steps: unknown[] } }).turn.steps.length }))
  expect(runSummaryState.open).toBe(true)
  expect(runSummaryState.steps).toBeGreaterThan(0)
  await expectSharedAccessibilityInBothThemes(page, ['box-badge', ['box-path', '[part="stage-label"]'], ['box-run-trace.dispatch-run-trace', '[part="summary"]'], 'box-run-summary.dispatch-run-summary'])

  await page.getByRole('button', { name: 'Review changes' }).click()
  await expect(page.getByRole('heading', { name: 'Review validation changes' })).toBeVisible()
  await expect(page.getByText('2 files')).toBeVisible()
  await expect(page.locator('box-diff-viewer[heading="settings/Communities.settings-meta.xml"]')).toBeVisible()
  await expectSharedAccessibilityInBothThemes(page, ['box-table.validation-change-table'])
  await expectSharedAccessibilityInBothThemes(page, ['box-diff-viewer'], ['label-content-name-mismatch'])
  await expectDiffGutterContrast(page)
  expect(await page.locator('box-diff-viewer').evaluate((viewer) => ({
    before: viewer.getAttribute('before-label'),
    after: viewer.getAttribute('after-label'),
    mode: viewer.getAttribute('mode'),
    hasCurrent: viewer.getAttribute('before-text')?.includes('false'),
    hasPackaged: viewer.getAttribute('after-text')?.includes('true'),
  }))).toEqual({ before: 'Current state', after: 'Validated package', mode: 'split', hasCurrent: true, hasPackaged: true })
  await page.setViewportSize({ width: 1024, height: 768 })
  await expect(page.locator('box-diff-viewer')).toHaveAttribute('mode', 'inline')
  expect(await page.locator('box-diff-viewer').evaluate((viewer) => {
    const scroller = viewer.shadowRoot!.querySelector<HTMLElement>('[part="scroller"]')!
    return scroller.scrollWidth - scroller.clientWidth
  })).toBeLessThanOrEqual(1)
  await page.setViewportSize({ width: 1280, height: 720 })
  await expect(page.locator('box-diff-viewer')).toHaveAttribute('mode', 'split')
  await page.locator('box-table.validation-change-table').locator('[part="row"][data-index="1"]').click()
  await expect(page.locator('box-table.validation-change-table').getByRole('grid')).toBeVisible()
  await expect(page.locator('box-table.validation-change-table').getByRole('row', { selected: true })).toHaveCount(1)
  await expect(page.locator('box-diff-viewer[heading="objects/Contract__c/fields/Amount__c.field-meta.xml"]')).toBeVisible()
  await page.locator('box-table.validation-change-table').locator('[part="row"][data-index="0"]').click()
  await expect(page.locator('box-diff-viewer[heading="settings/Communities.settings-meta.xml"]')).toBeVisible()
  await page.getByRole('button', { name: 'Close drawer' }).click()

  for (const details of await page.getByRole('button', { name: 'Details' }).all()) await details.click()
  const childAlignment = await page.locator('box-run-trace.dispatch-run-trace').evaluate((trace) => {
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

  const traceAlignment = await page.locator('box-run-trace.dispatch-run-trace').evaluate((trace) => {
    const hostStyle = getComputedStyle(trace)
    const steps = [...trace.shadowRoot!.querySelectorAll<HTMLElement>('[part="step"]')]
    const markers = steps.map((step) => step.querySelector<HTMLElement>('[part="marker"]')!.getBoundingClientRect())
    const connector = getComputedStyle(steps[0], '::after')
    const step = steps[0].getBoundingClientRect()
    const lineCenter = step.left + Number.parseFloat(connector.left) + Number.parseFloat(connector.width) / 2
    const lineTop = step.top + Number.parseFloat(connector.top)
    const lineBottom = step.bottom - Number.parseFloat(connector.bottom)
    return {
      geometryTokens: {
        column: hostStyle.getPropertyValue('--boe-run-trace-marker-column-width').trim(),
        marker: hostStyle.getPropertyValue('--boe-run-trace-marker-size').trim(),
        inset: hostStyle.getPropertyValue('--boe-run-trace-marker-inline-inset').trim(),
        offset: hostStyle.getPropertyValue('--boe-run-trace-marker-block-offset').trim(),
        gap: hostStyle.getPropertyValue('--boe-run-trace-step-column-gap').trim(),
      },
      markerCenterDelta: Math.abs((markers[0].left + markers[0].width / 2) - (markers[1].left + markers[1].width / 2)),
      connectorCenterDelta: Math.abs(lineCenter - (markers[0].left + markers[0].width / 2)),
      firstEdgeGap: Math.abs(lineTop - markers[0].bottom),
      secondEdgeGap: Math.abs(markers[1].top - lineBottom),
    }
  })
  expect(traceAlignment.geometryTokens).toEqual({ column: '22px', marker: '18px', inset: '0px', offset: '3px', gap: '12px' })
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
  await expectStackedWorkspaceRail(page, '.summary-workspace', '.summary-surface')
  await expect(page.locator('box-status-icon.summary-status-icon')).toHaveAttribute('kind', 'done')
  await expect(page.locator('box-status-icon.summary-status-icon')).toHaveAttribute('label', 'Complete')
  await page.setViewportSize({ width: 390, height: 844 })
  await expect.poll(() => page.locator('box-status-icon.summary-status-icon').evaluate((icon) => {
    const svg = icon.shadowRoot!.querySelector<SVGSVGElement>('svg')!
    return svg.getAnimations({ subtree: true }).every((animation) => animation.playState === 'finished')
  })).toBe(true)
  const summaryIconBounds = await page.locator('box-status-icon.summary-status-icon').evaluate((icon) => {
    const host = icon.getBoundingClientRect()
    const glyph = icon.shadowRoot!.querySelector<HTMLElement>('[part="glyph"]')!.getBoundingClientRect()
    const svg = icon.shadowRoot!.querySelector<SVGSVGElement>('svg')!
    const svgBounds = svg.getBoundingClientRect()
    const cardElement = icon.closest('box-card')!.shadowRoot!.querySelector<HTMLElement>('[part="card"]')!
    const card = cardElement.getBoundingClientRect()
    const cardStyle = getComputedStyle(cardElement)
    const heading = icon.parentElement!.querySelector<HTMLElement>('h2')!.getBoundingClientRect()
    return {
      host: { left: host.left, right: host.right, top: host.top, bottom: host.bottom },
      glyph: { left: glyph.left, right: glyph.right, top: glyph.top, bottom: glyph.bottom },
      svg: { left: svgBounds.left, right: svgBounds.right, top: svgBounds.top, bottom: svgBounds.bottom },
      cardLeft: card.left,
      cardPaddingLeft: Number.parseFloat(cardStyle.paddingLeft),
      cardPaddingRight: Number.parseFloat(cardStyle.paddingRight),
      headingLeft: heading.left,
      contentInset: heading.left - card.left,
    }
  })
  expect(summaryIconBounds.glyph.left).toBeGreaterThanOrEqual(summaryIconBounds.host.left - 0.01)
  expect(summaryIconBounds.glyph.right).toBeLessThanOrEqual(summaryIconBounds.host.right + 0.01)
  expect(summaryIconBounds.glyph.top).toBeGreaterThanOrEqual(summaryIconBounds.host.top - 0.01)
  expect(summaryIconBounds.glyph.bottom).toBeLessThanOrEqual(summaryIconBounds.host.bottom + 0.01)
  expect(summaryIconBounds.svg).toEqual(summaryIconBounds.host)
  expect(Math.abs(summaryIconBounds.host.left - summaryIconBounds.headingLeft)).toBeLessThan(0.01)
  expect(summaryIconBounds.cardPaddingLeft).toBeGreaterThanOrEqual(16)
  expect(summaryIconBounds.cardPaddingRight).toBeGreaterThanOrEqual(16)
  expect(summaryIconBounds.contentInset).toBeGreaterThanOrEqual(16)
  await page.setViewportSize({ width: 1280, height: 720 })
  await expect(page.getByText('Every selected system finished successfully. Open a destination to review the deployed experience.')).toHaveCount(0)
  await expect(page.getByRole('heading', { name: 'Deployment targets' })).toBeVisible()
  await expect(page.getByText('acme.app.box.com')).toBeVisible()
  await expect(page.getByText('Enterprise ID EID-mock')).toBeVisible()
  await expect(page.getByText('example.my.salesforce.com', { exact: true })).toBeVisible()
  await expect(page.getByText('Org ID 00D-mock')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Open Box' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Open Salesforce' })).toBeVisible()
  await expect(page.getByRole('heading', { name: 'Open deployed applications' })).toBeVisible()
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
  await expect(page.getByText('The immutable audit record for this run.')).toHaveCount(0)
  await expect(page.getByText('acme.app.box.com')).toBeVisible()
  await expect(page.getByText('Enterprise ID EID-mock')).toBeVisible()
  await expect(page.getByText('example.my.salesforce.com', { exact: true })).toBeVisible()
  await expect(page.getByText('Org ID 00D-mock')).toBeVisible()
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
  for (const colorScheme of ['light', 'dark'] as const) {
    await page.emulateMedia({ colorScheme })
    for (const width of [1440, 390]) {
      await page.setViewportSize({ width, height: 900 })
      const table = page.getByRole('table', { name: 'All deployments' })
      await expect(table.getByRole('link', { name: 'Northstar CLM rollout' })).toBeVisible()
      expect(await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)).toBeLessThanOrEqual(1)
      const header = table.getByRole('columnheader', { name: /Deployment/ }).first()
      const sortButton = header.getByRole('button')
      await sortButton.focus()
      // Controlled React row/sort updates must preserve keyboard focus. Focus
      // once only: subsequent activations use the surviving native button.
      for (let activation = 0; activation < 4; activation++) {
        const next = await header.getAttribute('aria-sort') === 'ascending' ? 'descending' : 'ascending'
        await page.keyboard.press(activation % 2 === 0 ? 'Enter' : 'Space')
        await expect(header).toHaveAttribute('aria-sort', next)
        await expect(sortButton).toBeFocused()
      }
      await expect(table.locator('[aria-selected]')).toHaveCount(0)
      await expectAccessible(page, ['box-table.deployment-history-table'], ['aria-allowed-attr', 'aria-required-children', 'aria-required-parent', 'aria-valid-attr-value'])
      await test.info().attach(`history-sort-${colorScheme}-${width}`, { body: await page.screenshot({ fullPage: true }), contentType: 'image/png' })
      await table.getByRole('link', { name: 'Northstar CLM rollout' }).click()
      await expect(page.getByRole('heading', { name: 'Northstar CLM rollout' })).toBeVisible()
      await expect(page.getByRole('table', { name: 'Components deployed by this deployment' })).toBeVisible()
      await page.getByRole('link', { name: 'Back to deployment history' }).click()
    }
  }
  await page.setViewportSize({ width: 390, height: 844 })
  const mobileHistoryTable = page.locator('box-table.deployment-history-table')
  const mobileHistoryGeometry = await mobileHistoryTable.evaluate((element) => {
    const shell = element.shadowRoot!.querySelector<HTMLElement>('[part="shell"]')!
    const table = element.shadowRoot!.querySelector<HTMLElement>('[part="table"]')!
    const headers = [...element.shadowRoot!.querySelectorAll<HTMLElement>('[part="sort-button"]')]
    return {
      overflow: shell.scrollWidth - shell.clientWidth,
      tableWidth: table.getBoundingClientRect().width,
      headersWrap: headers.some((header) => header.scrollHeight > header.clientHeight + 1),
    }
  })
  expect(mobileHistoryGeometry.overflow).toBeGreaterThan(200)
  expect(mobileHistoryGeometry.tableWidth).toBeGreaterThanOrEqual(680)
  expect(mobileHistoryGeometry.headersWrap).toBe(false)
  await mobileHistoryTable.evaluate((element) => {
    const shell = element.shadowRoot!.querySelector<HTMLElement>('[part="shell"]')!
    shell.scrollLeft = shell.scrollWidth
  })
  await expect.poll(() => mobileHistoryTable.evaluate((element) => element.shadowRoot!.querySelector<HTMLElement>('[part="shell"]')!.scrollLeft)).toBeGreaterThan(200)
  await page.getByRole('link', { name: 'Northstar CLM rollout' }).click()
  await expect(page.getByRole('heading', { name: 'Deployment details' })).toBeVisible()
  await expect(page.getByRole('group', { name: 'Deployment actions' })).toBeVisible()
  await expectAccessible(page, ['.history-environment-actions'], ['aria-prohibited-attr'])
  expect(await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)).toBeLessThanOrEqual(1)
})

test('keeps the primary workflow usable on a mobile viewport', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto('/')
  expect(await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)).toBeLessThanOrEqual(1)
  expect(await page.locator('box-nav-sidebar.sidebar .nav-route [data-nav-label]').evaluateAll((labels) => labels.map((label) => getComputedStyle(label).display))).toEqual(['none', 'none', 'none', 'none', 'none'])
  await expect(page.getByRole('link', { name: 'Overview', exact: true })).toBeVisible()
  await expect(page.getByRole('link', { name: 'Deployments', exact: true })).toBeVisible()
  await expect(page.getByRole('link', { name: 'Compare Box environments', exact: true })).toBeVisible()
  await expect(page.getByRole('link', { name: 'Deployment history', exact: true })).toBeVisible()
  await expect(page.getByRole('link', { name: 'Settings', exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'New deployment' }).click()

  await expect(page.getByRole('heading', { name: 'Choose a solution' })).toBeVisible()
  const lifecycleScroll = page.getByRole('region', { name: 'Deployment lifecycle stages' })
  await lifecycleScroll.focus()
  await expect(lifecycleScroll).toBeFocused()
  await expect(lifecycleScroll.locator('box-path')).toHaveAttribute('variant', 'base')
  expect(await lifecycleScroll.evaluate((region) => {
    const path = region.querySelector('box-path')!.getBoundingClientRect()
    return {
      overflow: region.scrollWidth - region.clientWidth,
      clipped: [...region.querySelector('box-path')!.shadowRoot!.querySelectorAll<HTMLElement>('[part="stage"]')].some((stage) => {
        const bounds = stage.getBoundingClientRect()
        return bounds.left < path.left - 1 || bounds.right > path.right + 1
      }),
      labelOverflow: [...region.querySelector('box-path')!.shadowRoot!.querySelectorAll<HTMLElement>('[part="stage-label"]')].some((label) => label.scrollWidth > label.clientWidth + 1),
    }
  })).toEqual({ overflow: 0, clipped: false, labelOverflow: false })
  await expectAccessible(page, ['.workflow-path-scroll'], ['scrollable-region-focusable'])
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
  await expect(defaults.getByText('Starting configuration for new deployments. Each deployment can override these choices.')).toHaveCount(0)
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
