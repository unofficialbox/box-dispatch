import type { DeploymentPlan, DispatchRun, RunEvent } from '../types'

export type ProviderProgress = {
  id: string
  name: string
  state: 'pending' | 'active' | 'complete' | 'failed'
  updates: RunEvent[]
  components: ComponentProgress[]
}

export type ComponentProgress = {
  id: string
  state: 'queued' | 'running' | 'completed' | 'failed'
  message: string
  at: string
  sequence: number
  firstSequence: number
}

export function presentProviderProgress(components: DeploymentPlan['components'], run: DispatchRun | null, events: RunEvent[]): ProviderProgress[] {
  const currentProvider = [...events].reverse().find((event) => event.type === 'activity' && event.provider)?.provider
  return components.map((component) => {
    const result = (run?.providers ?? []).find((provider) => provider.name.toLowerCase() === component.id)
    const componentEvents = events.filter((event) => event.provider === component.id)
    const updates = componentEvents.filter((event) => event.type === 'activity')
    const componentProgress = presentComponents(updates)
    if (result) {
      const failed = result.status === 'failed'
      return { id: component.id, name: component.name, state: failed ? 'failed' : 'complete', updates, components: finishInterruptedComponents(componentProgress, failed) }
    }
    if (run?.status === 'failed' && currentProvider === component.id) return { id: component.id, name: component.name, state: 'failed', updates, components: finishInterruptedComponents(componentProgress, true) }
    if (providerFinished(updates.at(-1)?.message)) return { id: component.id, name: component.name, state: 'complete', updates, components: componentProgress }
    if (run?.status === 'completed' && updates.length > 0) return { id: component.id, name: component.name, state: 'complete', updates, components: componentProgress }
    if (currentProvider === component.id) return { id: component.id, name: component.name, state: 'active', updates, components: componentProgress }
    return { id: component.id, name: component.name, state: 'pending', updates, components: componentProgress }
  })
}

function finishInterruptedComponents(components: ComponentProgress[], failed: boolean) {
  if (!failed) return components
  return components.map((component) => component.state === 'running' ? { ...component, state: 'failed' as const, message: 'Validation stopped before this check completed' } : component)
}

function presentComponents(events: RunEvent[]): ComponentProgress[] {
  const byComponent = new Map<string, ComponentProgress>()
  for (const event of events) {
    if (!event.component || !event.progressState || event.progressState === 'activity') continue
    const previous = byComponent.get(event.component)
    byComponent.set(event.component, {
      id: event.component,
      state: event.progressState,
      message: event.message,
      at: event.at,
      sequence: event.sequence,
      firstSequence: previous?.firstSequence ?? event.sequence,
    })
  }
  return [...byComponent.values()].sort((left, right) => left.firstSequence - right.firstSequence)
}

function providerFinished(message = '') {
  return /(?:validation complete|configuration applied|requires no supported changes)$/i.test(message)
}
