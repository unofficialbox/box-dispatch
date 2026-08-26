import { useLayoutEffect, useMemo, useRef } from 'react'
import type { RunEvent } from '../types'
import type { ProviderProgress } from './runTimelineModel'
import { latestActivityEvents, type FeedItem } from './liveActivityModel'

export function LiveActivityFeed({ providers }: { providers: ProviderProgress[] }) {
  const events: FeedItem[] = useMemo(() => providers.flatMap((provider) => provider.updates.map((event) => ({ ...event, providerName: provider.name }))).sort((left, right) => left.sequence - right.sequence), [providers])
  const visibleEvents = useMemo(() => latestActivityEvents(events).slice(-12), [events])
  const lastEventSequence = visibleEvents.at(-1)?.sequence
  const logRef = useRef<HTMLDivElement>(null)
  useLayoutEffect(() => {
    const log = logRef.current
    if (log) log.scrollTop = log.scrollHeight
  }, [visibleEvents.length, lastEventSequence])
  if (events.length === 0) return null
  return <section className="live-activity-feed" aria-label="Recent validation activity" aria-live="polite">
    <header><div><h3>Live activity</h3><p>Latest provider and component updates.</p></div></header>
    <div className="live-activity-log" ref={logRef} tabIndex={0} aria-label="Live validation log"><ol>{visibleEvents.map((event) => <li className={eventState(event)} key={event.sequence}>
      <span className="activity-indicator">
        {isWorking(event) && <box-spinner aria-label="Working" label="" size="small" />}
      </span>
      <div><strong>{event.component || event.providerName}</strong><p>{event.message}</p></div>
      <box-badge label={eventLabel(event)} tone={eventTone(event)} />
    </li>)}</ol></div>
  </section>
}


function eventState(event: RunEvent) {
  return event.progressState === 'failed' ? 'failed' : event.progressState === 'completed' ? 'complete' : event.progressState === 'running' ? 'running' : 'activity'
}

function isWorking(event: RunEvent) { return event.progressState === 'running' || event.progressState === 'activity' }

function eventLabel(event: RunEvent) {
  if (event.progressState === 'completed') return 'Complete'
  if (event.progressState === 'failed') return 'Failed'
  if (event.progressState === 'running') return 'Working'
  return 'Update'
}

function eventTone(event: RunEvent) {
  if (event.progressState === 'completed') return 'success'
  if (event.progressState === 'failed') return 'error'
  if (event.progressState === 'running') return 'inprogress'
  return 'neutral'
}
