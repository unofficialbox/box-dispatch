import { useEffect, useState } from 'react'
import './App.css'

type Phase = 'Choose' | 'Connect' | 'Configure' | 'Review' | 'Deploy'

type Deployment = {
  id: string
  name: string
  strategy: string
  completedAt: string
  providers: ProviderSummary[]
}

type ProviderSummary = { name: string; status: string }

type PlanComponent = {
  id: string
  name: string
  configured: boolean
  verified: boolean
  ready: boolean
}

type DeploymentPlan = {
  exists: boolean
  templateId: string
  template: string
  repository: string
  components: PlanComponent[]
}

type DispatchRun = {
  id: string
  action: 'validate' | 'deploy'
  status: 'queued' | 'running' | 'completed' | 'failed'
  providers: ProviderSummary[]
}

type RunDiagnostic = {
  title: string
  summary: string
  nextSteps: string[]
  cliHint: string
}

type RunEvent = {
  sequence: number
  at: string
  type: 'status' | 'activity'
  provider?: string
  message: string
  status: DispatchRun['status']
}

type SalesforceConnectionOption = {
  alias: string
  kind: string
  status: string
  expiresAt?: string
  selected: boolean
}

const phases: Phase[] = ['Choose', 'Connect', 'Configure', 'Review', 'Deploy']

const fallbackDeployments: Deployment[] = [
  { id: 'dispatch-20260821-001', name: 'CLM deployment', strategy: 'Reuse existing', completedAt: '2026-08-21T10:42:00Z', providers: [{ name: 'Box', status: 'present' }, { name: 'Salesforce', status: 'present' }] },
  { id: 'dispatch-20260820-002', name: 'Contract workspace refresh', strategy: 'Create new', completedAt: '2026-08-20T16:08:00Z', providers: [{ name: 'Box', status: 'present' }] },
]

const fallbackPlan: DeploymentPlan = {
  exists: false,
  templateId: 'clm',
  template: 'CLM deployment',
  repository: 'https://github.com/unofficialbox/box-bedrock-for-clm',
  components: [
    { id: 'box', name: 'Box', configured: true, verified: true, ready: true },
    { id: 'salesforce', name: 'Salesforce', configured: true, verified: true, ready: true },
  ],
}

function App() {
	const [activePhase, setActivePhase] = useState<Phase>('Review')
	const [deployments, setDeployments] = useState<Deployment[]>(fallbackDeployments)
  const [plan, setPlan] = useState<DeploymentPlan>(fallbackPlan)
  const [run, setRun] = useState<DispatchRun | null>(null)
	const [recentRuns, setRecentRuns] = useState<DispatchRun[]>([])
  const [runEvents, setRunEvents] = useState<RunEvent[]>([])
	const [diagnostic, setDiagnostic] = useState<RunDiagnostic | null>(null)
	const [diagnosticRunID, setDiagnosticRunID] = useState<string | null>(null)
	const [connectionDrawerOpen, setConnectionDrawerOpen] = useState(false)
	const [salesforceOptions, setSalesforceOptions] = useState<SalesforceConnectionOption[]>([])
	const [selectedSalesforceAlias, setSelectedSalesforceAlias] = useState('')
	const [connectionsLoading, setConnectionsLoading] = useState(false)
  const [notice, setNotice] = useState('Loading the saved deployment plan…')
  const activeRunID = run && (run.status === 'queued' || run.status === 'running') ? run.id : null
	const refreshRunHistory = () => {
		void fetchJSON<DispatchRun[]>('/api/runs', new AbortController().signal)
			.then(setRecentRuns)
			.catch(() => undefined)
	}

  useEffect(() => {
    const controller = new AbortController()
	void Promise.allSettled([
		fetchJSON<Deployment[]>('/api/deployments', controller.signal),
		fetchJSON<DeploymentPlan>('/api/plan', controller.signal),
		fetchJSON<DispatchRun[]>('/api/runs', controller.signal),
	]).then(([deploymentsResult, planResult, runsResult]) => {
		if (deploymentsResult.status === 'fulfilled' && deploymentsResult.value.length > 0) setDeployments(deploymentsResult.value)
		if (runsResult.status === 'fulfilled') setRecentRuns(runsResult.value)
		if (planResult.status === 'fulfilled' && planResult.value.exists) {
			setPlan(planResult.value)
			setNotice('Saved BCL plan loaded. Review its selected providers before deployment.')
		} else {
			setNotice('No saved BCL plan is available yet. Start a deployment in Dispatch to create one.')
		}
	})
    return () => controller.abort()
  }, [])

  useEffect(() => {
    if (!activeRunID) return
    const stream = new EventSource(`/api/runs/${activeRunID}/events`)
    const receiveEvent = (message: MessageEvent<string>) => {
      const event = JSON.parse(message.data) as RunEvent
      setRunEvents((events) => [...events.slice(-19), event])
      if (event.type === 'status' && (event.status === 'completed' || event.status === 'failed')) {
        setRun((current) => current ? { ...current, status: event.status } : current)
			refreshRunHistory()
        stream.close()
      }
    }
    stream.addEventListener('dispatch', receiveEvent)
    stream.onerror = () => stream.close()
    return () => stream.close()
  }, [activeRunID])

  const beginValidation = () => {
    void fetch('/api/runs', { method: 'POST' }).then(async (response) => {
      if (!response.ok) throw new Error('Validation could not start.')
      return (await response.json()) as DispatchRun
    }).then((nextRun) => {
      setRun(nextRun)
      setRunEvents([])
      setActivePhase('Deploy')
      setNotice('Validation started. Dispatch is following the live run.')
    }).catch(() => setNotice('Validation could not start. Assemble a package in Dispatch, then try again.'))
  }

  const applyDeployment = () => {
    if (!run) return
    void fetch(`/api/runs/${run.id}/deploy`, { method: 'POST' }).then(async (response) => {
      if (!response.ok) throw new Error('Deployment could not start.')
      return (await response.json()) as DispatchRun
    }).then((nextRun) => {
      setRun(nextRun)
      setRunEvents([])
      setNotice('Deployment started. Only validated supported changes will be applied.')
    }).catch(() => setNotice('Deployment could not start. Complete a successful validation first.'))
  }

	const openDiagnostics = (runID: string) => {
		setDiagnosticRunID(runID)
		setDiagnostic(null)
		void fetch(`/api/runs/${runID}/diagnostics`).then(async (response) => {
			if (!response.ok) throw new Error('Diagnostic guidance is unavailable.')
			return (await response.json()) as RunDiagnostic
		}).then(setDiagnostic).catch(() => setNotice('Diagnostic guidance is unavailable. Open the failed run in the Dispatch terminal for the original provider output.'))
	}

	const openConnectionDrawer = () => {
		setConnectionDrawerOpen(true)
		setConnectionsLoading(true)
		void fetch('/api/connections/salesforce/options').then(async (response) => {
			if (!response.ok) throw new Error('Authenticated Salesforce orgs are unavailable.')
			return (await response.json()) as SalesforceConnectionOption[]
		}).then((options) => {
			setSalesforceOptions(options)
			setSelectedSalesforceAlias(options.find((option) => option.selected)?.alias ?? options[0]?.alias ?? '')
		}).catch(() => setNotice('Authenticated Salesforce orgs are unavailable. Connect one with the Salesforce CLI, then try again.')).finally(() => setConnectionsLoading(false))
	}

	const selectSalesforceConnection = () => {
		if (!selectedSalesforceAlias) return
		setConnectionsLoading(true)
		void fetch('/api/connections/salesforce', {
			method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ alias: selectedSalesforceAlias }),
		}).then(async (response) => {
			if (!response.ok) throw new Error('Salesforce org could not be selected.')
			return await response.json()
		}).then(() => {
			setPlan((current) => ({ ...current, components: current.components.map((component) => component.id === 'salesforce' ? { ...component, configured: true, verified: false, ready: false } : component) }))
			setConnectionDrawerOpen(false)
			setNotice('Salesforce org saved. Validate configuration to verify the new connection.')
		}).catch(() => setNotice('Salesforce org could not be selected. Choose an authenticated org and try again.')).finally(() => setConnectionsLoading(false))
	}

  const savePlan = () => {
		void fetch('/api/plan', {
			method: 'PUT',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({
				templateId: plan.templateId,
				template: plan.template,
				repository: plan.repository,
				components: plan.components.map((component) => component.id),
			}),
		}).then(async (response) => {
			if (!response.ok) throw new Error('Plan could not be saved.')
			return (await response.json()) as DeploymentPlan
		}).then((savedPlan) => {
			setPlan(savedPlan)
			setNotice('Plan saved locally as BCL.')
		}).catch(() => setNotice('Plan could not be saved. Start the local Dispatch API and try again.'))
	}

  return (
    <div className="app-shell">
      <aside className="sidebar" aria-label="Main navigation">
        <a className="brand" href="#workspace" aria-label="Box Dispatch home"><span className="brand-icon" aria-hidden="true">B/</span><span>Dispatch</span></a>
        <nav>
          <a className="nav-link active" href="#workspace">Deployments</a>
          <a className="nav-link" href="#new">New deployment</a>
          <a className="nav-link" href="#history">History</a>
          <a className="nav-link" href="#settings">Settings</a>
        </nav>
        <div className="sidebar-footer"><span className="avatar" aria-hidden="true">AD</span><span><strong>Local operator</strong><small>Development profile</small></span></div>
      </aside>

      <main id="workspace" className="workspace">
        <header className="topbar">
			<div><h1>{plan.template || 'Deployment plan'}</h1></div>
          <button className="environment" type="button" aria-label="Selected environment"><span>Environment</span><strong>Development</strong></button>
        </header>

        <ol className="stepper" aria-label="Deployment stages">
          {phases.map((phase, index) => {
            const current = phases.indexOf(activePhase)
            const state = index < current ? 'complete' : index === current ? 'active' : 'pending'
            return <li className={`step ${state}`} key={phase}><span className="step-index">{state === 'complete' ? '✓' : index + 1}</span><span>{phase}</span></li>
          })}
        </ol>

		{activePhase === 'Deploy' ? <DeployView run={run} events={runEvents} notice={notice} onApply={applyDeployment} onDiagnostics={openDiagnostics} /> : <ReviewView plan={plan} notice={notice} onSave={savePlan} onDeploy={beginValidation} onConnections={openConnectionDrawer} />}

        <section id="history" className="history" aria-labelledby="history-title">
          <div className="section-heading"><div><h2 id="history-title">Recent deployments</h2></div><a href="#history">View full history</a></div>
          <div className="table-wrap"><table><thead><tr><th>Deployment</th><th>Providers</th><th>Strategy</th><th>Completed</th></tr></thead><tbody>{deployments.map((deployment) => <tr key={deployment.id}><td><strong>{deployment.name}</strong><span>{deployment.id}</span></td><td>{deployment.providers.map((provider) => provider.name).join(' + ')}</td><td>{deployment.strategy}</td><td>{formatCompletedAt(deployment.completedAt)}</td></tr>)}</tbody></table></div>
        </section>
		{recentRuns.length > 0 && <section className="run-history" aria-labelledby="run-history-title"><div className="section-heading"><div><h2 id="run-history-title">Recent web runs</h2></div><span>{recentRuns.length} saved locally</span></div><ol>{recentRuns.slice(0, 5).map((savedRun) => <li key={savedRun.id}><div><strong>{savedRun.action === 'deploy' ? 'Apply configuration' : 'Validate configuration'}</strong><span>{savedRun.id}</span></div><span className={`status ${savedRun.status === 'completed' ? 'ready' : 'running'}`}>{savedRun.status === 'failed' ? 'Needs attention' : savedRun.status}</span>{savedRun.status === 'failed' && <button className="text-button" type="button" onClick={() => openDiagnostics(savedRun.id)}>View diagnostics</button>}</li>)}</ol></section>}
      </main>
		{diagnosticRunID && <DiagnosticsDrawer diagnostic={diagnostic} onClose={() => setDiagnosticRunID(null)} />}
		{connectionDrawerOpen && <ConnectionsDrawer options={salesforceOptions} selectedAlias={selectedSalesforceAlias} loading={connectionsLoading} onChange={setSelectedSalesforceAlias} onSave={selectSalesforceConnection} onClose={() => setConnectionDrawerOpen(false)} />}
    </div>
  )
}

function formatCompletedAt(value: string) {
  const timestamp = new Date(value)
  if (Number.isNaN(timestamp.getTime())) return value
  return new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' }).format(timestamp)
}

async function fetchJSON<T>(path: string, signal: AbortSignal): Promise<T> {
  const response = await fetch(path, { signal })
  if (!response.ok) throw new Error(`${path} is unavailable.`)
  return (await response.json()) as T
}

function ReviewView({ plan, notice, onSave, onDeploy, onConnections }: { plan: DeploymentPlan; notice: string; onSave: () => void; onDeploy: () => void; onConnections: () => void }) {
  const status = plan.components.every((component) => component.ready) ? 'Ready' : 'Needs attention'
  return <section className="review-layout" aria-label="Review deployment plan"><article className="plan-card"><div className="section-heading"><div><h2>Review deployment plan</h2></div><span className={`status ${status === 'Ready' ? 'ready' : 'running'}`}>{status}</span></div><PlanGroup title="Solution" rows={[["Template", plan.template], ["Repository", plan.repository], ["Deployment strategy", "Reuse existing"]]} />{plan.components.map((component) => <PlanGroup key={component.id} title={component.name} rows={[["Selected", "Included in this plan"], ["Connection", component.ready ? 'Verified and ready' : component.configured ? 'Configured — needs verification' : 'Not configured']]} />)}<p className="notice" role="status">{notice}</p><footer className="action-row"><box-button label="Save plan" tone="secondary" onClick={onSave}></box-button><box-button label="Validate configuration" tone="primary" onClick={onDeploy}></box-button></footer></article><ActivityRail events={[]} onConnections={onConnections} /></section>
}

function DeployView({ run, events, notice, onApply, onDiagnostics }: { run: DispatchRun | null; events: RunEvent[]; notice: string; onApply: () => void; onDiagnostics: (runID: string) => void }) {
  const title = run?.action === 'deploy' ? 'Applying configuration' : 'Validating configuration'
  const state = run?.status ?? 'queued'
  const status = state === 'completed' ? 'Complete' : state === 'failed' ? 'Needs attention' : 'In progress'
  return <section className="review-layout" aria-label="Live deployment progress"><article className="plan-card deploy-card"><div className="section-heading"><div><h2>{title}</h2></div><span className={`status ${state === 'failed' ? 'running' : state === 'completed' ? 'ready' : 'running'}`}>{status}</span></div><div className="stage-list">{(run?.providers ?? []).map((provider, index) => <div className={`deploy-stage ${provider.status === 'present' ? 'complete' : index === 0 && state === 'running' ? 'active' : ''}`} key={provider.name}><span className="stage-mark">{provider.status === 'present' ? '✓' : index + 1}</span><div><strong>{provider.name}</strong><span>{provider.status}</span></div></div>)}</div><p className="notice" role="status">{notice}</p><footer className="action-row">{run?.action === 'validate' && run.status === 'completed' && <box-button label="Apply validated changes" tone="primary" onClick={onApply}></box-button>}{run?.status === 'failed' && <button className="text-button" type="button" onClick={() => onDiagnostics(run.id)}>View diagnostic guidance</button>}</footer></article><ActivityRail events={events} /></section>
}

function PlanGroup({ title, rows }: { title: string; rows: [string, string][] }) {
  return <section className="plan-group"><h3>{title}</h3>{rows.map(([label, value]) => <div className="plan-row" key={label}><span>{label}</span><strong>{value}</strong></div>)}</section>
}

function ActivityRail({ events, onConnections }: { events: RunEvent[]; onConnections?: () => void }) {
  return <aside className="activity-card" aria-labelledby="activity-title"><div className="section-heading"><div><h2 id="activity-title">Activity</h2></div>{events.length > 0 && <span className="live-dot">Live</span>}</div>{events.length === 0 ? <p className="empty-activity">Run activity will appear here.</p> : <ol className="activity-list">{events.map((event, index) => <li className={index === events.length - 1 ? 'current' : ''} key={event.sequence}><span>{event.status === 'completed' ? '✓' : event.status === 'failed' ? '!' : '•'}</span><div><strong>{event.message}</strong><small>{event.provider || 'Dispatch'}</small></div></li>)}</ol>}<div className="target-summary"><strong>Connections</strong><span>Box is managed in Dispatch CLI.</span>{onConnections ? <button className="text-button" type="button" onClick={onConnections}>Change Salesforce org</button> : <span>Salesforce connection is locked while this run is active.</span>}</div></aside>
}

function DiagnosticsDrawer({ diagnostic, onClose }: { diagnostic: RunDiagnostic | null; onClose: () => void }) {
	return <div className="drawer-backdrop" role="presentation"><aside className="diagnostics-drawer" role="dialog" aria-modal="true" aria-labelledby="diagnostics-title"><button className="drawer-close" type="button" onClick={onClose} aria-label="Close diagnostics">×</button>{diagnostic ? <><h2 id="diagnostics-title">{diagnostic.title}</h2><p>{diagnostic.summary}</p><h3>Recommended next steps</h3><ol>{diagnostic.nextSteps.map((step) => <li key={step}>{step}</li>)}</ol><div className="cli-hint"><strong>Full provider detail</strong><p>{diagnostic.cliHint}</p></div></> : <><h2 id="diagnostics-title">Loading diagnostic guidance</h2><p>Dispatch is preparing safe next steps for this failed run.</p></>}</aside></div>
}

function ConnectionsDrawer({ options, selectedAlias, loading, onChange, onSave, onClose }: { options: SalesforceConnectionOption[]; selectedAlias: string; loading: boolean; onChange: (alias: string) => void; onSave: () => void; onClose: () => void }) {
	return <div className="drawer-backdrop" role="presentation"><aside className="diagnostics-drawer" role="dialog" aria-modal="true" aria-labelledby="connections-title"><button className="drawer-close" type="button" onClick={onClose} aria-label="Close connections">×</button><h2 id="connections-title">Salesforce connection</h2><p>Choose an org that is already authenticated in the Salesforce CLI. Selecting it clears its previous verification so Dispatch can recheck it before deployment.</p>{loading && options.length === 0 ? <p>Loading authenticated orgs…</p> : options.length === 0 ? <div className="cli-hint"><strong>No authenticated aliases found</strong><p>Authenticate an org and give it an alias with the Salesforce CLI, then reopen this panel.</p></div> : <><label className="connection-select" htmlFor="salesforce-org"><span>Authenticated org</span><select id="salesforce-org" value={selectedAlias} onChange={(event) => onChange(event.target.value)} disabled={loading}>{options.map((option) => <option key={option.alias} value={option.alias}>{option.alias} · {option.kind}{option.expiresAt ? ` · expires ${option.expiresAt}` : ''}</option>)}</select></label><p className="connection-footnote">Only aliases, status, and scratch-org expiration are shown here. Credentials remain in the local Salesforce CLI.</p><footer className="drawer-actions"><box-button label={loading ? 'Saving…' : 'Use this Salesforce org'} tone="primary" disabled={loading || !selectedAlias} onClick={onSave}></box-button></footer></>}</aside></div>
}

export default App
