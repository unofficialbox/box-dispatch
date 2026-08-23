import type { ConnectionSummary, DeploymentPlan, DeploymentSummary, DispatchRun } from '../types'
import type { TableColumn, TableRow } from '@unofficialbox/box-open-elements/table'

type OverviewPageProps = {
  plan: DeploymentPlan
  connections: ConnectionSummary[]
  deployments: DeploymentSummary[]
  run: DispatchRun | null
  onNewDeployment: () => void
  onContinue: () => void
  onBoxConnection: () => void
  onSalesforceConnection: () => void
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

function ConnectionHealth({ plan, connections, onBoxConnection, onSalesforceConnection }: Pick<OverviewPageProps, 'plan' | 'connections' | 'onBoxConnection' | 'onSalesforceConnection'>) {
  return <box-card className="overview-connection-health"><section><header><div><p className="overview-eyebrow">Local provider status</p><h2>Connection health</h2></div><box-badge label={`${plan.components.filter((component) => connectionFor(component.name, connections)?.verified).length} verified`} tone="success"></box-badge></header><ul>{plan.components.map((component) => {
    const connection = connectionFor(component.name, connections)
    const ready = Boolean(connection?.verified)
    const onConnection = component.id === 'box' ? onBoxConnection : component.id === 'salesforce' ? onSalesforceConnection : undefined
    return <li key={component.id}><span className={`provider-mark ${component.id}`} aria-hidden="true">{component.id === 'box' ? 'B' : component.id === 'salesforce' ? 'SF' : component.name.slice(0, 2).toUpperCase()}</span><div><strong>{component.name}</strong><small>{connection?.selection || connection?.authType || (ready ? 'Local connection verified' : 'Connection needs verification')}</small></div><div className="health-actions"><span className={`health-state ${ready ? 'healthy' : 'attention'}`}>{ready ? 'Healthy' : 'Needs attention'}</span>{onConnection ? <box-button label={ready ? 'Manage' : 'Connect'} tone={ready ? 'secondary' : 'primary'} onClick={onConnection}></box-button> : null}</div></li>
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
  const columns: TableColumn[] = [
    { key: 'deployment', label: 'Deployment' },
    { key: 'systems', label: 'Systems' },
    { key: 'strategy', label: 'Strategy' },
    { key: 'completed', label: 'Completed' },
  ]
  const rows: TableRow[] = recent.map((deployment) => ({
    id: deployment.id,
    cells: {
      deployment: { kind: 'text', text: deployment.name || deployment.id, tone: 'brand' },
      systems: deployment.providers.map((provider) => readableProvider(provider.name)).join(' + ') || 'Not recorded',
      strategy: displayStrategy(deployment.strategy),
      completed: formatDate(deployment.completedAt),
    },
  }))
  return <box-card className="overview-history"><section><header><div><p className="overview-eyebrow">Audit records</p><h2>Recent deployments</h2></div><span>{deployments.length} recorded</span></header><box-table className="overview-history-table" label="Recent deployments" columns={columns} rows={rows} selectionMode="none" emptyText="No completed deployment records are available yet. Completed runs will appear here."></box-table></section></box-card>
}

export function OverviewPage({ plan, connections, deployments, run, onNewDeployment, onContinue, onBoxConnection, onSalesforceConnection }: OverviewPageProps) {
  const selectedSystems = plan.components.length
  const verifiedSystems = plan.components.filter((component) => connectionFor(component.name, connections)?.verified).length
  const completedThisWeek = deployments.filter((deployment) => isCurrentWeek(deployment.completedAt)).length
  const latest = deployments[0]
  const allReady = selectedSystems > 0 && verifiedSystems === selectedSystems
  return <section className="overview-page" aria-label="Overview">
    <header className="overview-heading"><div><p className="overview-eyebrow">Dispatch workspace</p><h1>Overview</h1><p>{allReady ? 'All selected systems are ready. Review the saved plan or start a new deployment.' : 'Review current work, provider readiness, and recent local deployment records.'}</p></div><box-button label="New deployment" tone="primary" onClick={onNewDeployment}></box-button></header>
    <section className="overview-metrics" aria-label="Deployment summary">
      <box-metric-card className="overview-metric" heading="Saved plan" value={plan.exists ? plan.template : 'None'} message={plan.exists ? `${displayStrategy(plan.strategy)} strategy` : 'Choose a solution to begin'}></box-metric-card>
      <box-metric-card className="overview-metric" heading="Connection readiness" value={`${verifiedSystems} of ${selectedSystems} verified`} message={allReady ? 'Every selected system is ready' : 'Connect or verify the remaining systems'} status={allReady ? 'Ready' : 'Action needed'}></box-metric-card>
      <box-metric-card className="overview-metric" heading="Completed this week" value={String(completedThisWeek)} message="From local deployment audit records"></box-metric-card>
      <box-metric-card className="overview-metric" heading="Latest deployment" value={latest ? latest.name || latest.id : 'None'} message={latest ? formatDate(latest.completedAt) : 'No completed run recorded'}></box-metric-card>
    </section>
    <section className="overview-dashboard">
      <div className="overview-primary"><CurrentDeployment plan={plan} connections={connections} run={run} onContinue={onContinue}/><DeploymentHistory deployments={deployments}/></div>
      <aside className="overview-secondary"><ConnectionHealth plan={plan} connections={connections} onBoxConnection={onBoxConnection} onSalesforceConnection={onSalesforceConnection}/></aside>
    </section>
  </section>
}
