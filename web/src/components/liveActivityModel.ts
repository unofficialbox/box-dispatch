import type { RunEvent } from '../types'

export type FeedItem = RunEvent & { providerName: string }

// A component emits a running event followed by its terminal result. Keep the
// latest state for each provider/component pair so the live feed is a current
// view of work, not a duplicate event history.
export function latestActivityEvents(events: FeedItem[]): FeedItem[] {
  const latestByActivity = new Map<string, FeedItem>()
  for (const event of events) {
    const key = `${event.provider ?? event.providerName}:${event.component ?? '__provider__'}`
    latestByActivity.set(key, event)
  }
  return [...latestByActivity.values()].sort((left, right) => left.sequence - right.sequence)
}
