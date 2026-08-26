import type { ConnectionSummary, DeploymentPlan, DispatchRun, Phase } from './types'

export function connectionsReadyForPlan(plan: DeploymentPlan, connections: ConnectionSummary[]) {
  return plan.components.length > 0 && plan.components.every((component) =>
    connections.some((connection) => connection.name.toLowerCase() === component.name.toLowerCase() && connection.verified),
  )
}

export function resumeWorkflowPhase(plan: DeploymentPlan, connections: ConnectionSummary[], run: DispatchRun | null = null): Phase {
  if (!plan.exists || plan.components.length === 0) return 'Choose'
  if (!connectionsReadyForPlan(plan, connections)) return 'Connect'
  if (!run) return 'Review'
  if (run.action === 'deploy' && run.status === 'completed') return 'Summary'
  return 'Deploy'
}

export function guardedWorkflowPhase(requested: Phase, plan: DeploymentPlan, connections: ConnectionSummary[]): Phase {
  if ((requested === 'Review' || requested === 'Deploy') && !connectionsReadyForPlan(plan, connections)) return 'Connect'
  return requested
}
