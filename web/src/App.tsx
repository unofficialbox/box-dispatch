import { useEffect, useState } from 'react'
import './App.css'
import { BoxConnectionDrawer, DiagnosticsDrawer, SalesforceConnectionDrawer } from './components/Drawers'
import { DeploymentHeader } from './components/DeploymentHeader'
import { Sidebar } from './components/Sidebar'
import { ChoosePage } from './pages/ChoosePage'
import { ConnectPage } from './pages/ConnectPage'
import { ConfigurePage } from './pages/ConfigurePage'
import { DeployPage } from './pages/DeployPage'
import { OverviewPage } from './pages/OverviewPage'
import { ReviewPage } from './pages/ReviewPage'
import type { BoxConnectionInput, ConnectionSummary, DeploymentPlan, DeploymentSummary, DispatchRun, Phase, RunDiagnostic, RunEvent, SalesforceConnectionOption, SalesforceRESTInput, ScratchOrgJob, SolutionTemplate } from './types'

const fallbackPlan: DeploymentPlan = { exists: false, templateId: 'clm', template: 'CLM deployment', repository: 'https://github.com/unofficialbox/box-bedrock-for-clm', strategy: 'reuse', components: [{ id: 'box', name: 'Box', configured: true, verified: true, ready: true }, { id: 'salesforce', name: 'Salesforce', configured: true, verified: true, ready: true }] }
const fallbackTemplates: SolutionTemplate[] = [
  { id: 'clm', name: 'Contract Lifecycle Management', sector: 'Legal operations', description: 'Content-centric contract workflows with Box and intelligent agents.' },
  { id: 'lifesciences', name: 'Life Sciences', sector: 'Regulated content', description: 'Accelerate document-heavy life sciences processes and insight.' },
  { id: 'citizen-services', name: 'Citizen Services', sector: 'Public sector', description: 'Modernize constituent intake, case content, and service delivery.' },
  { id: 'new', name: 'Create a New Solution', sector: 'Starter', description: 'Begin with the Box Dispatch reference architecture and shape your own solution.' },
]

function App() {
  const [screen, setScreen] = useState<'overview' | 'workflow'>('overview')
  const [activePhase, setActivePhase] = useState<Phase>('Review')
  const [plan, setPlan] = useState<DeploymentPlan>(fallbackPlan)
  const [templates, setTemplates] = useState<SolutionTemplate[]>(fallbackTemplates)
  const [selectedTemplateID, setSelectedTemplateID] = useState('clm')
  const [selectedComponents, setSelectedComponents] = useState<string[]>(['box', 'salesforce'])
  const [componentSelections, setComponentSelections] = useState<Record<string, string[]>>({ box: ['Workspace structure', 'Metadata templates', 'Doc Gen templates', 'Sample content'], salesforce: ['CLM Contract', 'CLM Clause', 'Layouts', 'Permission sets'] })
  const [assembling, setAssembling] = useState(false)
  const [run, setRun] = useState<DispatchRun | null>(null)
  const [runEvents, setRunEvents] = useState<RunEvent[]>([])
  const [diagnostic, setDiagnostic] = useState<RunDiagnostic | null>(null)
  const [diagnosticRunID, setDiagnosticRunID] = useState<string | null>(null)
  const [connectionDrawerOpen, setConnectionDrawerOpen] = useState(false)
  const [boxConnectionDrawerOpen, setBoxConnectionDrawerOpen] = useState(false)
  const [salesforceOptions, setSalesforceOptions] = useState<SalesforceConnectionOption[]>([])
  const [connections, setConnections] = useState<ConnectionSummary[]>([])
  const [deployments, setDeployments] = useState<DeploymentSummary[]>([])
  const [selectedSalesforceAlias, setSelectedSalesforceAlias] = useState('')
  const [scratchJob, setScratchJob] = useState<ScratchOrgJob | null>(null)
  const [connectionsLoading, setConnectionsLoading] = useState(false)
  const [boxConnectionLoading, setBoxConnectionLoading] = useState(false)
  const [boxConnectionError, setBoxConnectionError] = useState('')
  const [notice, setNotice] = useState('Loading the saved deployment plan…')
  const activeRunID = run && (run.status === 'queued' || run.status === 'running') ? run.id : null

  useEffect(() => {
    const controller = new AbortController()
    void Promise.allSettled([fetchJSON<DeploymentPlan>('/api/plan', controller.signal), fetchJSON<SolutionTemplate[]>('/api/templates', controller.signal), fetchJSON<ConnectionSummary[]>('/api/connections', controller.signal), fetchJSON<DeploymentSummary[]>('/api/deployments', controller.signal)]).then(([planResult, templatesResult, connectionsResult, deploymentsResult]) => {
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
      if (connectionsResult.status === 'fulfilled') setConnections(connectionsResult.value)
      if (deploymentsResult.status === 'fulfilled') setDeployments(deploymentsResult.value)
    })
    return () => controller.abort()
  }, [])

  useEffect(() => {
    if (!activeRunID) return
    const stream = new EventSource(`/api/runs/${activeRunID}/events`)
    const receiveEvent = (message: MessageEvent<string>) => {
      const event = JSON.parse(message.data) as RunEvent
      setRunEvents((events) => events.some((existing) => existing.sequence === event.sequence) ? events : [...events, event])
      if (event.type === 'status' && (event.status === 'completed' || event.status === 'failed')) {
        setRun((current) => current ? { ...current, status: event.status } : current)
        void fetch(`/api/runs/${activeRunID}`).then(async (response) => {
          if (!response.ok) throw new Error('Run details are unavailable.')
          return (await response.json()) as DispatchRun
        }).then(setRun).catch(() => undefined)
        stream.close()
      }
    }
    stream.addEventListener('dispatch', receiveEvent)
    return () => stream.close()
  }, [activeRunID])

  const beginValidation = () => {
    void fetch('/api/runs', { method: 'POST' }).then(async (response) => {
      if (!response.ok) throw new Error('Validation could not start.')
      return (await response.json()) as DispatchRun
    }).then((nextRun) => { setRun(nextRun); setRunEvents([]); setActivePhase('Deploy'); setNotice('') }).catch(() => setNotice('Validation could not start. Assemble a package in Dispatch, then try again.'))
  }
  const applyDeployment = () => {
    if (!run) return
    void fetch(`/api/runs/${run.id}/deploy`, { method: 'POST' }).then(async (response) => {
      if (!response.ok) throw new Error('Deployment could not start.')
      return (await response.json()) as DispatchRun
    }).then((nextRun) => { setRun(nextRun); setRunEvents([]); setNotice('') }).catch(() => setNotice('Deployment could not start. Complete a successful validation first.'))
  }
  const openDiagnostics = (runID: string) => {
    setDiagnosticRunID(runID); setDiagnostic(null)
    void fetch(`/api/runs/${runID}/diagnostics`).then(async (response) => {
      if (!response.ok) throw new Error('Diagnostic guidance is unavailable.')
      return (await response.json()) as RunDiagnostic
    }).then(setDiagnostic).catch(() => setNotice('Diagnostic guidance is unavailable. Refresh the failed run and try again.'))
  }
  const openSalesforceConnection = () => {
    setScratchJob(null); setConnectionDrawerOpen(true); setConnectionsLoading(true)
    void fetch('/api/connections/salesforce/options').then(async (response) => {
      if (!response.ok) throw new Error('Authenticated Salesforce orgs are unavailable.')
      return (await response.json()) as SalesforceConnectionOption[]
    }).then((options) => { setSalesforceOptions(options); setSelectedSalesforceAlias(options.find((option) => option.selected)?.alias ?? options[0]?.alias ?? '') }).catch(() => setNotice('Salesforce environment details are unavailable. Add the REST connection in the Salesforce panel, then try again.')).finally(() => setConnectionsLoading(false))
  }
  const selectSalesforceConnection = async () => {
    if (!selectedSalesforceAlias) return false
    setConnectionsLoading(true)
    try {
      const response = await fetch('/api/connections/salesforce', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ alias: selectedSalesforceAlias }) })
      if (!response.ok) throw new Error('Salesforce org could not be selected.')
      const selection = await response.json() as ConnectionSummary
      setConnections((current) => [...current.filter((connection) => connection.name !== 'Salesforce'), selection])
      setPlan((current) => ({ ...current, components: current.components.map((component) => component.id === 'salesforce' ? { ...component, configured: true, verified: false, ready: false } : component) }))
      setNotice('Salesforce org saved. Validate configuration to verify the new connection.')
      return true
    } catch {
      setNotice('Salesforce org could not be selected. Choose an authenticated org and try again.')
      return false
    } finally {
      setConnectionsLoading(false)
    }
  }
  const openBoxConnection = () => {
    setBoxConnectionError('')
    setBoxConnectionDrawerOpen(true)
  }
  const closeBoxConnection = () => {
    setBoxConnectionError('')
    setBoxConnectionDrawerOpen(false)
  }
  const saveBoxConnection = async (input: BoxConnectionInput) => {
    setBoxConnectionError('')
    setBoxConnectionLoading(true)
    try {
      const response = await fetch('/api/connections/box', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(input) })
      if (!response.ok) throw new Error(await responseError(response, 'Box connection could not be saved.'))
      const selection = await response.json() as ConnectionSummary
      setConnections((current) => [...current.filter((connection) => connection.name !== 'Box'), selection])
      setPlan((current) => ({ ...current, components: current.components.map((component) => component.id === 'box' ? { ...component, configured: true, verified: selection.verified, ready: selection.verified } : component) }))
      setNotice(`${selection.alias || 'Box CCG'} connected and verified.`)
      return true
    } catch (error: unknown) {
      const message = error instanceof TypeError
        ? 'Dispatch’s local service is unavailable. Reopen Dispatch or wait for it to restart, then try again. Your credentials were not saved.'
        : error instanceof Error ? error.message : 'Box connection could not be saved. Check the CCG details and try again.'
      setBoxConnectionError(message)
      return false
    } finally {
      setBoxConnectionLoading(false)
    }
  }
  const verifyBoxConnection = async () => {
    setBoxConnectionError('')
    setBoxConnectionLoading(true)
    try {
      const response = await fetch('/api/connections/box/check', { method: 'POST' })
      if (!response.ok) throw new Error(await responseError(response, 'Box connection could not be verified.'))
      const selection = await response.json() as ConnectionSummary
      setConnections((current) => [...current.filter((connection) => connection.name !== 'Box'), selection])
      setPlan((current) => ({ ...current, components: current.components.map((component) => component.id === 'box' ? { ...component, configured: true, verified: selection.verified, ready: selection.verified } : component) }))
      setNotice(`${selection.alias || 'Box CCG'} is active and ready.`)
      return true
    } catch (error: unknown) {
      const message = error instanceof TypeError
        ? 'Dispatch’s local service is unavailable. Reopen Dispatch or wait for it to restart, then try again.'
        : error instanceof Error ? error.message : 'Box connection could not be verified.'
      setBoxConnectionError(message)
      return false
    } finally {
      setBoxConnectionLoading(false)
    }
  }
  const refreshSalesforceConnection = async () => {
    const [connectionResponse, optionsResponse] = await Promise.all([fetch('/api/connections'), fetch('/api/connections/salesforce/options')])
    if (!connectionResponse.ok || !optionsResponse.ok) throw new Error('Salesforce connection state could not be refreshed.')
    const nextConnections = await connectionResponse.json() as ConnectionSummary[]
    const nextOptions = await optionsResponse.json() as SalesforceConnectionOption[]
    setConnections(nextConnections)
    setSalesforceOptions(nextOptions)
    setSelectedSalesforceAlias(nextOptions.find((option) => option.selected)?.alias ?? nextOptions[0]?.alias ?? '')
    const salesforce = nextConnections.find((connection) => connection.name === 'Salesforce')
    setPlan((current) => ({ ...current, components: current.components.map((component) => component.id === 'salesforce' ? { ...component, configured: salesforce?.configured ?? false, verified: salesforce?.verified ?? false, ready: salesforce?.verified ?? false } : component) }))
  }
  const saveSalesforceREST = (input: SalesforceRESTInput) => {
    setConnectionsLoading(true)
    void fetch('/api/connections/salesforce/rest', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(input) }).then(async (response) => {
      if (!response.ok) throw new Error(await responseError(response, 'Salesforce REST connection could not be saved.'))
      return response.json() as Promise<ConnectionSummary>
    }).then((selection) => {
      setConnections((current) => [...current.filter((connection) => connection.name !== 'Salesforce'), selection])
      setPlan((current) => ({ ...current, components: current.components.map((component) => component.id === 'salesforce' ? { ...component, configured: true, verified: false, ready: false } : component) }))
      setNotice('Salesforce REST connection saved in the local Go service. Check availability or create a scratch org next.')
    }).catch((error: Error) => setNotice(error.message)).finally(() => setConnectionsLoading(false))
  }
  const checkSalesforceAvailability = () => {
    setConnectionsLoading(true)
    void fetch('/api/connections/salesforce/check', { method: 'POST' }).then(async (response) => {
      if (!response.ok) throw new Error(await responseError(response, 'Salesforce org is unavailable.'))
      return response.json()
    }).then(async () => {
      await refreshSalesforceConnection()
      setScratchJob({ id: 'availability-check', status: 'active', message: 'The selected Salesforce org is available and ready for validation.' })
      setNotice('Salesforce org is available and ready for validation.')
    }).catch((error: Error) => {
      setScratchJob({ id: 'availability-check', status: 'failed', message: error.message })
      setNotice(error.message)
    }).finally(() => setConnectionsLoading(false))
  }
  const createScratchOrg = (alias: string) => {
    setConnectionsLoading(true)
    setScratchJob({ id: 'starting', status: 'queued', message: 'Starting the scratch-org request…', alias })
    void fetch('/api/salesforce/scratch-orgs', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ alias, orgName: 'Box Dispatch', durationDays: 30 }) }).then(async (response) => {
      if (!response.ok) throw new Error(await responseError(response, 'Scratch org creation could not start.'))
      return response.json() as Promise<ScratchOrgJob>
    }).then((job) => {
      setScratchJob(job)
      const poll = async () => {
        const response = await fetch(`/api/salesforce/scratch-orgs/${job.id}`)
        if (!response.ok) throw new Error(await responseError(response, 'Scratch-org status is unavailable.'))
        const nextJob = await response.json() as ScratchOrgJob
        setScratchJob(nextJob)
        if (nextJob.status === 'active') {
          await refreshSalesforceConnection()
          setNotice(`Scratch org ${nextJob.alias ?? ''} is active and selected.`.trim())
          setConnectionsLoading(false)
          return
        }
        if (nextJob.status === 'failed') {
          setNotice(nextJob.message)
          setConnectionsLoading(false)
          return
        }
        window.setTimeout(() => { void poll().catch(handleScratchError) }, 1000)
      }
      const handleScratchError = (error: unknown) => {
        const message = error instanceof Error ? error.message : 'Scratch-org status is unavailable.'
        setScratchJob((current) => ({ id: current?.id ?? job.id, status: 'failed', message }))
        setNotice(message)
        setConnectionsLoading(false)
      }
      void poll().catch(handleScratchError)
    }).catch((error: Error) => {
      setScratchJob({ id: 'failed', status: 'failed', message: error.message })
      setNotice(error.message)
      setConnectionsLoading(false)
    })
  }
  const beginNewDeployment = () => { setScreen('workflow'); setRun(null); setRunEvents([]); setActivePhase('Choose'); setNotice('Choose a supported solution quickstart, then Dispatch will assemble its BCL package locally.') }
  const toggleSalesforce = () => setSelectedComponents((components) => components.includes('salesforce') ? ['box'] : ['box', 'salesforce'])
  const assemblePackage = () => {
    setAssembling(true); setNotice('Assembling the selected template locally. Dispatch is preparing the BCL package…')
    void fetch('/api/packages', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ templateId: selectedTemplateID, components: selectedComponents }) }).then(async (response) => {
      if (!response.ok) throw new Error((await response.json().catch(() => ({ error: '' })) as { error?: string }).error || 'Package could not be assembled.')
      return (await response.json()) as DeploymentPlan
    }).then((nextPlan) => { setPlan({ ...nextPlan, strategy: nextPlan.strategy ?? 'reuse' }); setActivePhase('Connect'); setNotice('Package assembled locally. Confirm the selected system connections next.') }).catch((error: Error) => setNotice(error.message)).finally(() => setAssembling(false))
  }
  const savePlan = (onSaved?: () => void) => {
    void fetch('/api/plan', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ templateId: plan.templateId, template: plan.template, repository: plan.repository, components: plan.components.map((component) => component.id), strategy: plan.strategy }) }).then(async (response) => {
      if (!response.ok) throw new Error('Plan could not be saved.')
      return (await response.json()) as DeploymentPlan
    }).then((savedPlan) => { setPlan(savedPlan); setNotice('Plan saved locally as BCL.'); onSaved?.() }).catch(() => setNotice('Plan could not be saved. Start the local Dispatch API and try again.'))
  }
  const toggleProvider = (provider: 'box' | 'salesforce', included: boolean) => {
    if (provider === 'box') return
    setPlan((current) => {
      const existing = current.components.find((component) => component.id === provider)
      if (included && !existing) return { ...current, components: [...current.components, { id: 'salesforce', name: 'Salesforce', configured: false, verified: false, ready: false }] }
      if (!included) return { ...current, components: current.components.filter((component) => component.id !== provider) }
      return current
    })
  }
  const setStrategy = (strategy: DeploymentPlan['strategy']) => setPlan((current) => ({ ...current, strategy }))
  const toggleDeploymentComponent = (provider: 'box' | 'salesforce', component: string, included: boolean) => setComponentSelections((current) => ({ ...current, [provider]: included ? [...new Set([...(current[provider] ?? []), component])] : (current[provider] ?? []).filter((item) => item !== component) }))
  const continueToReview = () => savePlan(() => setActivePhase('Review'))
  const setWorkflowPhase = (phase: Phase) => { setScreen('workflow'); setActivePhase(phase) }

  return <div className="app-shell"><Sidebar activeView={screen} onOverview={() => setScreen('overview')} onNewDeployment={beginNewDeployment}/><main id="workspace" className="workspace">{screen === 'overview' ? <OverviewPage plan={plan} connections={connections} deployments={deployments} run={run} onNewDeployment={beginNewDeployment} onContinue={() => setWorkflowPhase(activePhase)} onBoxConnection={openBoxConnection} onSalesforceConnection={openSalesforceConnection}/> : <><DeploymentHeader plan={plan} activePhase={activePhase} run={run} onPhaseChange={setWorkflowPhase}/>{activePhase === 'Choose' ? <ChoosePage templates={templates} selectedTemplateID={selectedTemplateID} selectedComponents={selectedComponents} assembling={assembling} notice={notice} onTemplateChange={setSelectedTemplateID} onToggleSalesforce={toggleSalesforce} onAssemble={assemblePackage}/> : activePhase === 'Connect' ? <ConnectPage plan={plan} connections={connections} notice={notice} onBoxConnection={openBoxConnection} onSalesforceConnection={openSalesforceConnection} onBack={() => setActivePhase('Choose')} onNext={() => setActivePhase('Configure')}/> : activePhase === 'Configure' ? <ConfigurePage plan={plan} connections={connections} notice={notice} componentSelections={componentSelections} onToggleProvider={toggleProvider} onToggleComponent={toggleDeploymentComponent} onStrategyChange={setStrategy} onBack={() => setActivePhase('Connect')} onNext={continueToReview}/> : activePhase === 'Deploy' ? <DeployPage plan={plan} run={run} events={runEvents} notice={notice} onApply={applyDeployment} onDiagnostics={openDiagnostics}/> : <ReviewPage plan={plan} notice={notice} onDeploy={beginValidation} onEditConnections={() => setActivePhase('Connect')} onBack={() => setActivePhase('Configure')}/>}</>}</main>{diagnosticRunID && <DiagnosticsDrawer diagnostic={diagnostic} onClose={() => setDiagnosticRunID(null)}/>} {connectionDrawerOpen && <SalesforceConnectionDrawer options={salesforceOptions} selectedAlias={selectedSalesforceAlias} loading={connectionsLoading} scratchJob={scratchJob} onChange={setSelectedSalesforceAlias} onSave={selectSalesforceConnection} onSaveREST={saveSalesforceREST} onCheck={checkSalesforceAvailability} onCreateScratch={createScratchOrg} onClose={() => setConnectionDrawerOpen(false)}/>} {boxConnectionDrawerOpen && <BoxConnectionDrawer connection={connections.find((connection) => connection.name === 'Box')} loading={boxConnectionLoading} error={boxConnectionError} onSave={saveBoxConnection} onVerify={verifyBoxConnection} onClose={closeBoxConnection}/>}</div>
}

async function fetchJSON<T>(path: string, signal: AbortSignal): Promise<T> {
  const response = await fetch(path, { signal })
  if (!response.ok) throw new Error(`${path} is unavailable.`)
  return (await response.json()) as T
}

async function responseError(response: Response, fallback: string): Promise<string> {
  const payload = await response.json().catch(() => null) as { error?: string } | null
  return payload?.error?.trim() || fallback
}

export default App
