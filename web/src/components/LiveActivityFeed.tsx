import { useLayoutEffect, useMemo, useRef } from 'react'
import '@unofficialbox/box-open-elements/timeline'
import type { TimelineEvent, TimelineTone } from '@unofficialbox/box-open-elements/patterns/timeline'
import type { RunEvent } from '../types'
import type { ProviderProgress } from './runTimelineModel'
import { latestActivityEvents, type FeedItem } from './liveActivityModel'

export function LiveActivityFeed({ providers }: { providers: ProviderProgress[] }) {
  const events: FeedItem[] = useMemo(() => providers.flatMap((provider) => provider.updates.map((event) => ({ ...event, providerName: provider.name }))).sort((left, right) => left.sequence - right.sequence), [providers])
  const visibleEvents = useMemo(() => latestActivityEvents(events).slice(-12), [events])
  const timelineEvents = useMemo<TimelineEvent[]>(() => visibleEvents.map((event) => ({
    id: String(event.sequence),
    action: event.component || event.providerName,
    actor: { name: event.providerName },
    summary: event.message,
    timestamp: event.at,
    tone: eventTone(event),
    badge: eventLabel(event),
  })), [visibleEvents])
  const lastEventSequence = visibleEvents.at(-1)?.sequence
  const logRef = useRef<HTMLElementTagNameMap['box-timeline']>(null)
  useLayoutEffect(() => {
    const log = logRef.current
    if (log) {
      log.events = timelineEvents
      log.scrollTop = log.scrollHeight
    }
  }, [lastEventSequence, timelineEvents])
  if (events.length === 0) return null
  return <section className="live-activity-feed" aria-label="Recent validation activity" aria-live="polite">
    <box-timeline className="live-activity-log" ref={logRef} heading="Live activity" tabIndex={0} aria-label="Live validation log"></box-timeline>
  </section>
}

function eventLabel(event: RunEvent) {
  if (event.progressState === 'completed') return 'Complete'
  if (event.progressState === 'failed') return 'Failed'
  if (event.progressState === 'running') return 'Working'
  return 'Update'
}

function eventTone(event: RunEvent): TimelineTone {
  if (event.progressState === 'completed') return 'success'
  if (event.progressState === 'failed') return 'error'
  if (event.progressState === 'running') return 'brand'
  return 'neutral'
}
