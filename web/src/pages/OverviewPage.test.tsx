// @vitest-environment jsdom
import { fireEvent, render, screen, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { OverviewPage } from './OverviewPage'
import type { ConnectionSummary, DeploymentPlan } from '../types'

const plan: DeploymentPlan = {
  exists: false,
  name: '',
  templateId: '',
  template: 'CLM deployment',
  repository: '',
  strategy: 'reuse',
  components: [
    { id: 'box', name: 'Box', configured: true, verified: true, ready: true },
    { id: 'salesforce', name: 'Salesforce', configured: true, verified: true, ready: true },
  ],
}

const connections: ConnectionSummary[] = [
  { name: 'Box', configured: true, verified: true },
  { name: 'Salesforce', configured: true, verified: true },
]

describe('OverviewPage', () => {
  it('does not present healthy connections as an existing deployment plan', () => {
    const { container } = render(<OverviewPage plan={plan} connections={connections} deployments={[]} run={null} onNewDeployment={vi.fn()} onContinue={vi.fn()} onBoxConnection={vi.fn()} onSalesforceConnection={vi.fn()} onOpenProvider={vi.fn()} onViewHistory={vi.fn()} />)

    expect(screen.getByText('Choose a solution to start a deployment.')).toBeTruthy()
    expect(screen.getByText('Not started')).toBeTruthy()
    expect(screen.getByText('No runs')).toBeTruthy()
    expect(screen.getAllByText('Choose a solution').length).toBeGreaterThanOrEqual(2)
    expect(screen.queryByText('CLM deployment')).toBeNull()
    expect(screen.queryByText('Your deployment plan and connections are ready.')).toBeNull()
    expect(container.querySelector('.overview-current [data-provider-logo]')).toBeNull()
    expect(container.querySelector('.overview-connection-health [data-provider-logo="box"]')).toBeTruthy()
    expect(container.querySelector('.overview-connection-health [data-provider-logo="salesforce"]')).toBeTruthy()
    expect(container.querySelectorAll('.overview-connection-health .provider-connection-panel--compact')).toHaveLength(2)
  })

  it('keeps an active deployment reachable after refresh', () => {
    const onContinue = vi.fn()
    const { container } = render(<OverviewPage
      plan={{ ...plan, exists: true, name: 'Northstar CLM' }}
      connections={connections}
      deployments={[]}
      run={{ id: 'deploy-1', deployment: 'Northstar CLM', action: 'deploy', status: 'running', providers: [] }}
      onNewDeployment={vi.fn()} onContinue={onContinue} onBoxConnection={vi.fn()} onSalesforceConnection={vi.fn()} onOpenProvider={vi.fn()} onViewHistory={vi.fn()}
    />)

    const button = container.querySelector<HTMLElement>('box-button[label="View deployment"]')
    expect(button).toBeTruthy()
    fireEvent.click(button!)
    expect(onContinue).toHaveBeenCalledOnce()
  })

  it('uses concise provider labels rather than logos in deployment history', () => {
    const onOpenDeployment = vi.fn()
    const { container } = render(<OverviewPage plan={{ ...plan, exists: true, name: 'Northstar CLM' }} connections={connections} deployments={[{ id: 'deployment-1', name: 'Northstar CLM', strategy: 'reuse', completedAt: '2026-08-25T17:00:00Z', providers: [{ name: 'box', status: 'succeeded' }, { name: 'salesforce', status: 'succeeded' }] }]} run={null} onNewDeployment={vi.fn()} onContinue={vi.fn()} onBoxConnection={vi.fn()} onSalesforceConnection={vi.fn()} onOpenProvider={vi.fn()} onViewHistory={vi.fn()} onOpenDeployment={onOpenDeployment} />)

    const table = within(container).getByRole('table', { name: 'Recent deployments' })
    expect(table).toBeTruthy()
    expect(container.querySelector('.overview-history-table [data-provider-logo]')).toBeNull()
    expect(within(table).getByText('Box, Salesforce')).toBeTruthy()
    fireEvent.click(within(table).getByRole('button', { name: 'View summary for Northstar CLM' }))
    expect(onOpenDeployment).toHaveBeenCalledWith('deployment-1')
  })

  it('moves a completed deployment out of the active deployment state', () => {
    const onViewHistory = vi.fn()
    const { container } = render(<OverviewPage
      plan={{ ...plan, exists: true, name: 'Experience Cloud E2E' }}
      connections={connections}
      deployments={[]}
      run={{ id: 'deploy-complete', deployment: 'Experience Cloud E2E', action: 'deploy', status: 'completed', providers: [] }}
      onNewDeployment={vi.fn()} onContinue={vi.fn()} onBoxConnection={vi.fn()} onSalesforceConnection={vi.fn()} onOpenProvider={vi.fn()} onViewHistory={onViewHistory}
    />)

    expect(within(container).getByText('No deployment in progress')).toBeTruthy()
    expect(within(container).queryByText('Saved deployment')).toBeNull()
    expect(container.querySelector('box-button[label="Continue deployment"]')).toBeNull()
    const activeMetric = container.querySelector<HTMLElement>('.overview-metric:first-child')!
    expect(activeMetric.textContent).toContain('Active deployment')
    expect(activeMetric.textContent).toContain('None')
    expect(activeMetric.textContent).not.toContain('Complete')
    const currentDeployment = container.querySelector<HTMLElement>('.overview-current')!
    expect(currentDeployment.querySelector('box-badge[label="Complete"]')).toBeNull()
    expect(currentDeployment.textContent).toContain('The latest deployment, Experience Cloud E2E, completed successfully and is available in history.')
    const latestMetric = container.querySelector<HTMLElement>('.overview-metric:last-child')!
    expect(latestMetric.textContent).toContain('Latest deployment')
    const historyButton = container.querySelector<HTMLElement>('box-button[label="View history"]')
    fireEvent.click(historyButton!)
    expect(onViewHistory).toHaveBeenCalledOnce()
  })

  it('uses immutable history when a completed run is not restored after refresh', () => {
    const completed = { id: 'deployment-1', name: 'clmDemo1', strategy: 'reuse', completedAt: '2026-08-28T02:12:39Z', providers: [{ name: 'box', status: 'present' }, { name: 'salesforce', status: 'present' }] }
    const { container } = render(<OverviewPage
      plan={{ ...plan, exists: true, name: 'clmDemo1' }}
      connections={connections}
      deployments={[completed]}
      run={null}
      onNewDeployment={vi.fn()} onContinue={vi.fn()} onBoxConnection={vi.fn()} onSalesforceConnection={vi.fn()} onOpenProvider={vi.fn()} onViewHistory={vi.fn()}
    />)

    expect(within(container).getByText('No deployment is currently in progress.')).toBeTruthy()
    expect(within(container).getByText('No deployment in progress')).toBeTruthy()
    expect(container.querySelector('box-button[label="Continue deployment"]')).toBeNull()
    expect(container.querySelector<HTMLElement>('.overview-metric:first-child')?.textContent).toContain('None')
  })

  it('keeps a failed current run resumable even when an older deployment has the same name', () => {
    const completed = { id: 'deployment-1', name: 'clmDemo1', strategy: 'reuse', completedAt: '2026-08-27T02:12:39Z', providers: [{ name: 'box', status: 'present' }] }
    const { container } = render(<OverviewPage
      plan={{ ...plan, exists: true, name: 'clmDemo1' }}
      connections={connections}
      deployments={[completed]}
      run={{ id: 'deploy-failed', deployment: 'clmDemo1', action: 'deploy', status: 'failed', providers: [{ name: 'box', status: 'failed' }] }}
      onNewDeployment={vi.fn()} onContinue={vi.fn()} onBoxConnection={vi.fn()} onSalesforceConnection={vi.fn()} onOpenProvider={vi.fn()} onViewHistory={vi.fn()}
    />)

    expect(container.querySelector('box-button[label="Continue deployment"]')).toBeTruthy()
    expect(container.querySelector<HTMLElement>('.overview-metric:first-child')?.textContent).toContain('Active')
  })

  it('summarizes the same selected environments and saved counts shown in Settings', () => {
    const detailedConnections: ConnectionSummary[] = [
      { name: 'Box', configured: true, verified: true, launchUrl: 'https://app.box.com/', connections: [{ id: 'box-1', alias: 'Production Box', identity: 'owner@example.com', domain: 'acme.app.box.com', status: 'Ready', selected: true }, { id: 'box-2', alias: 'Demo Box', status: 'Not ready', selected: false }] },
      { name: 'Salesforce', configured: true, verified: true, launchUrl: 'https://example.my.salesforce.com/', orgs: [{ id: 'sf-1', alias: 'CLM Scratch', username: 'test@example.com', domain: 'example.scratch.my.salesforce.com', orgId: '00D123', kind: 'Scratch org', status: 'Ready', selected: true }] },
    ]
    const onOpenProvider = vi.fn()
    const onSalesforceConnection = vi.fn()
    const { container } = render(<OverviewPage plan={{ ...plan, exists: true }} connections={detailedConnections} deployments={[]} run={null} onNewDeployment={vi.fn()} onContinue={vi.fn()} onBoxConnection={vi.fn()} onSalesforceConnection={onSalesforceConnection} onOpenProvider={onOpenProvider} onViewHistory={vi.fn()} />)

    const health = container.querySelector<HTMLElement>('.overview-connection-health')!
    expect(within(health).getByText('Production Box')).toBeTruthy()
    expect(within(health).getByText('CLM Scratch')).toBeTruthy()
    expect(within(health).getByText('acme.app.box.com')).toBeTruthy()
    expect(within(health).getByText('example.scratch.my.salesforce.com')).toBeTruthy()
    expect(within(health).getByText('2 saved connections')).toBeTruthy()
    expect(within(health).getByText('1 saved connection')).toBeTruthy()
    expect(health.querySelector('box-badge[label="Selected"]')).toBeNull()
    expect(health.querySelectorAll('box-badge[label="Ready"]')).toHaveLength(2)
    expect(health.querySelectorAll('box-button[label="Manage"]')).toHaveLength(0)
    expect(health.querySelectorAll('box-icon-button[icon="arrow-right"]')).toHaveLength(2)
    expect(health.querySelectorAll('box-icon-button[icon="gear"]')).toHaveLength(2)

    fireEvent.click(health.querySelector('box-icon-button[label="Open Box"]')!)
    expect(onOpenProvider).toHaveBeenCalledWith('box')
    fireEvent.click(health.querySelector('box-icon-button[label="Configure Salesforce"]')!)
    expect(onSalesforceConnection).toHaveBeenCalledOnce()
  })
})
