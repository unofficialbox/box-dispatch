import { useEffect, useMemo, useRef } from 'react'
import type { RunStep, RunStepStatus } from '@unofficialbox/box-open-elements'
import '@unofficialbox/box-open-elements/run-summary'
import type { ComponentProgress, ProviderProgress } from './runTimelineModel'
import type { RunTurn, AgentTodo } from '@unofficialbox/box-open-elements/patterns/run/progress'

export function RunTimeline({ providers }: { providers: ProviderProgress[] }) {
  const summaryRef = useRef<HTMLElementTagNameMap['box-run-summary']>(null)

  const turn = useMemo<RunTurn>(() => {
    const steps: RunStep[] = providers.map((provider) => {
      const latest = provider.updates.at(-1)
      const first = provider.updates.at(0)
      const terminal = provider.state === 'complete' || provider.state === 'failed'
      return {
        id: provider.id,
        title: provider.name,
        description: latest?.message ?? `Waiting to begin ${provider.name} work`,
        status: providerStatus(provider.state),
        startedAt: first?.at,
        finishedAt: terminal ? latest?.at : undefined,
        children: provider.components.map((component) => ({
          id: component.id,
          label: formatComponentName(component),
          progress: component.state === 'completed' ? 100 : undefined,
          status: componentStatus(component.state),
        })),
      }
    })
    const todos: AgentTodo[] = providers.flatMap((provider) =>
      provider.components.map((component) => ({
        id: `${provider.id}-${component.id}`,
        content: formatComponentName(component),
        status: componentToTodoStatus(component.state),
      }))
    )
    const firstUpdate = providers[0]?.updates[0]
    const lastUpdate = providers.flatMap(p => p.updates).sort((a, b) =>
      new Date(b.at).getTime() - new Date(a.at).getTime()
    )[0]
    const hasAnyTerminal = providers.some(p => p.state === 'complete' || p.state === 'failed')
    return {
      steps,
      todos,
      startedAt: firstUpdate ? new Date(firstUpdate.at).getTime() : 0,
      endedAt: hasAnyTerminal && lastUpdate ? new Date(lastUpdate.at).getTime() : undefined,
      incomplete: !hasAnyTerminal,
    }
  }, [providers])

  const steps = turn.steps

  useEffect(() => {
    if (summaryRef.current) {
      summaryRef.current.turn = turn
      summaryRef.current.open = true
    }
  }, [turn])

  return <>
    <box-run-trace className="dispatch-run-trace" heading="Provider progress" steps={steps}></box-run-trace>
    <box-run-summary ref={summaryRef} className="dispatch-run-summary" open></box-run-summary>
  </>
}

function providerStatus(state: ProviderProgress['state']): RunStepStatus {
  if (state === 'complete') return 'succeeded'
  if (state === 'failed') return 'failed'
  if (state === 'active') return 'running'
  return 'pending'
}

function componentStatus(state: ComponentProgress['state']): RunStepStatus {
  if (state === 'completed') return 'succeeded'
  if (state === 'failed') return 'failed'
  if (state === 'running') return 'running'
  return 'pending'
}

function componentToTodoStatus(state: ComponentProgress['state']): AgentTodo['status'] {
  if (state === 'completed') return 'completed'
  if (state === 'failed') return 'skipped'
  if (state === 'running') return 'in_progress'
  return 'pending'
}

function formatComponentName(component: ComponentProgress) {
  const separator = component.id.indexOf(':')
  const name = separator < 0 ? component.id : `${component.id.slice(separator + 1)} · ${component.id.slice(0, separator)}`
  return component.message && component.message !== name ? `${name} — ${component.message}` : name
}
