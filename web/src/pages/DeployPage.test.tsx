// @vitest-environment jsdom
import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { DeployPage } from './DeployPage'
import type { DeploymentPlan, DispatchRun, RunEvent } from '../types'

const plan: DeploymentPlan = {
  exists: true,
  name: 'Northstar CLM rollout',
  templateId: 'clm',
  template: 'CLM deployment',
  repository: 'example/dispatch-template',
  strategy: 'reuse',
  components: [
    { id: 'box', name: 'Box', configured: true, verified: true, ready: true },
    { id: 'salesforce', name: 'Salesforce', configured: true, verified: true, ready: true },
  ],
}

const failedRun: DispatchRun = {
  id: 'web-20260825T120000Z-0001',
  action: 'validate',
  status: 'failed',
  providers: [{ name: 'salesforce', status: 'failed' }],
}

const events: RunEvent[] = [
  { sequence: 1, at: '2026-08-25T12:00:00Z', type: 'activity', provider: 'box', component: 'Authentication', message: 'Authentication verified', status: 'running', progressState: 'completed' },
  { sequence: 2, at: '2026-08-25T12:00:01Z', type: 'activity', provider: 'salesforce', component: 'Authentication', message: 'Salesforce authentication failed: session expired', status: 'failed', progressState: 'failed' },
]

describe('DeployPage', () => {
  it('makes authentication failure and recovery guidance visually explicit', () => {
    const { container } = render(<DeployPage plan={plan} run={failedRun} events={events} notice="" onApply={vi.fn()} onDiagnostics={vi.fn()} onViewChanges={vi.fn()} />)

    expect(screen.getByText('Authentication failed before configuration validation began.')).toBeTruthy()
    expect(screen.getByText('Salesforce authentication failed. Reconnect it before retrying.')).toBeTruthy()
    expect(container.querySelector('box-badge[label="Needs attention"][tone="error"]')).toBeTruthy()
    expect(container.querySelector('box-button[label="View diagnostic guidance"][tone="primary"]')).toBeTruthy()
  })

  it('keeps completion actions out of the live run surface', () => {
    const completedRun: DispatchRun = { id: 'deploy-1', action: 'deploy', status: 'completed', providers: [{ name: 'box', status: 'present' }, { name: 'salesforce', status: 'present' }] }
    const { container } = render(<DeployPage plan={plan} run={completedRun} events={[]} notice="" onApply={vi.fn()} onDiagnostics={vi.fn()} onViewChanges={vi.fn()} />)

    expect(screen.getByText('All selected systems finished successfully.')).toBeTruthy()
    expect(container.querySelector('box-button[label^="Open"]')).toBeNull()
  })

  it('offers an optional file diff after validation finds Salesforce changes', () => {
    const onViewChanges = vi.fn()
    const completedValidation: DispatchRun = { id: 'validate-1', action: 'validate', status: 'completed', changeCount: 1, providers: [{ name: 'box', status: 'present' }, { name: 'salesforce', status: 'needs deployment' }] }
    const changeEvents: RunEvent[] = [{ sequence: 1, at: '2026-08-25T12:00:00Z', type: 'activity', provider: 'salesforce', component: 'Settings:Communities', message: 'Configuration differs; will be updated', status: 'running', progressState: 'completed' }]
    const { container } = render(<DeployPage plan={plan} run={completedValidation} events={changeEvents} notice="" onApply={vi.fn()} onDiagnostics={vi.fn()} onViewChanges={onViewChanges} />)

    const button = container.querySelector<HTMLElement>('box-button[label="View file changes"]')
    expect(button).toBeTruthy()
    button?.click()
    expect(onViewChanges).toHaveBeenCalledWith('validate-1')
  })
})
