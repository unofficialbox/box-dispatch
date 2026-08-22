import type { DeploymentPlan, DispatchRun, RunEvent } from '../types'
import { DetailList, DetailsRail } from '../components/DetailsRail'
import { RunTimeline } from '../components/RunTimeline'
import { presentProviderProgress } from '../components/runTimelineModel'

export function DeployPage({ plan, run, events, notice, onApply, onDiagnostics }: { plan: DeploymentPlan; run: DispatchRun | null; events: RunEvent[]; notice: string; onApply: () => void; onDiagnostics: (runID: string) => void }) {
  const title = run?.action === 'deploy' ? 'Deployment' : 'Validation'
  const state = run?.status ?? 'queued'
  const status = state === 'completed' ? 'Complete' : state === 'failed' ? 'Needs attention' : 'In progress'
  const providers = presentProviderProgress(plan.components, run, events)
  const completedProviders = providers.filter((provider) => provider.state === 'complete').length
  const currentProvider = providers.find((provider) => provider.state === 'active' || provider.state === 'failed')
  const latestEvent = events.at(-1)
  return <section className="deploy-workspace" aria-label="Live deployment progress"><section className="validation-surface"><header className="validation-heading run-heading"><div><h2>{title}</h2><p>{status === 'In progress' ? 'Dispatch is following the local provider activity as it happens.' : status === 'Complete' ? 'All selected systems finished successfully.' : 'The run stopped before every selected system completed.'}</p></div><box-badge label={status} tone={state === 'completed' ? 'success' : state === 'failed' ? 'danger' : 'info'}></box-badge></header><div className="run-overall-progress"><box-progress-bar label={`${completedProviders} of ${providers.length} systems complete`} value={completedProviders} max={providers.length}></box-progress-bar><span>{events.length} live update{events.length === 1 ? '' : 's'}</span></div><RunTimeline providers={providers}/>{notice && <p className="notice" role="status">{notice}</p>}<footer className="action-row">{run?.action === 'validate' && run.status === 'completed' && <box-button label="Apply validated changes" tone="primary" onClick={onApply}></box-button>}{run?.status === 'failed' && <box-button label="View diagnostic guidance" tone="secondary" onClick={() => onDiagnostics(run.id)}></box-button>}</footer></section><DetailsRail title="Run details" description={latestEvent?.message || 'Waiting for provider activity.'}><DetailList rows={[["Status", status], ['Run ID', run?.id || 'Preparing'], ['Current system', currentProvider?.name || (state === 'completed' ? 'All systems complete' : 'Waiting')], ['Completed', `${completedProviders} of ${providers.length}`], ['Strategy', plan.strategy === 'reuse' ? 'Reuse existing' : 'Create new']]}/></DetailsRail></section>
}
