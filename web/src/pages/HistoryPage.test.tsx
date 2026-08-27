// @vitest-environment jsdom
import { render, screen, within } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { HistoryPage } from './HistoryPage'
import type { DeploymentSummary } from '../types'

const deployment = (index: number, status = 'present'): DeploymentSummary => ({
  id: `deployment-${index}`,
  name: `Deployment ${index}`,
  strategy: 'reuse',
  completedAt: `2026-08-${String(index + 1).padStart(2, '0')}T12:00:00Z`,
  providers: [{ name: 'box', status }, { name: 'salesforce', status }],
})

describe('HistoryPage', () => {
  it('renders the full deployment history with outcomes', () => {
    const deployments = [deployment(1), deployment(2), deployment(3), deployment(4), deployment(5), deployment(6, 'failed')]
    render(<HistoryPage deployments={deployments}/>)

    const table = screen.getByRole('table', { name: 'All deployments' })
    expect(within(table).getByText('Deployment 6')).toBeTruthy()
    expect(within(table).getAllByText('Box, Salesforce')).toHaveLength(6)
    expect(screen.getByText('6 recorded')).toBeTruthy()
    expect(table.querySelector('box-badge[label="Needs attention"]')).toBeTruthy()
  })
})
