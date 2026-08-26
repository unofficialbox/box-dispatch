import { useEffect, useRef } from 'react'
import type { RunStep, RunStepStatus } from '@unofficialbox/box-open-elements'
import type { ComponentProgress, ProviderProgress } from './runTimelineModel'
import { LiveActivityFeed } from './LiveActivityFeed'

export function RunTimeline({ providers }: { providers: ProviderProgress[] }) {
  const traceRef = useRef<HTMLElement>(null)
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

  useEffect(() => {
    const trace = traceRef.current
    if (!trace) return
    let cancelled = false
    let style: HTMLStyleElement | null = null
    const installRunningIndicator = () => {
      if (cancelled || !trace.shadowRoot || trace.shadowRoot.querySelector('[data-dispatch-running-pulse]')) return
      style = document.createElement('style')
      style.dataset.dispatchRunningPulse = ''
      style.textContent = `
        @keyframes dispatch-provider-pulse {
          0%, 100% { box-shadow: 0 0 0 0 color-mix(in srgb, var(--run-tone) 30%, transparent); }
          50% { box-shadow: 0 0 0 8px color-mix(in srgb, var(--run-tone) 10%, transparent); }
        }
        [part="step"][data-status="running"] [part="marker"] {
          animation: dispatch-provider-pulse 1.45s ease-in-out infinite;
        }
        @media (prefers-reduced-motion: reduce) {
          [part="step"][data-status="running"] [part="marker"] {
            animation: none;
            box-shadow: 0 0 0 4px color-mix(in srgb, var(--run-tone) 14%, transparent);
          }
        }
      `
      trace.shadowRoot.append(style)
    }
    installRunningIndicator()
    void customElements.whenDefined('box-run-trace').then(installRunningIndicator)
    return () => {
      cancelled = true
      style?.remove()
    }
  }, [])

  return <><box-run-trace ref={traceRef} className="dispatch-run-trace" heading="Provider progress" steps={steps}></box-run-trace><LiveActivityFeed providers={providers}/></>
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

function formatComponentName(component: ComponentProgress) {
  const separator = component.id.indexOf(':')
  const name = separator < 0 ? component.id : `${component.id.slice(separator + 1)} · ${component.id.slice(0, separator)}`
  return component.message && component.message !== name ? `${name} — ${component.message}` : name
}
