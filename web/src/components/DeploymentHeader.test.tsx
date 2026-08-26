// @vitest-environment jsdom
import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { DeploymentHeader } from './DeploymentHeader'
import type { DeploymentPlan, DispatchRun } from '../types'

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

const failedValidation: DispatchRun = {
  id: 'validation-1',
  action: 'validate',
  status: 'failed',
  providers: [{ name: 'salesforce', status: 'failed' }],
}

describe('DeploymentHeader', () => {
  it('uses the BOE select for the environment control', () => {
    const { container } = render(<DeploymentHeader plan={plan} activePhase="Choose" run={null} onPhaseChange={vi.fn()} />)

    const environment = container.querySelector('box-select.environment')
    expect(environment).not.toBeNull()
    expect(environment?.getAttribute('label')).toBe('Environment')
    expect(environment?.getAttribute('value')).toBe('development')
    expect(container.querySelector('button.environment')).toBeNull()
  })

  it('shows a failed validation as a red failed step', () => {
    const { container } = render(<DeploymentHeader plan={plan} activePhase="Review" run={failedValidation} onPhaseChange={vi.fn()} />)

    const validate = screen.getByRole('button', { name: 'Validate, failed' })
    const step = validate.closest('li')
    const symbol = validate.querySelector('.workflow-node-symbol')

    expect(step?.classList.contains('active')).toBe(true)
    expect(step?.classList.contains('failed')).toBe(true)
    expect(symbol?.textContent).toBe('×')
    expect(symbol?.getAttribute('aria-hidden')).toBe('true')
    expect(container.querySelector('.workflow-indicator li.failed')).toBe(step)
  })

  it('adds a completed Summary step after deployment', () => {
    const completedDeployment: DispatchRun = { id: 'deploy-1', action: 'deploy', status: 'completed', providers: [] }
    render(<DeploymentHeader plan={plan} activePhase="Summary" run={completedDeployment} onPhaseChange={vi.fn()} />)

    expect(screen.getByRole('button', { name: 'Deploy, complete' })).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Summary, complete' })).toBeTruthy()
  })
})
