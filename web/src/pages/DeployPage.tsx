import type { DeploymentPlan, DispatchRun, RunEvent } from '../types'
import { DetailList, DetailsRail } from '../components/DetailsRail'
import { RunTimeline } from '../components/RunTimeline'
import { presentProviderProgress } from '../components/runTimelineModel'

export function DeployPage({ plan, run, events, notice, starting = false, retrying = false, onRetryValidation, onEditConnections, onApply, onDiagnostics, onViewChanges }: { plan: DeploymentPlan; run: DispatchRun | null; events: RunEvent[]; notice: string; starting?: boolean; retrying?: boolean; onRetryValidation?: () => void; onEditConnections?: () => void; onApply: () => void; onDiagnostics: (runID: string) => void; onViewChanges: (runID: string) => void }) {
  const title = run?.action === 'deploy' ? 'Deployment' : 'Validation'
  const state = run?.status ?? 'queued'
  const status = state === 'completed' ? 'Complete' : state === 'failed' ? 'Needs attention' : 'In progress'
  const providers = presentProviderProgress(plan.components, run, events)
  const allComponents = providers.flatMap((provider) => provider.components)
  const completedComponents = allComponents.filter((component) => component.state === 'completed').length
  const totalComponents = Math.max(allComponents.length, 1)
  const progressPercent = state === 'completed' ? 100 : Math.floor((completedComponents / totalComponents) * 100)
  const currentProvider = providers.find((provider) => provider.state === 'active' || provider.state === 'failed')
  const latestEvent = events.at(-1)
  const authenticationFailed = events.some((event) => event.component === 'Authentication' && event.progressState === 'failed')
  const statusCopy = status === 'In progress' ? 'Provider progress updates automatically.' : status === 'Complete' ? 'All selected systems finished successfully.' : authenticationFailed ? 'Authentication failed before configuration validation began.' : 'The run stopped before every selected system completed.'
  const detailCopy = state === 'failed' && authenticationFailed ? `${currentProvider?.name ?? 'A provider'} authentication failed. Reconnect it before retrying.` : latestEvent?.message || 'Waiting for provider activity.'

  return <section className="deploy-workspace" aria-label="Live deployment progress">
    <section className="validation-surface">
      <header className="validation-heading run-heading"><div><h2>{title}</h2><p>{statusCopy}</p></div><box-badge label={status} tone={state === 'completed' ? 'success' : state === 'failed' ? 'error' : 'info'}></box-badge></header>
      <div className="run-overall-progress"><box-progress-bar label={allComponents.length > 0 ? `${completedComponents} of ${totalComponents} steps complete (${progressPercent}%)` : 'Preparing…'} value={completedComponents} max={totalComponents}></box-progress-bar></div>
      <RunTimeline providers={providers}/>
      {notice && <p className="notice" role="status">{notice}</p>}
      {run?.status === 'failed' && <p className="notice">Review the failure and confirm the provider state. A retry starts fresh validation before any changes can be applied.</p>}
      <footer className="action-row"><div className="stage-navigation">{run?.action === 'validate' && run.status === 'completed' && <box-button label="Review changes" tone="neutral" onClick={() => onViewChanges(run.id)}></box-button>}{run?.action === 'validate' && run.status === 'completed' && <box-button label={starting ? 'Starting deployment…' : 'Continue to deployment'} disabled={starting} tone="primary" onClick={onApply}></box-button>}{run?.status === 'failed' && <><box-button label="View diagnostic guidance" tone="primary" onClick={() => onDiagnostics(run.id)}></box-button>{onEditConnections && <box-button label="Edit connections" tone="neutral" disabled={retrying} onClick={onEditConnections}></box-button>}{onRetryValidation && <box-button label={retrying ? 'Checking connections…' : 'Retry validation'} tone="neutral" disabled={retrying} onClick={onRetryValidation}></box-button>}</>}</div></footer>
    </section>
    <DetailsRail title="Run details" description={detailCopy}><DetailList rows={[["Deployment", plan.name], ["Status", status], ['Run ID', run?.id || 'Preparing'], ['Current system', currentProvider?.name || (state === 'completed' ? 'All systems complete' : 'Waiting')], ['Progress', `${completedComponents} of ${totalComponents} steps (${progressPercent}%)`], ['Strategy', plan.strategy === 'reuse' ? 'Reuse existing' : 'Create new']]}/></DetailsRail>
  </section>
}
