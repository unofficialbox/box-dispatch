import type { DeploymentPlan, DispatchRun, RunEvent } from '../types'
import { DetailList, DetailsRail } from '../components/DetailsRail'
import { RunTimeline } from '../components/RunTimeline'
import { presentProviderProgress } from '../components/runTimelineModel'

export function DeployPage({ plan, run, events, notice, onApply, onDiagnostics, onViewChanges }: { plan: DeploymentPlan; run: DispatchRun | null; events: RunEvent[]; notice: string; onApply: () => void; onDiagnostics: (runID: string) => void; onViewChanges: (runID: string) => void }) {
  const title = run?.action === 'deploy' ? 'Deployment' : 'Validation'
  const state = run?.status ?? 'queued'
  const status = state === 'completed' ? 'Complete' : state === 'failed' ? 'Needs attention' : 'In progress'
  const providers = presentProviderProgress(plan.components, run, events)
  const completedProviders = providers.filter((provider) => provider.state === 'complete').length
  const currentProvider = providers.find((provider) => provider.state === 'active' || provider.state === 'failed')
  const latestEvent = events.at(-1)
  const authenticationFailed = events.some((event) => event.component === 'Authentication' && event.progressState === 'failed')
  const statusCopy = status === 'In progress' ? 'Provider progress updates automatically.' : status === 'Complete' ? 'All selected systems finished successfully.' : authenticationFailed ? 'Authentication failed before configuration validation began.' : 'The run stopped before every selected system completed.'
  const detailCopy = state === 'failed' && authenticationFailed ? `${currentProvider?.name ?? 'A provider'} authentication failed. Reconnect it before retrying.` : latestEvent?.message || 'Waiting for provider activity.'

  return <section className="deploy-workspace" aria-label="Live deployment progress">
    <section className="validation-surface">
      <header className="validation-heading run-heading"><div><h2>{title}</h2><p>{statusCopy}</p></div><box-badge label={status} tone={state === 'completed' ? 'success' : state === 'failed' ? 'error' : 'info'}></box-badge></header>
      <div className="run-overall-progress"><box-progress-bar label={`${completedProviders} of ${providers.length} systems complete`} value={completedProviders} max={providers.length}></box-progress-bar></div>
      <RunTimeline providers={providers}/>
      {notice && <p className="notice" role="status">{notice}</p>}
      <footer className="action-row"><div className="stage-navigation">{run?.action === 'validate' && run.status === 'completed' && <box-button label="Review changes" tone="neutral" onClick={() => onViewChanges(run.id)}></box-button>}{run?.action === 'validate' && run.status === 'completed' && <box-button label="Continue to deployment" tone="primary" onClick={onApply}></box-button>}{run?.status === 'failed' && <box-button label="View diagnostic guidance" tone="primary" onClick={() => onDiagnostics(run.id)}></box-button>}</div></footer>
    </section>
    <DetailsRail title="Run details" description={detailCopy}><DetailList rows={[["Deployment", plan.name], ["Status", status], ['Run ID', run?.id || 'Preparing'], ['Current system', currentProvider?.name || (state === 'completed' ? 'All systems complete' : 'Waiting')], ['Completed', `${completedProviders} of ${providers.length}`], ['Strategy', plan.strategy === 'reuse' ? 'Reuse existing' : 'Create new']]}/></DetailsRail>
  </section>
}
