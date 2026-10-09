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
    const onViewChanges = vi.fn()
    const onOverview = vi.fn()
    const { container } = render(<SummaryPage plan={plan} connections={[{ name: 'Box', configured: true, verified: true, launchUrl: 'https://app.box.com/', connections: [{ id: 'box-1', alias: 'Production Box', status: 'Ready', selected: true, domain: 'acme.app.box.com', enterpriseId: '5105484' }] }, { name: 'Salesforce', configured: true, verified: true, launchUrl: '/api/connections/salesforce/open', orgs: [{ id: 'sf-1', alias: 'CLM Scratch', kind: 'Scratch org', status: 'Ready', selected: true, domain: 'example.my.salesforce.com', orgId: '00D123' }] }]} run={run} onOpenProvider={onOpenProvider} onViewChanges={onViewChanges} onOverview={onOverview} />)

    expect(screen.getByText('Northstar CLM is ready')).toBeTruthy()
    expect(container.querySelector('box-status-icon.summary-status-icon[kind="done"][label="Complete"]')).toBeTruthy()
    expect(container.textContent).not.toContain('✓')
    expect(screen.getByText('acme.app.box.com')).toBeTruthy()
    expect(screen.getByText('Enterprise ID 5105484')).toBeTruthy()
    expect(screen.getByText('example.my.salesforce.com')).toBeTruthy()
    expect(screen.getByText('Org ID 00D123')).toBeTruthy()
    const box = container.querySelector('box-button[label="Open Box"]')
    const salesforce = container.querySelector('box-button[label="Open Salesforce"]')
    const boxSettings = container.querySelector('box-link-button[label="Open Box App & Settings"]')!
    const clmApp = container.querySelector('box-link-button[label="Open Contract Lifecycle Management"]')!
    const experienceSite = container.querySelector('box-link-button[label="Open Experience Cloud site"]')!
    expect(box).toBeTruthy()
    expect(salesforce).toBeTruthy()
    expect(container.querySelector('box-button[label*="5105484"], box-button[label*="00D123"]')).toBeNull()
    expect(boxSettings.classList.contains('summary-destination-link')).toBe(true)
    expect(clmApp.classList.contains('summary-destination-link')).toBe(true)
    expect(experienceSite.classList.contains('summary-destination-link')).toBe(true)
    expect(boxSettings.getAttribute('target')).toBe('_blank')
    expect(boxSettings.getAttribute('rel')).toBe('noreferrer')
    expect(boxSettings.getAttribute('href')).toBe('/api/connections/salesforce/open?destination=box-settings')
    expect(clmApp.getAttribute('href')).toBe('/api/connections/salesforce/open?destination=clm-app')
    expect(experienceSite.getAttribute('href')).toBe('/api/connections/salesforce/open?destination=experience-site&site=0DB1')
    fireEvent.click(box!)
    expect(onOpenProvider).toHaveBeenCalledWith('box')
    fireEvent.click(salesforce!)
    expect(onOpenProvider).toHaveBeenCalledWith('salesforce')
    fireEvent.click(container.querySelector('box-button[label="Review changes"]')!)
    expect(onViewChanges).toHaveBeenCalledWith('deploy-1')
  })

  it('does not call a deployment complete when work remains', () => {
    const plan: DeploymentPlan = { exists: true, name: 'Northstar CLM', templateId: 'clm', template: 'CLM deployment', repository: 'example/repo', strategy: 'reuse', components: [{ id: 'box', name: 'Box', configured: true, verified: true, ready: true }] }
    const run: DispatchRun = { id: 'deploy-2', action: 'deploy', status: 'completed', providers: [{ name: 'box', status: 'present', remainingCount: 1, manualItemCount: 1 }] }
    const { container } = render(<SummaryPage plan={plan} connections={[]} run={run} onOpenProvider={vi.fn()} onViewChanges={vi.fn()} onOverview={vi.fn()} />)

    expect(screen.getByText('Northstar CLM needs attention')).toBeTruthy()
    expect(screen.getByText('Deployment needs attention')).toBeTruthy()
    expect(screen.getByText(/Some components remain or require manual work/)).toBeTruthy()
    expect(container.querySelector('.summary-surface-attention')).toBeTruthy()
    expect(container.querySelector('box-status-icon.summary-status-icon[kind="warning"][label="Needs attention"]')).toBeTruthy()
    const facts = container.querySelector('box-fact-list') as HTMLElement & { rows: Array<{ label: string; value: string }> }
    expect(facts.rows).toContainEqual({ label: 'Status', value: 'Needs attention' })
    expect(screen.queryByText('Every selected system finished successfully.')).toBeNull()
  })

  it('does not infer success when provider results were not recorded', () => {
    const plan: DeploymentPlan = { exists: true, name: 'Northstar CLM', templateId: 'clm', template: 'CLM deployment', repository: 'example/repo', strategy: 'reuse', components: [{ id: 'box', name: 'Box', configured: true, verified: true, ready: true }] }
    const run: DispatchRun = { id: 'deploy-3', action: 'deploy', status: 'completed', providers: [] }
    const { container } = render(<SummaryPage plan={plan} connections={[]} run={run} onOpenProvider={vi.fn()} onViewChanges={vi.fn()} onOverview={vi.fn()} />)

    expect(screen.getByText('Northstar CLM was recorded')).toBeTruthy()
    expect(screen.getByText('Deployment recorded')).toBeTruthy()
    expect(container.querySelector('box-status-icon.summary-status-icon[kind="pending"][label="Recorded"]')).toBeTruthy()
    const facts = container.querySelector('box-fact-list') as HTMLElement & { rows: Array<{ label: string; value: string }> }
    expect(facts.rows).toContainEqual({ label: 'Status', value: 'Recorded' })
  })
})
