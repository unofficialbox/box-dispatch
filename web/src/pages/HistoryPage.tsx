import { useEffect, useState } from 'react'
import { DeploymentHistoryTable } from '../components/DeploymentHistoryTable'
import { DetailList, DetailsRail } from '../components/DetailsRail'
import { deploymentOutcome, displayProvider, displayStrategy, formatDeploymentDate } from '../deploymentPresentation'
import type { DeploymentDetail, DeploymentSummary } from '../types'

type HistoryPageProps = {
  deployments: DeploymentSummary[]
  selectedDeploymentID?: string | null
  onOpenDeployment?: (deploymentID: string) => void
  onCloseDeployment?: () => void
}

type ResultFilter = 'all' | 'complete' | 'attention' | 'recorded'

const matchesResult = (deployment: DeploymentSummary, filter: ResultFilter) => {
  if (filter === 'all') return true
  const label = deploymentOutcome(deployment).label
  if (filter === 'complete') return label === 'Complete'
  if (filter === 'attention') return label === 'Needs attention'
  return label === 'Recorded'
}

function HistoricalDeploymentDetail({ detail, onBack }: { detail: DeploymentDetail; onBack: () => void }) {
  const outcome = deploymentOutcome(detail)
  const systems = detail.providers.length ? detail.providers.map((provider) => displayProvider(provider.name)).join(', ') : 'Not recorded'
  return <section className="record-page history-detail-page" aria-labelledby="history-detail-title">
    <button className="history-back-button" type="button" aria-label="Back to deployment history" onClick={onBack}><span aria-hidden="true">←</span> Deployment history</button>
    <header className="record-page-heading history-detail-heading"><div><p className="overview-eyebrow">Historical deployment</p><h1 id="history-detail-title">{detail.name || detail.id}</h1><p>Read-only results captured when this deployment finished.</p></div><box-badge label={outcome.label} tone={outcome.tone}></box-badge></header>
    <div className="history-detail-layout">
      <section className="history-provider-summary" aria-labelledby="provider-summary-title">
        <header><div><h2 id="provider-summary-title">Provider summary</h2><p>Recorded configuration results for each deployed system.</p></div></header>
        <ul>{detail.providers.map((provider) => {
          const providerOutcome = deploymentOutcome({ ...detail, providers: [provider] })
          return <li key={provider.name}><header><strong>{displayProvider(provider.name)}</strong><box-badge label={providerOutcome.label} tone={providerOutcome.tone}></box-badge></header><dl><div><dt>Deployed</dt><dd>{provider.deployedCount}</dd></div><div><dt>Present</dt><dd>{provider.presentCount}</dd></div><div><dt>Remaining</dt><dd>{provider.remainingCount}</dd></div><div><dt>Manual</dt><dd>{provider.manualItemCount}</dd></div></dl></li>
        })}</ul>
      </section>
      <DetailsRail title="Deployment summary" description="The immutable audit record for this run."><DetailList rows={[["Deployment ID", detail.id], ["Run ID", detail.runId || 'Not recorded'], ["Systems", systems], ["Strategy", displayStrategy(detail.strategy)], ["Started", formatDeploymentDate(detail.startedAt)], ["Completed", formatDeploymentDate(detail.completedAt)], ["Duration", detail.duration || 'Not recorded']]}/></DetailsRail>
    </div>
  </section>
}

export function HistoryPage({ deployments, selectedDeploymentID = null, onOpenDeployment = () => undefined, onCloseDeployment = () => undefined }: HistoryPageProps) {
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
    if (detail?.id === selectedDeploymentID) return <HistoricalDeploymentDetail detail={detail} onBack={onCloseDeployment}/>
    const selectedError = detailError?.deploymentID === selectedDeploymentID ? detailError.message : ''
    return <section className="record-page history-detail-page" aria-labelledby="history-detail-state"><button className="history-back-button" type="button" aria-label="Back to deployment history" onClick={onCloseDeployment}><span aria-hidden="true">←</span> Deployment history</button><div className="history-detail-state" role={selectedError ? 'alert' : 'status'}><h1 id="history-detail-state">{selectedError ? 'Deployment summary unavailable' : 'Loading deployment summary'}</h1><p>{selectedError || 'Reading the historical audit record…'}</p></div></section>
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
      <div><span>Total deployments</span><strong>{deployments.length}</strong></div>
      <div><span>Complete</span><strong>{complete}</strong></div>
      <div><span>Needs attention</span><strong>{attention}</strong></div>
    </section>
    <section className="record-page-section" aria-labelledby="all-deployments-title"><header><div><h2 id="all-deployments-title">All deployments</h2><p>Filter the audit trail, then open any deployment for its recorded summary.</p></div><span>{filteredDeployments.length === deployments.length ? `${deployments.length} recorded` : `Showing ${filteredDeployments.length} of ${deployments.length}`}</span></header>
      <fieldset className="history-filters"><legend className="visually-hidden">Filter deployment history</legend><label>Search<input type="search" value={query} placeholder="Name or deployment ID" onChange={(event) => setQuery(event.target.value)}/></label><label>System<select value={providerFilter} onChange={(event) => setProviderFilter(event.target.value)}><option value="all">All systems</option>{providerOptions.map((provider) => <option key={provider} value={provider}>{displayProvider(provider)}</option>)}</select></label><label>Result<select value={resultFilter} onChange={(event) => setResultFilter(event.target.value as ResultFilter)}><option value="all">All results</option><option value="complete">Complete</option><option value="attention">Needs attention</option><option value="recorded">Recorded</option></select></label><label>Strategy<select value={strategyFilter} onChange={(event) => setStrategyFilter(event.target.value)}><option value="all">All strategies</option><option value="reuse">Reuse existing</option><option value="create_new">Create new</option></select></label><button type="button" className="history-clear-filters" onClick={clearFilters} disabled={!filtersActive}>Clear filters</button></fieldset>
      <DeploymentHistoryTable deployments={filteredDeployments} caption="All deployments" includeResult emptyMessage={filtersActive ? 'No deployments match these filters.' : undefined} onSelect={onOpenDeployment}/>
    </section>
  </section>
}
