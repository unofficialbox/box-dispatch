import type { ProviderProgress } from './runTimelineModel'

export function RunTimeline({ providers }: { providers: ProviderProgress[] }) {
  return <ol className="run-timeline" aria-label="Live provider progress" aria-live="polite">
    {providers.map((provider, index) => <RunTimelineItem key={provider.id} provider={provider} final={index === providers.length - 1} />)}
  </ol>
}

function RunTimelineItem({ provider, final }: { provider: ProviderProgress; final: boolean }) {
  const latest = provider.updates.at(-1)
  const visibleUpdates = provider.updates.slice(-6)
  const expanded = provider.state === 'active' || provider.state === 'failed'
  return <li className={`run-timeline-item ${provider.state} ${final ? 'final' : ''}`}>
    <time>{latest ? formatRunTime(latest.at) : ''}</time>
    <span className="run-timeline-rail" aria-hidden="true">
      <span className="run-timeline-node">{provider.state === 'complete' ? '✓' : provider.state === 'failed' ? '!' : ''}</span>
    </span>
    <div className="run-timeline-content">
      <header className="run-provider-summary">
        <span className={`run-provider-mark ${provider.id}`}>{provider.id === 'salesforce' ? 'SF' : provider.name.slice(0, 1)}</span>
        <span className="run-provider-copy">
          <strong>{provider.name}</strong>
          <small>{latest?.message ?? pendingMessage(provider.name)}</small>
        </span>
        <box-badge label={stateLabel(provider.state)} tone={stateTone(provider.state)}></box-badge>
      </header>
      {expanded && <box-card className="run-activity-card">
        <section>
          <header>
            <div>
              <h3>{provider.state === 'failed' ? 'Recent activity' : 'Live activity'}</h3>
              <p>{provider.updates.length} update{provider.updates.length === 1 ? '' : 's'} received</p>
            </div>
            {provider.state === 'active' && <span className="run-live-pulse"><i></i>Following</span>}
          </header>
          {visibleUpdates.length > 0 ? <ol className="run-event-list">
            {visibleUpdates.map((event, index) => <li key={event.sequence} className={index === visibleUpdates.length - 1 ? 'current' : ''}>
              <span>{event.message}</span>
              <time>{formatRunTime(event.at)}</time>
            </li>)}
          </ol> : <p className="run-event-empty">Waiting for the first provider update.</p>}
        </section>
      </box-card>}
    </div>
  </li>
}

function pendingMessage(provider: string) {
  return `Waiting to begin ${provider} work`
}

function stateLabel(state: ProviderProgress['state']) {
  if (state === 'complete') return 'Complete'
  if (state === 'failed') return 'Failed'
  if (state === 'active') return 'In progress'
  return 'Queued'
}

function stateTone(state: ProviderProgress['state']) {
  if (state === 'complete') return 'success'
  if (state === 'failed') return 'danger'
  if (state === 'active') return 'info'
  return 'neutral'
}

function formatRunTime(value: string) {
  const timestamp = new Date(value)
  if (Number.isNaN(timestamp.getTime())) return 'Now'
  return new Intl.DateTimeFormat(undefined, { hour: 'numeric', minute: '2-digit', second: '2-digit' }).format(timestamp)
}
