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

type SolutionTemplate = {
  id: string
  name: string
  sector?: string
  description?: string
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

const fallbackTemplates: SolutionTemplate[] = [
  { id: 'clm', name: 'Contract Lifecycle Management', sector: 'Legal operations', description: 'Content-centric contract workflows with Box and intelligent agents.' },
  { id: 'lifesciences', name: 'Life Sciences', sector: 'Regulated content', description: 'Accelerate document-heavy life sciences processes and insight.' },
  { id: 'citizen-services', name: 'Citizen Services', sector: 'Public sector', description: 'Modernize constituent intake, case content, and service delivery.' },
  { id: 'new', name: 'Create a New Solution', sector: 'Starter', description: 'Begin with the Box Dispatch reference architecture and shape your own solution.' },
]

function App() {
	const [activePhase, setActivePhase] = useState<Phase>('Review')
	const [deployments, setDeployments] = useState<Deployment[]>(fallbackDeployments)
  const [plan, setPlan] = useState<DeploymentPlan>(fallbackPlan)
	const [templates, setTemplates] = useState<SolutionTemplate[]>(fallbackTemplates)
	const [selectedTemplateID, setSelectedTemplateID] = useState('clm')
	const [selectedComponents, setSelectedComponents] = useState<string[]>(['box', 'salesforce'])
	const [assembling, setAssembling] = useState(false)
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
		fetchJSON<SolutionTemplate[]>('/api/templates', controller.signal),
	]).then(([deploymentsResult, planResult, runsResult, templatesResult]) => {
		if (deploymentsResult.status === 'fulfilled' && deploymentsResult.value.length > 0) setDeployments(deploymentsResult.value)
		if (runsResult.status === 'fulfilled') setRecentRuns(runsResult.value)
		if (templatesResult.status === 'fulfilled' && templatesResult.value.length > 0) {
			setTemplates(templatesResult.value)
			setSelectedTemplateID((current) => templatesResult.value.some((template) => template.id === current) ? current : templatesResult.value[0].id)
		}
		if (planResult.status === 'fulfilled' && planResult.value.exists) {
			setPlan(planResult.value)
			setActivePhase('Review')
			setNotice('Saved BCL plan loaded. Review its selected providers before deployment.')
		} else {
			setActivePhase('Choose')
			setNotice('Choose a supported solution quickstart to begin a new deployment.')
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

	const beginNewDeployment = () => {
		setRun(null)
		setRunEvents([])
		setActivePhase('Choose')
		setNotice('Choose a supported solution quickstart, then Dispatch will assemble its BCL package locally.')
	}

	const toggleSalesforce = () => {
		setSelectedComponents((components) => components.includes('salesforce') ? ['box'] : ['box', 'salesforce'])
	}

	const assemblePackage = () => {
		setAssembling(true)
		setNotice('Assembling the selected template locally. Dispatch is preparing the BCL package…')
		void fetch('/api/packages', {
			method: 'POST', headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ templateId: selectedTemplateID, components: selectedComponents }),
		}).then(async (response) => {
			if (!response.ok) throw new Error('Package could not be assembled.')
			return (await response.json()) as DeploymentPlan
		}).then((nextPlan) => {
			setPlan(nextPlan)
			setActivePhase('Review')
			setNotice('Package assembled and saved locally as BCL. Review the plan before validation.')
		}).catch(() => setNotice('Dispatch could not assemble this template. Check the local Dispatch terminal, then try again.')).finally(() => setAssembling(false))
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
          <button className="nav-link nav-button" type="button" onClick={beginNewDeployment}>New deployment</button>
          <a className="nav-link" href="#history">History</a>
          <a className="nav-link" href="#settings">Settings</a>
        </nav>
        <div className="sidebar-footer"><span className="avatar" aria-hidden="true">AD</span><span><strong>Local operator</strong><small>Development profile</small></span></div>
      </aside>

      <main id="workspace" className="workspace">
        <DeploymentHeader plan={plan} activePhase={activePhase} />

		{activePhase === 'Choose' ? <ChooseView templates={templates} selectedTemplateID={selectedTemplateID} selectedComponents={selectedComponents} assembling={assembling} notice={notice} onTemplateChange={setSelectedTemplateID} onToggleSalesforce={toggleSalesforce} onAssemble={assemblePackage} /> : activePhase === 'Deploy' ? <DeployView plan={plan} run={run} events={runEvents} notice={notice} onApply={applyDeployment} onDiagnostics={openDiagnostics} /> : <ReviewView plan={plan} notice={notice} onSave={savePlan} onDeploy={beginValidation} onConnections={openConnectionDrawer} />}

        <section id="history" className="history" aria-labelledby="history-title">
          <div className="section-heading"><div><h2 id="history-title">Recent deployments</h2></div><a href="#history">View full history</a></div>
          <div className="table-wrap"><table><thead><tr><th>Deployment</th><th>Providers</th><th>Stages</th><th>Completed</th></tr></thead><tbody>{deployments.map((deployment) => <tr key={deployment.id}><td><strong>{deployment.name}</strong><span>{deployment.id}</span></td><td>{deployment.providers.map((provider) => provider.name).join(' + ')}</td><td><StageTrack complete /></td><td>{formatCompletedAt(deployment.completedAt)}</td></tr>)}</tbody></table></div>
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

function DeploymentHeader({ plan, activePhase }: { plan: DeploymentPlan; activePhase: Phase }) {
  const readiness = plan.components.every((component) => component.ready) ? 'Ready to validate' : 'Connection needs attention'
  const isRunning = activePhase === 'Deploy'
  return <header className="deployment-header"><div className="breadcrumbs"><a href="#workspace">Deployments</a><span aria-hidden="true">/</span><span>{isRunning ? 'Validation run' : plan.template || 'New deployment'}</span></div><div className="header-row"><div><h1>{plan.template || 'Deployment plan'}</h1><div className="deployment-meta"><span className={isRunning ? 'meta-status running' : 'meta-status ready'}>{isRunning ? 'Validation in progress' : readiness}</span><span>{plan.components.map((component) => component.name).join(' + ') || 'Choose providers'}</span><span>Local BCL package</span></div></div><button className="environment" type="button" aria-label="Selected environment"><span>Environment</span><strong>Development</strong></button></div></header>
}

function StageTrack({ complete }: { complete: boolean }) {
  return <span className="stage-track" aria-label={complete ? 'All deployment stages completed' : 'Deployment stages in progress'}>{phases.map((phase, index) => <span className={complete || index === 0 ? 'complete' : ''} key={phase}>{complete || index === 0 ? '✓' : ''}</span>)}</span>
}

async function fetchJSON<T>(path: string, signal: AbortSignal): Promise<T> {
  const response = await fetch(path, { signal })
  if (!response.ok) throw new Error(`${path} is unavailable.`)
  return (await response.json()) as T
}

function ReviewView({ plan, notice, onSave, onDeploy, onConnections }: { plan: DeploymentPlan; notice: string; onSave: () => void; onDeploy: () => void; onConnections: () => void }) {
  const status = plan.components.every((component) => component.ready) ? 'Ready' : 'Needs attention'
  const ready = status === 'Ready'
  return <section className="review-layout" aria-label="Review deployment plan"><article className="task-surface review-surface"><header className="task-heading"><div><h2>{ready ? 'Ready to validate' : 'Connection needs attention'}</h2><p>{ready ? 'Dispatch will check this plan before it applies any change.' : 'Choose a verified connection before starting validation.'}</p></div><span className={`status ${ready ? 'ready' : 'running'}`}>{status}</span></header><div className="review-summary"><div><span>Solution</span><strong>{plan.template}</strong></div><div><span>Strategy</span><strong>Reuse existing</strong></div><div><span>Providers</span><strong>{plan.components.map((component) => component.name).join(' + ')}</strong></div></div><div className="review-details"><PlanGroup title="Package" rows={[["Source", plan.repository]]} />{plan.components.map((component) => <PlanGroup key={component.id} title={component.name} rows={[["Connection", component.ready ? 'Verified and ready' : component.configured ? 'Configured — needs verification' : 'Not configured']]} />)}</div><p className="notice" role="status">{notice}</p><footer className="action-row"><button className="text-button" type="button" onClick={onSave}>Save plan</button><box-button label="Validate configuration" tone="primary" onClick={onDeploy}></box-button></footer></article><ActivityRail events={[]} onConnections={onConnections} /></section>
}

function ChooseView({ templates, selectedTemplateID, selectedComponents, assembling, notice, onTemplateChange, onToggleSalesforce, onAssemble }: { templates: SolutionTemplate[]; selectedTemplateID: string; selectedComponents: string[]; assembling: boolean; notice: string; onTemplateChange: (templateID: string) => void; onToggleSalesforce: () => void; onAssemble: () => void }) {
  return <section className="choose-layout" aria-label="Choose deployment"><article className="task-surface choose-card"><header className="task-heading"><div><h2>Choose a starting point</h2><p>Dispatch prepares a local BCL package. You review it before any provider change.</p></div></header><div className="template-grid" role="radiogroup" aria-label="Solution quickstarts">{templates.map((template) => <button className={`template-choice ${template.id === selectedTemplateID ? 'selected' : ''}`} type="button" role="radio" aria-checked={template.id === selectedTemplateID} key={template.id} onClick={() => onTemplateChange(template.id)} disabled={assembling}><span className="template-sector">{template.sector || 'Solution'}</span><strong>{template.name}</strong><small>{template.description}</small></button>)}</div><section className="provider-choice"><div><h3>Include Salesforce?</h3><p>Box is required. Add Salesforce only for CRM records or customer workflows.</p></div><label className="provider-option"><input type="checkbox" checked={selectedComponents.includes('salesforce')} onChange={onToggleSalesforce} disabled={assembling} /><span><strong>Salesforce</strong><small>Optional CRM and record workflows</small></span></label></section><p className="notice" role="status">{notice}</p><footer className="action-row"><box-button label={assembling ? 'Preparing package…' : 'Continue to review'} tone="primary" disabled={assembling} onClick={onAssemble}></box-button></footer></article><aside className="activity-card choose-aside"><h2>How it works</h2><ol className="next-steps"><li><span>1</span><div><strong>Prepare</strong><small>Dispatch creates a local BCL package.</small></div></li><li><span>2</span><div><strong>Validate</strong><small>It checks only the selected providers.</small></div></li><li><span>3</span><div><strong>Apply</strong><small>You explicitly approve the changes.</small></div></li></ol></aside></section>
}

function DeployView({ plan, run, events, notice, onApply, onDiagnostics }: { plan: DeploymentPlan; run: DispatchRun | null; events: RunEvent[]; notice: string; onApply: () => void; onDiagnostics: (runID: string) => void }) {
  const title = run?.action === 'deploy' ? 'Applying configuration' : 'Validating configuration'
  const state = run?.status ?? 'queued'
  const status = state === 'completed' ? 'Complete' : state === 'failed' ? 'Needs attention' : 'In progress'
  const providerRows = presentProviderProgress(plan.components, run, events)
  const completed = providerRows.filter((provider) => provider.state === 'complete').length
  return <section className="review-layout" aria-label="Live deployment progress"><article className="task-surface deploy-card"><header className="task-heading"><div><h2>{title}</h2><p>{state === 'running' ? 'Dispatch follows each provider check as it completes.' : 'Provider results are retained with this local run.'}</p></div><span className={`status ${state === 'failed' ? 'running' : state === 'completed' ? 'ready' : 'running'}`}>{status}</span></header><div className="run-summary"><strong>{state === 'running' ? `${completed} of ${providerRows.length} providers checked` : `${providerRows.length} provider${providerRows.length === 1 ? '' : 's'} in this run`}</strong><span>{state === 'running' ? 'Checks run in order and stop at the first provider that needs attention.' : 'Review the provider result before continuing.'}</span></div><ol className="provider-timeline" aria-label="Provider validation progress">{providerRows.map((provider, index) => <ProviderTimelineItem key={provider.name} provider={provider} index={index} events={events} />)}<li className="timeline-item pending"><span className="timeline-node">{providerRows.length + 1}</span><div><strong>{run?.action === 'deploy' ? 'Complete deployment' : 'Review results'}</strong><span>{run?.action === 'deploy' ? 'Finishing the local deployment record' : 'Available after all selected providers finish'}</span></div></li></ol><p className="notice" role="status">{notice}</p><footer className="action-row">{run?.action === 'validate' && run.status === 'completed' && <box-button label="Apply validated changes" tone="primary" onClick={onApply}></box-button>}{run?.status === 'failed' && <button className="text-button" type="button" onClick={() => onDiagnostics(run.id)}>View diagnostic guidance</button>}</footer></article><ActivityRail events={events} isRunning={state === 'queued' || state === 'running'} /></section>
}

function ProviderTimelineItem({ provider, index, events }: { provider: { name: string; state: string; detail: string }; index: number; events: RunEvent[] }) {
  const updates = events.filter((event) => event.provider === provider.name.toLowerCase() && event.type === 'activity').slice(-3)
  return <li className={`timeline-item ${provider.state}`}><span className="timeline-node">{provider.state === 'complete' ? '✓' : provider.state === 'active' ? '…' : index + 1}</span><div><strong>Checking {provider.name}</strong><span>{provider.detail}</span>{updates.length > 0 && <ul className="provider-updates">{updates.map((event) => <li key={event.sequence}>{event.message}</li>)}</ul>}{provider.state === 'active' && <span className="stage-pulse" aria-label="Checking in progress"></span>}</div></li>
}

function presentProviderProgress(components: PlanComponent[], run: DispatchRun | null, events: RunEvent[]) {
  const currentProvider = [...events].reverse().find((event) => event.type === 'activity' && event.provider)?.provider
  return components.map((component, index) => {
    const result = run?.providers.find((provider) => provider.name.toLowerCase() === component.id)
    if (result) return { name: component.name, state: result.status === 'failed' ? 'failed' : 'complete', detail: result.status === 'present' ? 'Validated and ready' : result.status }
    if (run?.status === 'failed' && currentProvider === component.id) return { name: component.name, state: 'failed', detail: 'Validation needs attention' }
    if (run?.status === 'running' && currentProvider === component.id) return { name: component.name, state: 'active', detail: latestProviderMessage(events, component.id) }
    if ((run?.status === 'running' || run?.status === 'queued') && index === 0 && !currentProvider) return { name: component.name, state: 'active', detail: 'Preparing local validation checks' }
    return { name: component.name, state: 'pending', detail: 'Queued for validation' }
  })
}

function latestProviderMessage(events: RunEvent[], provider: string) {
  return [...events].reverse().find((event) => event.provider === provider)?.message ?? 'Checking configuration'
}

function PlanGroup({ title, rows }: { title: string; rows: [string, string][] }) {
  return <section className="plan-group"><h3>{title}</h3>{rows.map(([label, value]) => <div className="plan-row" key={label}><span>{label}</span><strong>{value}</strong></div>)}</section>
}

function ActivityRail({ events, isRunning = false, onConnections }: { events: RunEvent[]; isRunning?: boolean; onConnections?: () => void }) {
  return <aside className="activity-card" aria-labelledby="activity-title"><div className="section-heading"><div><h2 id="activity-title">Activity</h2></div>{isRunning && <span className="live-dot">Live</span>}</div>{events.length === 0 ? <p className="empty-activity">{isRunning ? 'Connecting to the local run stream…' : 'Run activity will appear here.'}</p> : <ol className="activity-list">{events.map((event, index) => <li className={index === events.length - 1 ? 'current' : ''} key={event.sequence}><span>{event.status === 'completed' ? '✓' : event.status === 'failed' ? '!' : '•'}</span><div><strong>{event.message}</strong><small>{event.provider || 'Dispatch'}</small></div></li>)}</ol>}<div className="target-summary"><strong>Connections</strong><span>Box is managed in Dispatch CLI.</span>{onConnections ? <button className="text-button" type="button" onClick={onConnections}>Change Salesforce org</button> : <span>Salesforce connection is locked while this run is active.</span>}</div></aside>
}

function DiagnosticsDrawer({ diagnostic, onClose }: { diagnostic: RunDiagnostic | null; onClose: () => void }) {
	return <div className="drawer-backdrop" role="presentation"><aside className="diagnostics-drawer" role="dialog" aria-modal="true" aria-labelledby="diagnostics-title"><button className="drawer-close" type="button" onClick={onClose} aria-label="Close diagnostics">×</button>{diagnostic ? <><h2 id="diagnostics-title">{diagnostic.title}</h2><p>{diagnostic.summary}</p><h3>Recommended next steps</h3><ol>{diagnostic.nextSteps.map((step) => <li key={step}>{step}</li>)}</ol><div className="cli-hint"><strong>Full provider detail</strong><p>{diagnostic.cliHint}</p></div></> : <><h2 id="diagnostics-title">Loading diagnostic guidance</h2><p>Dispatch is preparing safe next steps for this failed run.</p></>}</aside></div>
}

function ConnectionsDrawer({ options, selectedAlias, loading, onChange, onSave, onClose }: { options: SalesforceConnectionOption[]; selectedAlias: string; loading: boolean; onChange: (alias: string) => void; onSave: () => void; onClose: () => void }) {
	return <div className="drawer-backdrop" role="presentation"><aside className="diagnostics-drawer" role="dialog" aria-modal="true" aria-labelledby="connections-title"><button className="drawer-close" type="button" onClick={onClose} aria-label="Close connections">×</button><h2 id="connections-title">Salesforce connection</h2><p>Choose an org that is already authenticated in the Salesforce CLI. Selecting it clears its previous verification so Dispatch can recheck it before deployment.</p>{loading && options.length === 0 ? <p>Loading authenticated orgs…</p> : options.length === 0 ? <div className="cli-hint"><strong>No authenticated aliases found</strong><p>Authenticate an org and give it an alias with the Salesforce CLI, then reopen this panel.</p></div> : <><label className="connection-select" htmlFor="salesforce-org"><span>Authenticated org</span><select id="salesforce-org" value={selectedAlias} onChange={(event) => onChange(event.target.value)} disabled={loading}>{options.map((option) => <option key={option.alias} value={option.alias}>{option.alias} · {option.kind}{option.expiresAt ? ` · expires ${option.expiresAt}` : ''}</option>)}</select></label><p className="connection-footnote">Only aliases, status, and scratch-org expiration are shown here. Credentials remain in the local Salesforce CLI.</p><footer className="drawer-actions"><box-button label={loading ? 'Saving…' : 'Use this Salesforce org'} tone="primary" disabled={loading || !selectedAlias} onClick={onSave}></box-button></footer></>}</aside></div>
}

export default App
