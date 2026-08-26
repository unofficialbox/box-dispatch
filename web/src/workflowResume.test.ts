import { describe, expect, it } from 'vitest'
import { guardedWorkflowPhase, resumeWorkflowPhase } from './workflowResume'
import type { ConnectionSummary, DeploymentPlan } from './types'

const plan: DeploymentPlan = {
  exists: true,
  name: 'Northstar CLM rollout',
  templateId: 'clm',
  template: 'CLM deployment',
  repository: 'https://example.test/clm',
  strategy: 'reuse',
  components: [
    { id: 'box', name: 'Box', configured: true, verified: true, ready: true },
    { id: 'salesforce', name: 'Salesforce', configured: true, verified: true, ready: true },
  ],
}

const noConnections: ConnectionSummary[] = [
  { name: 'Box', configured: false, verified: false },
  { name: 'Salesforce', configured: false, verified: false },
]

const readyConnections: ConnectionSummary[] = [
  { name: 'Box', configured: true, verified: true },
  { name: 'Salesforce', configured: true, verified: true },
]

describe('workflow resume phase', () => {
  it('returns Connect for a saved plan without active connections', () => {
    expect(resumeWorkflowPhase(plan, noConnections)).toBe('Connect')
  })

  it('returns Review only when every selected system is verified', () => {
    expect(resumeWorkflowPhase(plan, readyConnections)).toBe('Review')
  })

  it('returns to an active deployment after a browser refresh', () => {
    expect(resumeWorkflowPhase(plan, readyConnections, { id: 'deploy-1', action: 'deploy', status: 'running', providers: [] })).toBe('Deploy')
  })

  it('opens Summary for the latest completed deployment', () => {
    expect(resumeWorkflowPhase(plan, readyConnections, { id: 'deploy-1', action: 'deploy', status: 'completed', providers: [] })).toBe('Summary')
  })

  it('redirects an attempt to reach Review or Deploy to Connect when connections are missing', () => {
    expect(guardedWorkflowPhase('Review', plan, noConnections)).toBe('Connect')
    expect(guardedWorkflowPhase('Deploy', plan, noConnections)).toBe('Connect')
  })
})
