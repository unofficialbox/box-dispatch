import { useEffect, useRef } from 'react'
import type { ConnectionSummary, DeploymentPlan, DeploymentSummary, DispatchRun } from '../types'
import { DeploymentHistoryTable } from '../components/DeploymentHistoryTable'
import { EmptyProviderConnection, ProviderConnectionPanel, ProviderConnectionRow } from '../components/ProviderConnectionPanel'
import { displayStrategy, formatDeploymentDate } from '../deploymentPresentation'

type OverviewPageProps = {
  plan: DeploymentPlan
  connections: ConnectionSummary[]
  deployments: DeploymentSummary[]
  run: DispatchRun | null
  onNewDeployment: () => void
  onContinue: () => void
  onBoxConnection: () => void
  onSalesforceConnection: () => void
  onOpenProvider: (providerID: string) => void
  onViewHistory: () => void
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

function OverviewActionButton({ icon, label, disabled = false, onPress }: { icon: 'arrow-right' | 'gear'; label: string; disabled?: boolean; onPress: () => void }) {
  const ref = useRef<HTMLElement>(null)
  const onPressRef = useRef(onPress)
  useEffect(() => { onPressRef.current = onPress }, [onPress])
  useEffect(() => {
    const button = ref.current
    if (!button) return
    const handleClick = (event: Event) => {
      event.preventDefault()
      if (!disabled) onPressRef.current()
    }
    button.addEventListener('click', handleClick)
    return () => button.removeEventListener('click', handleClick)
  }, [disabled])
  return <box-icon-button ref={ref} icon={icon} label={label} disabled={disabled}></box-icon-button>
}

function ConnectionHealth({ plan, connections, onBoxConnection, onSalesforceConnection, onOpenProvider }: Pick<OverviewPageProps, 'plan' | 'connections' | 'onBoxConnection' | 'onSalesforceConnection' | 'onOpenProvider'>) {
  const readyCount = plan.components.filter((component) => connectionFor(component.name, connections)?.verified).length
  return <box-card className="overview-connection-health"><section><header><div><p className="overview-eyebrow">Selected environments</p><h2>Connections</h2><p className="overview-connection-copy">Active provider environments. Manage all saved connections in Settings.</p></div><box-badge label={`${readyCount} ready`} tone={readyCount === plan.components.length ? 'success' : 'info'}></box-badge></header><div className="overview-provider-panels">{plan.components.map((component) => {
    const connection = connectionFor(component.name, connections)
    const ready = Boolean(connection?.verified)
    const onConnection = component.id === 'box' ? onBoxConnection : component.id === 'salesforce' ? onSalesforceConnection : undefined
    const records = component.id === 'box' ? connection?.connections ?? [] : component.id === 'salesforce' ? connection?.orgs ?? [] : []
    const selected = records.find((record) => record.selected)
    const title = component.name
    const primary = selected && 'username' in selected ? selected.alias || selected.username || 'Salesforce org' : selected && 'identity' in selected ? selected.alias || selected.identity || 'Box connection' : connection?.selection || component.name
    const details = selected && 'orgId' in selected ? [[selected.kind, selected.orgId ? `Org ID ${selected.orgId}` : ''].filter(Boolean).join(' · ')] : selected && 'identity' in selected ? [selected.identity || selected.subjectType || 'Box account'] : [connection?.authType || 'No selected environment']
    const actions = <div className="overview-provider-actions"><OverviewActionButton icon="arrow-right" label={`Open ${title}`} disabled={!connection?.launchUrl} onPress={() => onOpenProvider(component.id)}/><OverviewActionButton icon="gear" label={`Configure ${title}`} disabled={!onConnection} onPress={onConnection ?? (() => undefined)}/></div>
    return <ProviderConnectionPanel key={component.id} provider={component.id} title={title} count={records.length || (connection?.configured ? 1 : 0)} compact actions={actions}>{connection?.configured ? <ProviderConnectionRow primary={primary} details={details} ready={ready}/> : <EmptyProviderConnection provider={component.name} compact/>}</ProviderConnectionPanel>
  })}</div></section></box-card>
}

function CurrentDeployment({ plan, connections, run, onContinue, onViewHistory }: Pick<OverviewPageProps, 'plan' | 'connections' | 'run' | 'onContinue' | 'onViewHistory'>) {
  const running = run?.status === 'queued' || run?.status === 'running'
  const deploymentComplete = run?.action === 'deploy' && run.status === 'completed'
  const action = run?.action === 'deploy' ? 'Deployment' : 'Validation'
  const status = running ? `${action} in progress` : plan.exists ? 'Saved deployment' : 'No deployment selected'
  const configured = plan.components.filter((component) => component.ready).length
  const verified = plan.components.filter((component) => connectionFor(component.name, connections)?.verified).length
  const allReady = plan.components.length > 0 && verified === plan.components.length
  if (deploymentComplete) return <box-card className="overview-current overview-current-complete"><section><header><div><p className="overview-eyebrow">Active deployment</p><h2>No deployment in progress</h2></div></header><p className="overview-summary">The latest deployment, <strong>{plan.name || plan.template || 'Untitled deployment'}</strong>, completed successfully and is available in history.</p><div className="overview-current-actions"><box-button label="View history" tone="neutral" onClick={onViewHistory}></box-button></div></section></box-card>
  return <box-card className="overview-current"><section><header><div><p className="overview-eyebrow">{status}</p><h2>{plan.exists ? plan.name || plan.template || 'Deployment plan' : 'Choose a solution'}</h2></div>{running ? <box-badge label="Live" tone="info"></box-badge> : null}</header><p className="overview-summary">{plan.exists ? `${displayStrategy(plan.strategy)} · ${configured} of ${plan.components.length} selected system${plan.components.length === 1 ? '' : 's'} ready` : 'Choose a supported solution and the systems it should configure.'}</p>{plan.components.length > 0 ? <box-progress-bar label="Connection readiness" value={verified} max={plan.components.length}></box-progress-bar> : null}{plan.exists ? <box-button label={running ? `View ${action.toLowerCase()}` : allReady ? 'Continue deployment' : 'Connect systems'} tone="primary" onClick={onContinue}></box-button> : null}</section></box-card>
}

function DeploymentHistory({ deployments }: Pick<OverviewPageProps, 'deployments'>) {
  const recent = deployments.slice(0, 5)
  return <box-card className="overview-history"><section><header><div><p className="overview-eyebrow">Audit records</p><h2>Recent deployments</h2></div><span>{deployments.length} recorded</span></header><div className="overview-history-table"><DeploymentHistoryTable deployments={recent} caption="Recent deployments"/></div></section></box-card>
}

export function OverviewPage({ plan, connections, deployments, run, onNewDeployment, onContinue, onBoxConnection, onSalesforceConnection, onOpenProvider, onViewHistory }: OverviewPageProps) {
  const selectedSystems = plan.components.length
  const verifiedSystems = plan.components.filter((component) => connectionFor(component.name, connections)?.verified).length
  const completedThisWeek = deployments.filter((deployment) => isCurrentWeek(deployment.completedAt)).length
  const latest = deployments[0]
  const allReady = plan.exists && selectedSystems > 0 && verifiedSystems === selectedSystems
  const deploymentComplete = run?.action === 'deploy' && run.status === 'completed'
  const headingCopy = deploymentComplete ? 'No deployment is currently in progress.' : plan.exists ? allReady ? 'Ready to validate and deploy.' : 'Connect remaining systems before validation.' : 'Choose a solution to start a deployment.'
  return <section className="overview-page" aria-label="Overview">
    <header className="overview-heading"><div><h1>Overview</h1><p>{headingCopy}</p></div><box-button label="New deployment" tone="primary" onClick={onNewDeployment}></box-button></header>
    <section className="overview-metrics" aria-label="Deployment summary">
      <Metric label="Active deployment" value={deploymentComplete ? 'None' : plan.exists ? 'Active' : 'Not started'} detail={deploymentComplete ? 'Start a new deployment' : plan.exists ? displayStrategy(plan.strategy) : 'Choose a solution'}/>
      <Metric label="Connections" value={`${verifiedSystems} of ${selectedSystems}`} detail={verifiedSystems === selectedSystems && selectedSystems > 0 ? 'Ready' : 'Need attention'} tone={verifiedSystems === selectedSystems && selectedSystems > 0 ? 'success' : 'neutral'}/>
      <Metric label="Completed this week" value={String(completedThisWeek)} detail="Deployment records"/>
      <Metric label="Latest deployment" value={latest ? 'Complete' : 'No runs'} detail={latest ? formatDeploymentDate(latest.completedAt) : 'Run a deployment to see history'}/>
    </section>
    <section className="overview-dashboard">
      <div className="overview-primary"><CurrentDeployment plan={plan} connections={connections} run={run} onContinue={onContinue} onViewHistory={onViewHistory}/><DeploymentHistory deployments={deployments}/></div>
      <aside className="overview-secondary"><ConnectionHealth plan={plan} connections={connections} onBoxConnection={onBoxConnection} onSalesforceConnection={onSalesforceConnection} onOpenProvider={onOpenProvider}/></aside>
    </section>
  </section>
}

function Metric({ label, value, detail, tone = 'neutral' }: { label: string; value: string; detail: string; tone?: 'neutral' | 'success' }) {
  return <div className={`overview-metric ${tone}`}><span>{label}</span><strong>{value}</strong><small>{detail}</small></div>
}
