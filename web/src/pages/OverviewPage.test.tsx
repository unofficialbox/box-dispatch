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
    const { container } = render(<OverviewPage plan={plan} connections={connections} deployments={[]} run={null} onNewDeployment={vi.fn()} onContinue={vi.fn()} onBoxConnection={vi.fn()} onSalesforceConnection={vi.fn()} />)

    expect(screen.getByText('Choose a solution to start a deployment.')).toBeTruthy()
    expect(screen.getByText('Not started')).toBeTruthy()
    expect(screen.getByText('No runs')).toBeTruthy()
    expect(screen.getAllByText('Choose a solution').length).toBeGreaterThanOrEqual(2)
    expect(screen.queryByText('CLM deployment')).toBeNull()
    expect(screen.queryByText('Your deployment plan and connections are ready.')).toBeNull()
    expect(container.querySelector('.overview-current [data-provider-logo]')).toBeNull()
    expect(container.querySelector('.overview-connection-health [data-provider-logo="box"]')).toBeTruthy()
    expect(container.querySelector('.overview-connection-health [data-provider-logo="salesforce"]')).toBeTruthy()
  })

  it('keeps an active deployment reachable after refresh', () => {
    const onContinue = vi.fn()
    const { container } = render(<OverviewPage
      plan={{ ...plan, exists: true, name: 'Northstar CLM' }}
      connections={connections}
      deployments={[]}
      run={{ id: 'deploy-1', deployment: 'Northstar CLM', action: 'deploy', status: 'running', providers: [] }}
      onNewDeployment={vi.fn()} onContinue={onContinue} onBoxConnection={vi.fn()} onSalesforceConnection={vi.fn()}
    />)

    const button = container.querySelector<HTMLElement>('box-button[label="View deployment"]')
    expect(button).toBeTruthy()
    fireEvent.click(button!)
    expect(onContinue).toHaveBeenCalledOnce()
  })

  it('uses concise provider labels rather than logos in deployment history', () => {
    const { container } = render(<OverviewPage plan={{ ...plan, exists: true, name: 'Northstar CLM' }} connections={connections} deployments={[{ id: 'deployment-1', name: 'Northstar CLM', strategy: 'reuse', completedAt: '2026-08-25T17:00:00Z', providers: [{ name: 'box', status: 'succeeded' }, { name: 'salesforce', status: 'succeeded' }] }]} run={null} onNewDeployment={vi.fn()} onContinue={vi.fn()} onBoxConnection={vi.fn()} onSalesforceConnection={vi.fn()} />)

    const table = within(container).getByRole('table', { name: 'Recent deployments' })
    expect(table).toBeTruthy()
    expect(container.querySelector('.overview-history-table [data-provider-logo]')).toBeNull()
    expect(within(table).getByText('Box, Salesforce')).toBeTruthy()
  })
})
