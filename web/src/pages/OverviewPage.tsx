import { readinessLabel, type ConnectionSummary, type DeploymentPlan, type DeploymentSummary, type DispatchRun } from '../types'
import { ProviderLogo } from '../components/ProviderLogo'

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
const displayProvider = (provider: string) => provider.trim().toLowerCase() === 'box' ? 'Box' : provider.trim().toLowerCase() === 'salesforce' ? 'Salesforce' : provider

const dateFormatter = new Intl.DateTimeFormat(undefined, { month: 'short', day: 'numeric', year: 'numeric', hour: 'numeric', minute: '2-digit' })

const formatDate = (value?: string) => {
  if (!value) return 'Not recorded'
  const date = new Date(value)
  if (Number.isNaN(date.valueOf())) return value
  return dateFormatter.format(date)
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
  const readyCount = plan.components.filter((component) => connectionFor(component.name, connections)?.verified).length
  return <box-card className="overview-connection-health"><section><header><div><p className="overview-eyebrow">Local provider status</p><h2>Connections</h2></div><box-badge label={`${readyCount} ready`} tone="success"></box-badge></header><ul>{plan.components.map((component) => {
    const connection = connectionFor(component.name, connections)
    const ready = Boolean(connection?.verified)
    const onConnection = component.id === 'box' ? onBoxConnection : component.id === 'salesforce' ? onSalesforceConnection : undefined
    return <li key={component.id}><ProviderLogo provider={component.id} size="standard"/><div><strong>{component.name}</strong><small>{connection?.selection || connection?.authType || readinessLabel(ready)}</small></div><div className="health-actions"><span className={`health-state ${ready ? 'healthy' : 'attention'}`}>{readinessLabel(ready)}</span>{onConnection ? <box-button label={ready ? 'Manage' : 'Connect'} tone={ready ? 'neutral' : 'primary'} onClick={onConnection}></box-button> : null}</div></li>
  })}</ul></section></box-card>
}

function CurrentDeployment({ plan, connections, run, onContinue }: Pick<OverviewPageProps, 'plan' | 'connections' | 'run' | 'onContinue'>) {
  const running = run?.status === 'queued' || run?.status === 'running'
  const action = run?.action === 'deploy' ? 'Deployment' : 'Validation'
  const status = running ? `${action} in progress` : plan.exists ? 'Saved deployment' : 'No deployment selected'
  const configured = plan.components.filter((component) => component.ready).length
  const verified = plan.components.filter((component) => connectionFor(component.name, connections)?.verified).length
  const allReady = plan.components.length > 0 && verified === plan.components.length
  return <box-card className="overview-current"><section><header><div><p className="overview-eyebrow">{status}</p><h2>{plan.exists ? plan.name || plan.template || 'Deployment plan' : 'Choose a solution'}</h2></div>{running ? <box-badge label="Live" tone="info"></box-badge> : null}</header><p className="overview-summary">{plan.exists ? `${displayStrategy(plan.strategy)} · ${configured} of ${plan.components.length} selected system${plan.components.length === 1 ? '' : 's'} ready` : 'Choose a supported solution and the systems it should configure.'}</p>{plan.components.length > 0 ? <box-progress-bar label="Connection readiness" value={verified} max={plan.components.length}></box-progress-bar> : null}{plan.exists ? <box-button label={running ? `View ${action.toLowerCase()}` : allReady ? 'Continue deployment' : 'Connect systems'} tone="primary" onClick={onContinue}></box-button> : null}</section></box-card>
}

function DeploymentHistory({ deployments }: Pick<OverviewPageProps, 'deployments'>) {
  const recent = deployments.slice(0, 5)
  return <box-card className="overview-history"><section><header><div><p className="overview-eyebrow">Audit records</p><h2>Recent deployments</h2></div><span>{deployments.length} recorded</span></header><div className="overview-history-table"><table><caption className="visually-hidden">Recent deployments</caption><thead><tr><th scope="col">Deployment</th><th scope="col">Systems</th><th scope="col">Strategy</th><th scope="col">Completed</th></tr></thead><tbody>{recent.length ? recent.map((deployment) => <tr key={deployment.id}><th scope="row">{deployment.name || deployment.id}</th><td>{deployment.providers.length ? deployment.providers.map((provider) => displayProvider(provider.name)).join(', ') : 'Not recorded'}</td><td>{displayStrategy(deployment.strategy)}</td><td>{formatDate(deployment.completedAt)}</td></tr>) : <tr><td colSpan={4} className="history-empty">Completed deployments will appear here.</td></tr>}</tbody></table></div></section></box-card>
}

export function OverviewPage({ plan, connections, deployments, run, onNewDeployment, onContinue, onBoxConnection, onSalesforceConnection }: OverviewPageProps) {
  const selectedSystems = plan.components.length
  const verifiedSystems = plan.components.filter((component) => connectionFor(component.name, connections)?.verified).length
  const completedThisWeek = deployments.filter((deployment) => isCurrentWeek(deployment.completedAt)).length
  const latest = deployments[0]
  const allReady = plan.exists && selectedSystems > 0 && verifiedSystems === selectedSystems
  const headingCopy = plan.exists ? allReady ? 'Ready to validate and deploy.' : 'Connect remaining systems before validation.' : 'Choose a solution to start a deployment.'
  return <section className="overview-page" aria-label="Overview">
    <header className="overview-heading"><div><h1>Overview</h1><p>{headingCopy}</p></div><box-button label="New deployment" tone="primary" onClick={onNewDeployment}></box-button></header>
    <section className="overview-metrics" aria-label="Deployment summary">
      <Metric label="Saved plan" value={plan.exists ? 'Active' : 'Not started'} detail={plan.exists ? displayStrategy(plan.strategy) : 'Choose a solution'}/>
      <Metric label="Connections" value={`${verifiedSystems} of ${selectedSystems}`} detail={verifiedSystems === selectedSystems && selectedSystems > 0 ? 'Ready' : 'Need attention'} tone={verifiedSystems === selectedSystems && selectedSystems > 0 ? 'success' : 'neutral'}/>
      <Metric label="Completed this week" value={String(completedThisWeek)} detail="Deployment records"/>
      <Metric label="Latest deployment" value={latest ? 'Complete' : 'No runs'} detail={latest ? formatDate(latest.completedAt) : 'Run a deployment to see history'}/>
    </section>
    <section className="overview-dashboard">
      <div className="overview-primary"><CurrentDeployment plan={plan} connections={connections} run={run} onContinue={onContinue}/><DeploymentHistory deployments={deployments}/></div>
      <aside className="overview-secondary"><ConnectionHealth plan={plan} connections={connections} onBoxConnection={onBoxConnection} onSalesforceConnection={onSalesforceConnection}/></aside>
    </section>
  </section>
}

function Metric({ label, value, detail, tone = 'neutral' }: { label: string; value: string; detail: string; tone?: 'neutral' | 'success' }) {
  return <div className={`overview-metric ${tone}`}><span>{label}</span><strong>{value}</strong><small>{detail}</small></div>
}
