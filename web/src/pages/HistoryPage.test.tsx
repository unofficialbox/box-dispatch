// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { HistoryPage } from './HistoryPage'
import type { DeploymentDetail, DeploymentSummary } from '../types'

const deployment = (index: number, status = 'present', providers = ['box', 'salesforce'], strategy = 'reuse'): DeploymentSummary => ({
  id: `deployment-${index}`,
  name: `Deployment ${index}`,
  strategy,
  completedAt: `2026-08-${String(index + 1).padStart(2, '0')}T12:00:00Z`,
  providers: providers.map((name) => ({ name, status })),
})

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

describe('HistoryPage', () => {
  it('renders the full deployment history with outcomes', () => {
    const deployments = [deployment(1), deployment(2), deployment(3), deployment(4), deployment(5), deployment(6, 'failed')]
    render(<HistoryPage deployments={deployments} onOpenDeployment={vi.fn()}/>)

    const table = screen.getByRole('table', { name: 'All deployments' })
    expect(within(table).getByText('Deployment 6')).toBeTruthy()
    expect(within(table).getAllByText('Box, Salesforce')).toHaveLength(6)
    expect(screen.getByText('6 recorded')).toBeTruthy()
    expect(table.querySelector('box-badge[label="Needs attention"]')).toBeTruthy()
  })

  it('filters deployments by search, system, result, and strategy', () => {
    const deployments = [
      deployment(1, 'present', ['box'], 'reuse'),
      deployment(2, 'failed', ['salesforce'], 'create_new'),
      deployment(3, 'present', ['box', 'salesforce'], 'create_new'),
    ]
    render(<HistoryPage deployments={deployments} onOpenDeployment={vi.fn()}/>)

    fireEvent.change(screen.getByLabelText('System'), { target: { value: 'salesforce' } })
    fireEvent.change(screen.getByLabelText('Result'), { target: { value: 'complete' } })
    fireEvent.change(screen.getByLabelText('Strategy'), { target: { value: 'create_new' } })
    fireEvent.change(screen.getByLabelText('Search'), { target: { value: 'Deployment 3' } })

    const table = screen.getByRole('table', { name: 'All deployments' })
    expect(within(table).getByText('Deployment 3')).toBeTruthy()
    expect(within(table).queryByText('Deployment 1')).toBeNull()
    expect(within(table).queryByText('Deployment 2')).toBeNull()
    expect(screen.getByText('Showing 1 of 3')).toBeTruthy()

    fireEvent.click(screen.getByRole('button', { name: 'Clear filters' }))
    expect(within(table).getAllByRole('button', { name: /View summary for/ })).toHaveLength(3)
  })

  it('opens a selected deployment and renders its audit summary', async () => {
    const detail: DeploymentDetail = {
      ...deployment(1),
      startedAt: '2026-08-02T11:58:00Z',
      duration: '2m0s',
      runId: 'web-run-1',
      providers: [
        { name: 'box', status: 'present', deployedCount: 3, presentCount: 8, remainingCount: 0, manualItemCount: 1 },
        { name: 'salesforce', status: 'present', deployedCount: 12, presentCount: 24, remainingCount: 0, manualItemCount: 0 },
      ],
    }
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify(detail), { status: 200, headers: { 'Content-Type': 'application/json' } }))
    const onCloseDeployment = vi.fn()
    const { container } = render(<HistoryPage deployments={[deployment(1)]} selectedDeploymentID="deployment-1" onCloseDeployment={onCloseDeployment}/>)

    await waitFor(() => expect(screen.getByRole('heading', { name: 'Deployment 1' })).toBeTruthy())
    expect(screen.getByRole('heading', { name: 'Provider summary' })).toBeTruthy()
    expect(screen.getByText('web-run-1')).toBeTruthy()
    expect(screen.getByText('2m0s')).toBeTruthy()
    expect(container.querySelectorAll('box-badge[label="Complete"]')).toHaveLength(3)
    expect(screen.getByText('24')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: 'Back to deployment history' }))
    expect(onCloseDeployment).toHaveBeenCalledOnce()
  })

  it('selects a deployment from the history table', () => {
    const onOpenDeployment = vi.fn()
    render(<HistoryPage deployments={[deployment(1)]} onOpenDeployment={onOpenDeployment}/>)
    fireEvent.click(screen.getByRole('button', { name: 'View summary for Deployment 1' }))
    expect(onOpenDeployment).toHaveBeenCalledWith('deployment-1')
  })
})
