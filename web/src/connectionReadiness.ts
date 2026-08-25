import type { ConnectionSummary, DeploymentPlan } from './types'

export function refreshProviderReadiness(plan: DeploymentPlan, connections: ConnectionSummary[], providerID: 'box' | 'salesforce'): DeploymentPlan {
  const connection = connections.find((candidate) => candidate.name.toLowerCase() === providerID)
  return {
    ...plan,
    components: plan.components.map((component) => component.id === providerID
      ? { ...component, configured: connection?.configured ?? false, verified: connection?.verified ?? false, ready: connection?.verified ?? false }
      : component),
  }
}
