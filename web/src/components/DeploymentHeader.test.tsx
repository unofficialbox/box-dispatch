// @vitest-environment jsdom
import { fireEvent, render } from '@testing-library/react'
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
    const breadcrumb = container.querySelector('box-breadcrumb') as (HTMLElement & { items: Array<{ label: string; href?: string }> }) | null
    expect(breadcrumb?.items).toEqual([
      { label: 'Deployments', href: '#workspace', value: 'deployments' },
      { label: 'Northstar CLM rollout', value: 'current' },
    ])
  })

  it('maps failed validation and unavailable deployment phases into BOE step states', () => {
    const { container } = render(<DeploymentHeader plan={plan} activePhase="Review" run={failedValidation} onPhaseChange={vi.fn()} />)

    const progress = container.querySelector('box-progress-steps') as (HTMLElement & { items: Array<{ label: string; status?: string }>; value: string }) | null

    expect(progress?.value).toBe('Review')
    expect(progress?.items.find((item) => item.label === 'Validate')?.status).toBe('failed')
    expect(progress?.items.find((item) => item.label === 'Deploy')?.status).toBe('disabled')
    expect(progress?.items.find((item) => item.label === 'Summary')?.status).toBe('disabled')
  })

  it('marks deployment and summary complete after deployment', () => {
    const completedDeployment: DispatchRun = { id: 'deploy-1', action: 'deploy', status: 'completed', providers: [] }
    const { container } = render(<DeploymentHeader plan={plan} activePhase="Summary" run={completedDeployment} onPhaseChange={vi.fn()} />)

    const progress = container.querySelector('box-progress-steps') as (HTMLElement & { items: Array<{ label: string; status?: string }>; value: string }) | null
    expect(progress?.value).toBe('Summary')
    expect(progress?.items.find((item) => item.label === 'Deploy')?.status).toBe('complete')
    expect(progress?.items.find((item) => item.label === 'Summary')?.status).toBe('complete')
  })

  it('forwards BOE step changes to workflow navigation', () => {
    const onPhaseChange = vi.fn()
    const { container } = render(<DeploymentHeader plan={plan} activePhase="Choose" run={null} onPhaseChange={onPhaseChange} />)
    const progress = container.querySelector('box-progress-steps')

    fireEvent(progress!, new CustomEvent('value-changed', { detail: { value: 'Connect' } }))
    expect(onPhaseChange).toHaveBeenCalledWith('Connect')
  })
})
