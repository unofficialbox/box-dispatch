import type { ConnectionSummary, DeploymentPlan, DeploymentSummary, DispatchRun } from '../types'

type OverviewPageProps = {
  plan: DeploymentPlan
  connections: ConnectionSummary[]
  deployments: DeploymentSummary[]
  run: DispatchRun | null
  onNewDeployment: () => void
  onContinue: () => void
}

const displayStrategy = (strategy: string) => strategy === 'create_new' ? 'Create new' : 'Reuse existing'

const readableProvider = (name: string) => name || 'Provider'

const formatDate = (value?: string) => {
  if (!value) return 'Not recorded'
  const date = new Date(value)
  if (Number.isNaN(date.valueOf())) return value
  return new Intl.DateTimeFormat(undefined, { month: 'short', day: 'numeric', year: 'numeric', hour: 'numeric', minute: '2-digit' }).format(date)
}

const isCurrentWeek = (value: string) => {
  const date = new Date(value)
  if (Number.isNaN(date.valueOf())) return false
  const start = new Date()
  start.setHours(0, 0, 0, 0)
  start.setDate(start.getDate() - ((start.getDay() + 6) % 7))
  return date >= start
}

const connectionFor = (name: string, connections: ConnectionSummary[]) => connections.find((connection) => connection.name.toLowerCase() === name.toLowerCase())

function MetricCard({ label, value, detail }: { label: string; value: string; detail: string }) {
  return <box-card className="overview-metric"><section><h2>{label}</h2><strong>{value}</strong><p>{detail}</p></section></box-card>
}

function ConnectionHealth({ plan, connections }: Pick<OverviewPageProps, 'plan' | 'connections'>) {
  return <box-card className="overview-connection-health"><section><header><div><p className="overview-eyebrow">Local provider status</p><h2>Connection health</h2></div><box-badge label={`${plan.components.filter((component) => connectionFor(component.name, connections)?.verified).length} verified`} tone="success"></box-badge></header><ul>{plan.components.map((component) => {
    const connection = connectionFor(component.name, connections)
    const ready = Boolean(connection?.verified)
    return <li key={component.id}><span className={`provider-mark ${component.id}`} aria-hidden="true">{component.id === 'box' ? 'B' : component.id === 'salesforce' ? 'SF' : component.name.slice(0, 2).toUpperCase()}</span><div><strong>{component.name}</strong><small>{connection?.selection || connection?.authType || (ready ? 'Local connection verified' : 'Connection needs verification')}</small></div><span className={`health-state ${ready ? 'healthy' : 'attention'}`}>{ready ? 'Healthy' : 'Needs attention'}</span></li>
  })}</ul></section></box-card>
}

function CurrentDeployment({ plan, connections, run, onContinue }: Pick<OverviewPageProps, 'plan' | 'connections' | 'run' | 'onContinue'>) {
  const running = run?.status === 'queued' || run?.status === 'running'
  const action = run?.action === 'deploy' ? 'Deployment' : 'Validation'
  const status = running ? `${action} in progress` : plan.exists ? 'Saved deployment' : 'No deployment selected'
  const configured = plan.components.filter((component) => component.ready).length
  const verified = plan.components.filter((component) => connectionFor(component.name, connections)?.verified).length
  return <box-card className="overview-current"><section><header><div><p className="overview-eyebrow">{status}</p><h2>{plan.template || 'Choose a solution'}</h2></div>{running ? <box-badge label="Live" tone="info"></box-badge> : null}</header><p className="overview-summary">{plan.exists ? `${displayStrategy(plan.strategy)} · ${configured} of ${plan.components.length} selected system${plan.components.length === 1 ? '' : 's'} ready` : 'Start with a supported solution, then select the systems it needs.'}</p><div className="overview-provider-track" aria-label="Selected providers">{plan.components.map((component) => <span key={component.id}><i className={`provider-dot ${component.ready ? 'ready' : ''}`} aria-hidden="true"></i>{component.name}</span>)}</div>{plan.components.length > 0 ? <box-progress-bar label="Connection readiness" value={verified} max={plan.components.length}></box-progress-bar> : null}{plan.exists && !running ? <box-button label="Continue deployment" tone="primary" onClick={onContinue}></box-button> : null}</section></box-card>
}

function DeploymentHistory({ deployments }: Pick<OverviewPageProps, 'deployments'>) {
  const recent = deployments.slice(0, 5)
  return <box-card className="overview-history"><section><header><div><p className="overview-eyebrow">Audit records</p><h2>Recent deployments</h2></div><span>{deployments.length} recorded</span></header>{recent.length === 0 ? <p className="overview-empty">No completed deployment records are available yet. Completed runs will appear here.</p> : <div className="overview-history-table" role="table" aria-label="Recent deployments"><div className="overview-history-head" role="row"><span role="columnheader">Deployment</span><span role="columnheader">Systems</span><span role="columnheader">Strategy</span><span role="columnheader">Completed</span></div>{recent.map((deployment) => <div className="overview-history-row" key={deployment.id} role="row"><strong role="cell">{deployment.name || deployment.id}</strong><span role="cell">{deployment.providers.map((provider) => readableProvider(provider.name)).join(' + ') || 'Not recorded'}</span><span role="cell">{displayStrategy(deployment.strategy)}</span><time role="cell" dateTime={deployment.completedAt}>{formatDate(deployment.completedAt)}</time></div>)}</div>}</section></box-card>
}

export function OverviewPage({ plan, connections, deployments, run, onNewDeployment, onContinue }: OverviewPageProps) {
  const selectedSystems = plan.components.length
  const verifiedSystems = plan.components.filter((component) => connectionFor(component.name, connections)?.verified).length
  const completedThisWeek = deployments.filter((deployment) => isCurrentWeek(deployment.completedAt)).length
  const latest = deployments[0]
  const allReady = selectedSystems > 0 && verifiedSystems === selectedSystems
  return <section className="overview-page" aria-label="Overview">
    <header className="overview-heading"><div><p className="overview-eyebrow">Dispatch workspace</p><h1>Overview</h1><p>{allReady ? 'All selected systems are ready. Review the saved plan or start a new deployment.' : 'Review current work, provider readiness, and recent local deployment records.'}</p></div><box-button label="New deployment" tone="primary" onClick={onNewDeployment}></box-button></header>
    <section className="overview-metrics" aria-label="Deployment summary">
      <MetricCard label="Saved plan" value={plan.exists ? plan.template : 'None'} detail={plan.exists ? `${displayStrategy(plan.strategy)} strategy` : 'Choose a solution to begin'} />
      <MetricCard label="Connection readiness" value={`${verifiedSystems} of ${selectedSystems} verified`} detail={allReady ? 'Every selected system is ready' : 'Verify the remaining selected systems'} />
      <MetricCard label="Completed this week" value={String(completedThisWeek)} detail="From local deployment audit records" />
      <MetricCard label="Latest deployment" value={latest ? latest.name || latest.id : 'None'} detail={latest ? formatDate(latest.completedAt) : 'No completed run recorded'} />
    </section>
    <section className="overview-dashboard">
      <div className="overview-primary"><CurrentDeployment plan={plan} connections={connections} run={run} onContinue={onContinue}/><DeploymentHistory deployments={deployments}/></div>
      <aside className="overview-secondary"><ConnectionHealth plan={plan} connections={connections}/><box-card className="overview-next"><section><p className="overview-eyebrow">Recommended next action</p><h2>{allReady ? 'Review the deployment plan' : 'Verify selected connections'}</h2><p>{allReady ? 'Check the selected components and deployment strategy before validation.' : 'Open the saved deployment and reconnect or verify the selected provider.'}</p><box-button label={allReady ? 'Continue deployment' : 'Open deployment'} tone="secondary" onClick={onContinue}></box-button></section></box-card></aside>
    </section>
  </section>
}
