// @vitest-environment jsdom
import { render } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import '@unofficialbox/box-open-elements/run-trace'
import { RunTimeline } from './RunTimeline'
import { presentProviderProgress } from './runTimelineModel'
import { latestActivityEvents } from './liveActivityModel'
import type { RunTurn } from '@unofficialbox/box-open-elements/patterns/run/progress'

describe('presentProviderProgress', () => {
  it('keeps Salesforce active while a managed package is still installing', () => {
    const providers = presentProviderProgress([
      { id: 'box', name: 'Box', configured: true, verified: true, ready: true },
      { id: 'salesforce', name: 'Salesforce', configured: true, verified: true, ready: true },
    ], {
      id: 'deploy-1', action: 'deploy', status: 'running', providers: [{ name: 'box', status: 'present' }, { name: 'salesforce', status: 'needs deployment' }],
    }, [
      { sequence: 1, at: '2026-08-26T00:00:00Z', type: 'activity', provider: 'box', message: 'Box configuration applied', status: 'running', progressState: 'completed' },
      { sequence: 2, at: '2026-08-26T00:00:01Z', type: 'activity', provider: 'salesforce', message: 'Applying Salesforce configuration', status: 'running', progressState: 'running' },
      { sequence: 3, at: '2026-08-26T00:02:08Z', type: 'activity', provider: 'salesforce', component: 'Managed Package:Box for Salesforce 5.43', message: 'Salesforce reports in progress · 2m7s elapsed', status: 'running', progressState: 'running' },
    ])

    expect(providers.map((provider) => provider.state)).toEqual(['complete', 'active'])
    expect(providers[1].components[0]).toMatchObject({ state: 'running', message: 'Salesforce reports in progress · 2m7s elapsed' })
  })

  it('marks an interrupted component as failed when its provider fails', () => {
    const providers = presentProviderProgress([{ id: 'salesforce', name: 'Salesforce', configured: true, verified: true, ready: true }], {
      id: 'run-1', action: 'validate', status: 'failed', providers: [{ name: 'salesforce', status: 'failed' }],
    }, [{ sequence: 1, at: '2026-08-24T00:00:00Z', type: 'activity', provider: 'salesforce', message: 'Checking installed package version', status: 'running', component: 'Managed Package:Box for Salesforce', progressState: 'running' }])

    expect(providers[0].state).toBe('failed')
    expect(providers[0].components[0]).toMatchObject({ state: 'failed', message: 'Validation stopped before this check completed' })
  })

  it('maps the latest provider update into the run summary', () => {
    const { container } = render(<RunTimeline providers={[{
      id: 'box', name: 'Box', state: 'active', components: [], updates: [
        { sequence: 1, at: '2026-08-24T00:00:00Z', type: 'activity', provider: 'box', message: 'Inspecting the Box workspace', status: 'running', progressState: 'activity' },
        { sequence: 2, at: '2026-08-24T00:00:01Z', type: 'activity', provider: 'box', message: 'Checking metadata templates', status: 'running', component: 'Metadata Template:Contract', progressState: 'running' },
      ],
    }]} />)

    const summary = container.querySelector('box-run-summary') as HTMLElement & { turn: RunTurn; open: boolean }
    expect(summary.open).toBe(true)
    expect(summary.turn.steps).toEqual([
      expect.objectContaining({ id: 'box', title: 'Box', description: 'Checking metadata templates', status: 'running' }),
    ])
  })

  it('passes the running state through the public run trace contract', () => {
    const { container } = render(<RunTimeline providers={[{
      id: 'salesforce', name: 'Salesforce', state: 'active', components: [], updates: [
        { sequence: 1, at: '2026-08-24T00:00:00Z', type: 'activity', provider: 'salesforce', message: 'Applying Salesforce configuration', status: 'running', progressState: 'running' },
      ],
    }]} />)

    const trace = container.querySelector('box-run-trace') as HTMLElement & { steps: Array<{ id: string; status: string }> }
    expect(trace.steps).toEqual([expect.objectContaining({ id: 'salesforce', status: 'running' })])
  })

  it('updates the run summary when a newer activity arrives', () => {
    const initialProviders = [{
      id: 'box', name: 'Box', state: 'active' as const, components: [], updates: [
        { sequence: 1, at: '2026-08-24T00:00:00Z', type: 'activity' as const, provider: 'box', message: 'Inspecting the Box workspace', status: 'running' as const, progressState: 'activity' as const },
      ],
    }]
    const { rerender, container } = render(<RunTimeline providers={initialProviders} />)
    const summary = container.querySelector('box-run-summary') as HTMLElement & { turn: RunTurn }

    rerender(<RunTimeline providers={[{
      ...initialProviders[0], updates: [...initialProviders[0].updates, { sequence: 2, at: '2026-08-24T00:00:01Z', type: 'activity', provider: 'box', message: 'Checking metadata templates', status: 'running', progressState: 'running' }],
    }]} />)

    expect(summary.turn.steps[0]).toMatchObject({ description: 'Checking metadata templates', status: 'running' })
  })

  it('replaces an in-progress activity with its terminal update', () => {
    const events = latestActivityEvents([
      { sequence: 1, at: '2026-08-24T00:00:00Z', type: 'activity', provider: 'box', providerName: 'Box', component: 'Metadata Template:CLM Redline Review', message: 'Checking Metadata Template:CLM Redline Review', status: 'running', progressState: 'running' },
      { sequence: 2, at: '2026-08-24T00:00:01Z', type: 'activity', provider: 'box', providerName: 'Box', component: 'Metadata Template:CLM Redline Review', message: 'Ready to deploy', status: 'running', progressState: 'completed' },
    ])

    expect(events).toHaveLength(1)
    expect(events[0]).toMatchObject({ sequence: 2, message: 'Ready to deploy', progressState: 'completed' })
  })
})
