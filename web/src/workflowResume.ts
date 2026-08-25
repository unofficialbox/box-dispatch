import type { ConnectionSummary, DeploymentPlan, Phase } from './types'

export function connectionsReadyForPlan(plan: DeploymentPlan, connections: ConnectionSummary[]) {
  return plan.components.length > 0 && plan.components.every((component) =>
    connections.some((connection) => connection.name.toLowerCase() === component.name.toLowerCase() && connection.verified),
  )
}

export function resumeWorkflowPhase(plan: DeploymentPlan, connections: ConnectionSummary[]): Phase {
  if (!plan.exists || plan.components.length === 0) return 'Choose'
  return connectionsReadyForPlan(plan, connections) ? 'Review' : 'Connect'
}

export function guardedWorkflowPhase(requested: Phase, plan: DeploymentPlan, connections: ConnectionSummary[]): Phase {
  if ((requested === 'Review' || requested === 'Deploy') && !connectionsReadyForPlan(plan, connections)) return 'Connect'
  return requested
}
