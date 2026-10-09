import type { ConnectionSummary, DeploymentPlan } from './types'
import { refreshProviderReadiness } from './connectionReadiness'

const liveProviders = ['box', 'salesforce'] as const

type FetchConnectionStatus = (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>

export function markConnectionsChecking(connections: ConnectionSummary[]): ConnectionSummary[] {
  return connections.map((connection) => {
    const provider = connection.name.toLowerCase()
    if (!liveProviders.includes(provider as (typeof liveProviders)[number])) return connection
    return {
      ...connection,
      verified: false,
      status: 'Checking',
      connections: connection.connections?.map((saved) => saved.selected ? { ...saved, status: 'Checking' } : saved),
      orgs: connection.orgs?.map((saved) => saved.selected ? { ...saved, status: 'Checking' } : saved),
    }
  })
}

export function markPlanConnectionsChecking(plan: DeploymentPlan): DeploymentPlan {
  return {
    ...plan,
    components: plan.components.map((component) => liveProviders.includes(component.id as (typeof liveProviders)[number])
      ? { ...component, verified: false, ready: false }
      : component),
  }
}

export async function revalidateConnections(fetchConnectionStatus: FetchConnectionStatus = fetch, signal?: AbortSignal): Promise<ConnectionSummary[]> {
  const checkResults = await Promise.allSettled(liveProviders.map((provider) => fetchConnectionStatus(`/api/connections/${provider}/check`, { method: 'POST', signal })))
  const failedProviders = new Set(liveProviders.filter((_, index) => {
    const result = checkResults[index]
    return result.status === 'rejected' || !result.value.ok
  }))
  const response = await fetchConnectionStatus('/api/connections', { signal })
  if (!response.ok) throw new Error('Connection readiness could not be refreshed.')
  const connections = await response.json() as ConnectionSummary[]
  return connections.map((connection) => {
    const provider = connection.name.toLowerCase() as (typeof liveProviders)[number]
    if (!failedProviders.has(provider)) return connection
    return {
      ...connection,
      verified: false,
      status: 'Not ready',
      connections: connection.connections?.map((saved) => saved.selected ? { ...saved, status: 'Not ready' } : saved),
      orgs: connection.orgs?.map((saved) => saved.selected ? { ...saved, status: 'Not ready' } : saved),
    }
  })
}

export function refreshPlanReadiness(plan: DeploymentPlan, connections: ConnectionSummary[]): DeploymentPlan {
  return liveProviders.reduce((current, provider) => refreshProviderReadiness(current, connections, provider), plan)
}
