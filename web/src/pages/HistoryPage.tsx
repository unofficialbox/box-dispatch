import { useEffect, useState } from 'react'
import { Alert } from '@unofficialbox/box-open-elements-react/alert'
import { Select } from '@unofficialbox/box-open-elements-react/select'
import '@unofficialbox/box-open-elements/card'
import '@unofficialbox/box-open-elements/fact-list'
import '@unofficialbox/box-open-elements/link-button'
import '@unofficialbox/box-open-elements/metric-card'
import '@unofficialbox/box-open-elements/section'
import type { TableColumn, TableRow } from '@unofficialbox/box-open-elements/table'
import { BoeTable } from '../components/BoeTable'
import { DeploymentHistoryTable } from '../components/DeploymentHistoryTable'
import { DetailList, DetailsRail } from '../components/DetailsRail'
import { SearchFieldControl } from '../components/SearchFieldControl'
import { deploymentOutcome, displayProvider, displayStrategy, formatDeploymentDate } from '../deploymentPresentation'
import type { DeploymentDetail, DeploymentSummary } from '../types'

type HistoryPageProps = {
  deployments: DeploymentSummary[]
  selectedDeploymentID?: string | null
  onCloseDeployment?: () => void
  onOpenDestination?: (launchUrl: string, label: string) => void
  onViewChanges?: (deploymentID: string) => void
}

type ResultFilter = 'all' | 'complete' | 'attention' | 'recorded'

const matchesResult = (deployment: DeploymentSummary, filter: ResultFilter) => {
  if (filter === 'all') return true
  const label = deploymentOutcome(deployment).label
  if (filter === 'complete') return label === 'Complete'
  if (filter === 'attention') return label === 'Needs attention'
  return label === 'Recorded'
}

function HistoricalDeploymentDetail({ detail, onBack, onOpenDestination, onViewChanges }: { detail: DeploymentDetail; onBack: () => void; onOpenDestination: (launchUrl: string, label: string) => void; onViewChanges: (deploymentID: string) => void }) {
  const outcome = deploymentOutcome(detail)
  const systems = detail.providers.length ? detail.providers.map((provider) => displayProvider(provider.name)).join(', ') : 'Not recorded'
  const deployedComponents = detail.providers.flatMap((provider) => provider.deployedComponents.map((component) => ({ provider: provider.name, component })))
  const componentColumns: TableColumn[] = [{ key: 'system', label: 'System' }, { key: 'component', label: 'Component' }, { key: 'result', label: 'Result' }]
  const componentRows: TableRow[] = deployedComponents.map(({ provider, component }) => ({ id: `${provider}-${component}`, cells: { system: displayProvider(provider), component, result: { kind: 'badge', text: 'Deployed', tone: 'success' } } }))
  return <section className="record-page history-detail-page" aria-labelledby="history-detail-title">
    <box-link-button className="history-back-link" href="#history" label="Back to deployment history" onClick={onBack}></box-link-button>
    <header className="record-page-heading history-detail-heading"><div><p className="overview-eyebrow">Historical deployment</p><h1 id="history-detail-title">{detail.name || detail.id}</h1><p>Read-only results captured when this deployment finished.</p></div><box-badge label={outcome.label} tone={outcome.tone}></box-badge></header>
    <div className="history-detail-layout">
      <box-section className="history-provider-summary" heading="Provider summary" description="Recorded configuration results for each deployed system.">
        <div slot="actions" className="history-environment-actions" aria-label="Deployment actions">{detail.changesRecorded ? <box-button label="Review changes" tone="neutral" onClick={() => onViewChanges(detail.id)}></box-button> : <span className="history-change-unavailable">Change preview not recorded</span>}</div>
        <div className="history-provider-cards">{detail.providers.map((provider) => {
          const providerOutcome = deploymentOutcome({ ...detail, providers: [provider] })
          const providerName = displayProvider(provider.name)
          const openLabel = `Open ${providerName}`
          const environmentIDLabel = provider.name.toLowerCase() === 'box' ? 'Enterprise ID' : 'Org ID'
          return <box-card className="history-provider-card" key={provider.name}><section><header><div className="history-provider-identity"><strong>{providerName}</strong>{provider.environmentDomain ? <span>{provider.environmentDomain}</span> : null}{provider.environmentId ? <small>{environmentIDLabel} {provider.environmentId}</small> : null}</div><div className="history-provider-heading-actions">{provider.launchUrl ? <box-button label={openLabel} tone="neutral" onClick={() => onOpenDestination(provider.launchUrl!, openLabel)}></box-button> : null}<box-badge label={providerOutcome.label} tone={providerOutcome.tone}></box-badge></div></header><box-fact-list rows={[{ label: 'Deployed', value: String(provider.deployedCount) }, { label: 'Present', value: String(provider.presentCount) }, { label: 'Remaining', value: String(provider.remainingCount) }, { label: 'Manual', value: String(provider.manualItemCount) }]}></box-fact-list></section></box-card>
        })}</div>
        <box-section className="history-components" heading="Deployment details" description="Individual components added or updated by this deployment."><span slot="actions">{deployedComponents.length} deployed</span><BoeTable className="deployment-component-table" columns={componentColumns} rows={componentRows} label="Components deployed by this deployment" emptyText="No component changes were recorded for this deployment."/></box-section>
      </box-section>
      <DetailsRail title="Deployment summary" description="The immutable audit record for this run."><DetailList rows={[["Deployment ID", detail.id], ["Run ID", detail.runId || 'Not recorded'], ["Systems", systems], ["Strategy", displayStrategy(detail.strategy)], ["Started", formatDeploymentDate(detail.startedAt)], ["Completed", formatDeploymentDate(detail.completedAt)], ["Duration", detail.duration || 'Not recorded']]}/></DetailsRail>
    </div>
  </section>
}

export function HistoryPage({ deployments, selectedDeploymentID = null, onCloseDeployment = () => undefined, onOpenDestination = () => undefined, onViewChanges = () => undefined }: HistoryPageProps) {
  const [query, setQuery] = useState('')
  const [providerFilter, setProviderFilter] = useState('all')
  const [resultFilter, setResultFilter] = useState<ResultFilter>('all')
  const [strategyFilter, setStrategyFilter] = useState('all')
  const [detail, setDetail] = useState<DeploymentDetail | null>(null)
  const [detailError, setDetailError] = useState<{ deploymentID: string; message: string } | null>(null)
  const complete = deployments.filter((deployment) => deploymentOutcome(deployment).label === 'Complete').length
  const attention = deployments.filter((deployment) => deploymentOutcome(deployment).label === 'Needs attention').length
  const providerOptions = [...new Set(deployments.flatMap((deployment) => deployment.providers.map((provider) => provider.name.toLowerCase())))].sort()
  const normalizedQuery = query.trim().toLowerCase()
  const filteredDeployments = deployments.filter((deployment) => {
    const matchesQuery = !normalizedQuery || [deployment.name, deployment.id, ...deployment.providers.map((provider) => displayProvider(provider.name))].some((value) => value.toLowerCase().includes(normalizedQuery))
    const matchesProvider = providerFilter === 'all' || deployment.providers.some((provider) => provider.name.toLowerCase() === providerFilter)
    const matchesStrategy = strategyFilter === 'all' || (strategyFilter === 'create_new' ? deployment.strategy === 'create_new' : deployment.strategy !== 'create_new')
    return matchesQuery && matchesProvider && matchesResult(deployment, resultFilter) && matchesStrategy
  })
  const filtersActive = Boolean(normalizedQuery || providerFilter !== 'all' || resultFilter !== 'all' || strategyFilter !== 'all')

  useEffect(() => {
    if (!selectedDeploymentID) return
    const controller = new AbortController()
    void fetch(`/api/deployments/${encodeURIComponent(selectedDeploymentID)}`, { signal: controller.signal }).then(async (response) => {
      if (!response.ok) throw new Error(response.status === 404 ? 'This deployment record no longer exists.' : 'The deployment summary is unavailable.')
      return await response.json() as DeploymentDetail
    }).then((nextDetail) => {
      setDetail(nextDetail)
      setDetailError(null)
    }).catch((error: unknown) => {
      if (!controller.signal.aborted) setDetailError({ deploymentID: selectedDeploymentID, message: error instanceof Error ? error.message : 'The deployment summary is unavailable.' })
    })
    return () => controller.abort()
  }, [selectedDeploymentID])

  if (selectedDeploymentID) {
    if (detail?.id === selectedDeploymentID) return <HistoricalDeploymentDetail detail={detail} onBack={onCloseDeployment} onOpenDestination={onOpenDestination} onViewChanges={onViewChanges}/>
    const selectedError = detailError?.deploymentID === selectedDeploymentID ? detailError.message : ''
    return <section className="record-page history-detail-page" aria-labelledby="history-detail-state"><box-link-button className="history-back-link" href="#history" label="Back to deployment history" onClick={onCloseDeployment}></box-link-button><div className="history-detail-state"><h1 id="history-detail-state">{selectedError ? 'Deployment summary unavailable' : 'Loading deployment summary'}</h1>{selectedError ? <Alert heading="Deployment summary unavailable" message={selectedError} tone="error" open/> : <><box-spinner label="Loading deployment summary" size="large"></box-spinner><p role="status">Reading the historical audit record…</p></>}</div></section>
  }

  const clearFilters = () => {
    setQuery('')
    setProviderFilter('all')
    setResultFilter('all')
    setStrategyFilter('all')
  }
  return <section className="record-page history-page" aria-labelledby="history-title">
    <header className="record-page-heading"><div><p className="overview-eyebrow">Audit records</p><h1 id="history-title">Deployment history</h1><p>Review every recorded deployment and its provider outcome.</p></div></header>
    <section className="record-page-metrics" aria-label="History summary">
      <box-metric-card heading="Total deployments" value={String(deployments.length)} message="Recorded audit entries"></box-metric-card>
      <box-metric-card heading="Complete" value={String(complete)} message="Finished successfully"></box-metric-card>
      <box-metric-card heading="Needs attention" value={String(attention)} message="Review provider results"></box-metric-card>
    </section>
    <section className="record-page-section" aria-labelledby="all-deployments-title"><header><div><h2 id="all-deployments-title">All deployments</h2><p>Filter the audit trail, then open any deployment for its recorded summary.</p></div><span>{filteredDeployments.length === deployments.length ? `${deployments.length} recorded` : `Showing ${filteredDeployments.length} of ${deployments.length}`}</span></header>
      <fieldset className="history-filters"><legend className="visually-hidden">Filter deployment history</legend><SearchFieldControl label="Search" value={query} placeholder="Name or deployment ID" onChange={setQuery}/><Select label="System" value={providerFilter} options={[{ label: 'All systems', value: 'all' }, ...providerOptions.map((provider) => ({ label: displayProvider(provider), value: provider }))]} onValueChanged={(event) => setProviderFilter(event.detail.value)}/><Select label="Result" value={resultFilter} options={[{ label: 'All results', value: 'all' }, { label: 'Complete', value: 'complete' }, { label: 'Needs attention', value: 'attention' }, { label: 'Recorded', value: 'recorded' }]} onValueChanged={(event) => setResultFilter(event.detail.value as ResultFilter)}/><Select label="Strategy" value={strategyFilter} options={[{ label: 'All strategies', value: 'all' }, { label: 'Reuse existing', value: 'reuse' }, { label: 'Create new', value: 'create_new' }]} onValueChanged={(event) => setStrategyFilter(event.detail.value)}/><box-button className="history-clear-filters" label="Clear filters" tone="neutral" disabled={!filtersActive} onClick={clearFilters}></box-button></fieldset>
      <DeploymentHistoryTable deployments={filteredDeployments} caption="All deployments" includeResult emptyMessage={filtersActive ? 'No deployments match these filters.' : undefined}/>
    </section>
  </section>
}
