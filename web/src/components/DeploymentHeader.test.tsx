// @vitest-environment jsdom
import { render } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
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
    const { container } = render(<DeploymentHeader plan={plan} activePhase="Choose" run={null} />)

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

  it('maps failed validation into the BOE path error state', () => {
    const { container } = render(<DeploymentHeader plan={plan} activePhase="Review" run={failedValidation} />)

    const path = container.querySelector('box-path') as (HTMLElement & { stages: Array<{ id: string; label: string }>; states: string[] }) | null

    expect(path?.getAttribute('current')).toBe('Review')
    expect(path?.hasAttribute('has-error')).toBe(true)
    expect(path?.stages.map((stage) => stage.label)).toEqual(['Choose', 'Connect', 'Configure', 'Validate', 'Deploy', 'Summary'])
    expect(path?.states).toEqual(['complete', 'complete', 'complete', 'error', 'upcoming', 'upcoming'])
  })

  it('marks deployment and summary complete after deployment', () => {
    const completedDeployment: DispatchRun = { id: 'deploy-1', action: 'deploy', status: 'completed', providers: [{ name: 'box', status: 'present', remainingCount: 0, manualItemCount: 0 }] }
    const { container } = render(<DeploymentHeader plan={plan} activePhase="Summary" run={completedDeployment} />)

    const path = container.querySelector('box-path') as (HTMLElement & { states: string[] }) | null
    expect(container.querySelector('box-badge[label="Deployment complete"][tone="success"]')).toBeTruthy()
    expect(path?.getAttribute('current')).toBe('Summary')
    expect(path?.states).toEqual(['complete', 'complete', 'complete', 'complete', 'complete', 'current'])
  })

  it('marks the summary as needing attention when deployment work remains', () => {
    const partialDeployment: DispatchRun = { id: 'deploy-2', action: 'deploy', status: 'completed', providers: [{ name: 'box', status: 'present', remainingCount: 1, manualItemCount: 1 }] }
    const { container } = render(<DeploymentHeader plan={plan} activePhase="Summary" run={partialDeployment} />)

    const path = container.querySelector('box-path') as (HTMLElement & { states: string[] }) | null
    expect(container.querySelector('box-badge[label="Needs attention"][tone="error"]')).toBeTruthy()
    expect(path?.getAttribute('current')).toBe('Summary')
    expect(path?.hasAttribute('has-error')).toBe(true)
    expect(path?.states.at(-1)).toBe('error')
  })
})
