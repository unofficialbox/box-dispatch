import { describe, expect, it } from 'vitest'
import { guardedWorkflowPhase, resumeWorkflowPhase } from './workflowResume'
import type { ConnectionSummary, DeploymentPlan } from './types'

const plan: DeploymentPlan = {
  exists: true,
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

  it('redirects an attempt to reach Review or Deploy to Connect when connections are missing', () => {
    expect(guardedWorkflowPhase('Review', plan, noConnections)).toBe('Connect')
    expect(guardedWorkflowPhase('Deploy', plan, noConnections)).toBe('Connect')
  })
})
