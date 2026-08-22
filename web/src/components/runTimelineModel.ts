import type { DeploymentPlan, DispatchRun, RunEvent } from '../types'

export type ProviderProgress = {
  id: string
  name: string
  state: 'pending' | 'active' | 'complete' | 'failed'
  updates: RunEvent[]
}

export function presentProviderProgress(components: DeploymentPlan['components'], run: DispatchRun | null, events: RunEvent[]): ProviderProgress[] {
  const currentProvider = [...events].reverse().find((event) => event.type === 'activity' && event.provider)?.provider
  return components.map((component) => {
    const result = (run?.providers ?? []).find((provider) => provider.name.toLowerCase() === component.id)
    const componentEvents = events.filter((event) => event.provider === component.id)
    const updates = componentEvents.filter((event) => event.type === 'activity')
    if (result) return { id: component.id, name: component.name, state: result.status === 'failed' ? 'failed' : 'complete', updates }
    if (run?.status === 'failed' && currentProvider === component.id) return { id: component.id, name: component.name, state: 'failed', updates }
    if (providerFinished(updates.at(-1)?.message)) return { id: component.id, name: component.name, state: 'complete', updates }
    if (run?.status === 'completed' && updates.length > 0) return { id: component.id, name: component.name, state: 'complete', updates }
    if (currentProvider === component.id) return { id: component.id, name: component.name, state: 'active', updates }
    return { id: component.id, name: component.name, state: 'pending', updates }
  })
}

function providerFinished(message = '') {
  return /(?:validation complete|configuration applied|requires no supported changes)$/i.test(message)
}
