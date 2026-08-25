// @vitest-environment jsdom
import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { RunTimeline } from './RunTimeline'
import { presentProviderProgress } from './runTimelineModel'
import { latestActivityEvents } from './liveActivityModel'

describe('presentProviderProgress', () => {
  it('marks an interrupted component as failed when its provider fails', () => {
    const providers = presentProviderProgress([{ id: 'salesforce', name: 'Salesforce', configured: true, verified: true, ready: true }], {
      id: 'run-1', action: 'validate', status: 'failed', providers: [{ name: 'salesforce', status: 'failed' }],
    }, [{ sequence: 1, at: '2026-08-24T00:00:00Z', type: 'activity', provider: 'salesforce', message: 'Checking installed package version', status: 'running', component: 'Managed Package:Box for Salesforce', progressState: 'running' }])

    expect(providers[0].state).toBe('failed')
    expect(providers[0].components[0]).toMatchObject({ state: 'failed', message: 'Validation stopped before this check completed' })
  })

  it('renders each recent provider update in the live activity feed', () => {
    render(<RunTimeline providers={[{
      id: 'box', name: 'Box', state: 'active', components: [], updates: [
        { sequence: 1, at: '2026-08-24T00:00:00Z', type: 'activity', provider: 'box', message: 'Inspecting the Box workspace', status: 'running', progressState: 'activity' },
        { sequence: 2, at: '2026-08-24T00:00:01Z', type: 'activity', provider: 'box', message: 'Checking metadata templates', status: 'running', component: 'Metadata Template:Contract', progressState: 'running' },
      ],
    }]} />)

    expect(screen.getByRole('region', { name: 'Recent validation activity' })).toBeTruthy()
    expect(screen.getByText('Inspecting the Box workspace')).toBeTruthy()
    expect(screen.getByText('Checking metadata templates')).toBeTruthy()
    expect(document.querySelector('box-badge[label="Working"][tone="inprogress"]')).toBeTruthy()
    expect(document.querySelector('box-spinner[aria-label="Working"][size="small"]')).toBeTruthy()
  })

  it('tails the newest live activity update', () => {
    const initialProviders = [{
      id: 'box', name: 'Box', state: 'active' as const, components: [], updates: [
        { sequence: 1, at: '2026-08-24T00:00:00Z', type: 'activity' as const, provider: 'box', message: 'Inspecting the Box workspace', status: 'running' as const, progressState: 'activity' as const },
      ],
    }]
    const { rerender, container } = render(<RunTimeline providers={initialProviders} />)
    const log = container.querySelector('.live-activity-log') as HTMLDivElement
    Object.defineProperty(log, 'scrollHeight', { configurable: true, value: 480 })
    Object.defineProperty(log, 'scrollTop', { configurable: true, writable: true, value: 0 })

    rerender(<RunTimeline providers={[{
      ...initialProviders[0], updates: [...initialProviders[0].updates, { sequence: 2, at: '2026-08-24T00:00:01Z', type: 'activity', provider: 'box', message: 'Checking metadata templates', status: 'running', progressState: 'running' }],
    }]} />)

    expect(log.scrollTop).toBe(480)
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
