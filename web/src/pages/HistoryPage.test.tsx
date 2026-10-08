// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { TableCellValue, TableRow } from '@unofficialbox/box-open-elements/table'
import { HistoryPage } from './HistoryPage'
import type { DeploymentDetail, DeploymentSummary } from '../types'

const deployment = (index: number, status = 'present', providers = ['box', 'salesforce'], strategy = 'reuse'): DeploymentSummary => ({
  id: `deployment-${index}`,
  name: `Deployment ${index}`,
  strategy,
  completedAt: `2026-08-${String(index + 1).padStart(2, '0')}T12:00:00Z`,
  providers: providers.map((name) => ({ name, status })),
})

const cells = (row: TableRow) => row.cells as Record<string, TableCellValue>

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

describe('HistoryPage', () => {
  it('renders the full deployment history with outcomes', () => {
    const deployments = [deployment(1), deployment(2), deployment(3), deployment(4), deployment(5), deployment(6, 'failed')]
    const { container } = render(<HistoryPage deployments={deployments}/>)

    const table = container.querySelector('box-table.deployment-history-table') as HTMLElement & { rows: TableRow[] }
    expect(table.rows.some((row) => cells(row).deployment && JSON.stringify(cells(row).deployment).includes('Deployment 6'))).toBe(true)
    expect(table.rows.filter((row) => cells(row).systems === 'Box, Salesforce')).toHaveLength(6)
    expect(screen.getByText('6 recorded')).toBeTruthy()
    expect(table.rows.some((row) => JSON.stringify(cells(row).result).includes('Needs attention'))).toBe(true)
  })

  it('filters deployments by search, system, result, and strategy', () => {
    const deployments = [
      deployment(1, 'present', ['box'], 'reuse'),
      deployment(2, 'failed', ['salesforce'], 'create_new'),
      deployment(3, 'present', ['box', 'salesforce'], 'create_new'),
    ]
    const { container } = render(<HistoryPage deployments={deployments}/>)

    const filter = (label: string, value: string) => fireEvent(document.querySelector(`box-select[label="${label}"]`)!, new CustomEvent('value-changed', { detail: { value } }))
    filter('System', 'salesforce')
    filter('Result', 'complete')
    filter('Strategy', 'create_new')
    fireEvent(document.querySelector('box-search-field[label="Search"]')!, new CustomEvent('value-changed', { detail: { value: 'Deployment 3' } }))

    const table = container.querySelector('box-table.deployment-history-table') as HTMLElement & { rows: TableRow[] }
    expect(table.rows).toHaveLength(1)
    expect(JSON.stringify(cells(table.rows[0]).deployment)).toContain('Deployment 3')
    expect(screen.getByText('Showing 1 of 3')).toBeTruthy()

    fireEvent.click(document.querySelector('box-button[label="Clear filters"]')!)
    expect(table.rows).toHaveLength(3)
  })

  it('opens a selected deployment and renders its audit summary', async () => {
    const detail: DeploymentDetail = {
      ...deployment(1),
      startedAt: '2026-08-02T11:58:00Z',
      duration: '2m0s',
      runId: 'web-run-1',
      changesRecorded: true,
      changeCount: 2,
      providers: [
        { name: 'box', status: 'present', deployedCount: 1, presentCount: 8, remainingCount: 0, manualItemCount: 1, deployedComponents: ['Metadata Template:Contract'], environmentId: '5105484', launchUrl: 'https://app.box.com/' },
        { name: 'salesforce', status: 'present', deployedCount: 1, presentCount: 24, remainingCount: 0, manualItemCount: 0, deployedComponents: ['UIBundle:clmreactapp'], environmentId: '00D123', launchUrl: '/api/connections/salesforce/open' },
      ],
    }
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify(detail), { status: 200, headers: { 'Content-Type': 'application/json' } }))
    const onCloseDeployment = vi.fn()
    const onOpenDestination = vi.fn()
    const onViewChanges = vi.fn()
    const { container } = render(<HistoryPage deployments={[deployment(1)]} selectedDeploymentID="deployment-1" onCloseDeployment={onCloseDeployment} onOpenDestination={onOpenDestination} onViewChanges={onViewChanges}/>)

    await waitFor(() => expect(screen.getByRole('heading', { name: 'Deployment 1' })).toBeTruthy())
    expect(container.querySelector('box-section[heading="Provider summary"]')).toBeTruthy()
    const facts = [...container.querySelectorAll('box-fact-list')].find((element) => (element as HTMLElement & { rows: Array<{ label: string; value: string }> }).rows?.some((row) => row.label === 'Run ID')) as HTMLElement & { rows: Array<{ label: string; value: string }> }
    expect(facts.rows).toContainEqual({ label: 'Run ID', value: 'web-run-1' })
    expect(facts.rows).toContainEqual({ label: 'Duration', value: '2m0s' })
    expect(container.querySelectorAll('box-badge[label="Complete"]')).toHaveLength(3)
    const componentTable = container.querySelector('box-table.deployment-component-table') as HTMLElement & { rows: TableRow[] }
    await waitFor(() => expect(componentTable.rows).toHaveLength(2))
    expect(componentTable.rows.map((row) => cells(row).component)).toEqual(['Metadata Template:Contract', 'UIBundle:clmreactapp'])
    const providerFacts = [...container.querySelectorAll('.history-provider-card box-fact-list')] as Array<HTMLElement & { rows: Array<{ label: string; value: string }> }>
    expect(providerFacts[1].rows).toContainEqual({ label: 'Present', value: '24' })
    const providerCards = container.querySelectorAll('.history-provider-card')
    expect(providerCards[0].querySelector('header box-button[label="Open Box"]')).toBeTruthy()
    expect(providerCards[1].querySelector('header box-button[label="Open Salesforce"]')).toBeTruthy()
    expect(container.querySelector('box-button[label*="5105484"], box-button[label*="00D123"]')).toBeNull()
    fireEvent.click(container.querySelector('box-button[label="Open Box"]')!)
    expect(onOpenDestination).toHaveBeenCalledWith('https://app.box.com/', 'Open Box')
    fireEvent.click(container.querySelector('box-button[label="Open Salesforce"]')!)
    expect(onOpenDestination).toHaveBeenCalledWith('/api/connections/salesforce/open', 'Open Salesforce')
    fireEvent.click(container.querySelector('box-button[label="Review changes"]')!)
    expect(onViewChanges).toHaveBeenCalledWith('deployment-1')
    const backLink = container.querySelector('box-link-button[label="Back to deployment history"]')!
    expect(backLink.getAttribute('href')).toBe('#history')
    fireEvent.click(backLink)
    expect(onCloseDeployment).toHaveBeenCalledOnce()
  })

  it('explains when a legacy audit has no recorded change preview', async () => {
    const detail: DeploymentDetail = { ...deployment(1), startedAt: '2026-08-02T11:58:00Z', duration: '2m0s', changesRecorded: false, changeCount: 0, providers: [] }
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify(detail), { status: 200, headers: { 'Content-Type': 'application/json' } }))
    render(<HistoryPage deployments={[deployment(1)]} selectedDeploymentID="deployment-1"/>)

    await waitFor(() => expect(screen.getByText('Change preview not recorded')).toBeTruthy())
  })

  it('links a deployment row to its deep history route and sorts on request', () => {
    const { container } = render(<HistoryPage deployments={[deployment(2), deployment(1)]}/>)
    const table = container.querySelector('box-table.deployment-history-table') as HTMLElement & { rows: TableRow[] }
    expect(cells(table.rows[0]).deployment).toMatchObject({ kind: 'link', href: '#history/deployment-2' })
    fireEvent(table, new CustomEvent('sort', { detail: { key: 'deployment', direction: 'ascending' } }))
    expect(JSON.stringify(cells(table.rows[0]).deployment)).toContain('Deployment 1')
  })
})
