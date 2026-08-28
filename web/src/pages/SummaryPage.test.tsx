// @vitest-environment jsdom
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SummaryPage } from './SummaryPage'
import type { DeploymentPlan, DispatchRun } from '../types'

describe('SummaryPage', () => {
  it('uses specific provider destinations and a single overview action', () => {
    const plan: DeploymentPlan = { exists: true, name: 'Northstar CLM', templateId: 'clm', template: 'CLM deployment', repository: 'example/repo', strategy: 'reuse', components: [{ id: 'box', name: 'Box', configured: true, verified: true, ready: true }, { id: 'salesforce', name: 'Salesforce', configured: true, verified: true, ready: true }] }
    const run: DispatchRun = { id: 'deploy-1', action: 'deploy', status: 'completed', providers: [{ name: 'box', status: 'present' }, { name: 'salesforce', status: 'present' }], resources: [{ provider: 'salesforce', component: 'Salesforce Experience', kind: 'experience_site', name: 'CLM Experience', id: '0DB1', url: 'https://example.my.site.com/clm' }] }
    const onOpenProvider = vi.fn()
    const onOverview = vi.fn()
    render(<SummaryPage plan={plan} connections={[{ name: 'Box', configured: true, verified: true, launchUrl: 'https://app.box.com/' }, { name: 'Salesforce', configured: true, verified: true, launchUrl: '/api/connections/salesforce/open' }]} run={run} onOpenProvider={onOpenProvider} onOverview={onOverview} />)

    expect(screen.getByText('Northstar CLM is ready')).toBeTruthy()
    const box = screen.getByText('Box workspace').closest('li')?.querySelector('box-button[label="Open"]')
    const boxSettings = screen.getByRole('link', { name: 'Open Box App & Settings' })
    const clmApp = screen.getByRole('link', { name: 'Open Contract Lifecycle Management' })
    const experienceSite = screen.getByRole('link', { name: 'Open Experience Cloud site' })
    expect(box).toBeTruthy()
    expect(box?.getAttribute('tone')).toBe('primary')
    expect(boxSettings.classList.contains('summary-destination-link')).toBe(true)
    expect(clmApp.classList.contains('summary-destination-link')).toBe(true)
    expect(experienceSite.classList.contains('summary-destination-link')).toBe(true)
    expect(boxSettings.getAttribute('href')).toBe('/api/connections/salesforce/open?destination=box-settings')
    expect(clmApp.getAttribute('href')).toBe('/api/connections/salesforce/open?destination=clm-app')
    expect(experienceSite.getAttribute('href')).toBe('/api/connections/salesforce/open?destination=experience-site&site=0DB1')
    fireEvent.click(box!)
    expect(onOpenProvider).toHaveBeenCalledWith('box')
  })
})
