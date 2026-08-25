import { useEffect, useState } from 'react'
import './App.css'
import { AppToast, type AppToastNotice } from './components/AppToast'
import { BoxConnectionDrawer, DiagnosticsDrawer, SalesforceConnectionDrawer } from './components/Drawers'
import { DeploymentHeader } from './components/DeploymentHeader'
import { Sidebar } from './components/Sidebar'
import { ChoosePage } from './pages/ChoosePage'
import { ConnectPage } from './pages/ConnectPage'
import { ConfigurePage } from './pages/ConfigurePage'
import { DeployPage } from './pages/DeployPage'
import { OverviewPage } from './pages/OverviewPage'
import { ReviewPage } from './pages/ReviewPage'
import type { BoxOAuthJob, ConnectionSummary, DeploymentPlan, DeploymentSummary, DispatchRun, Phase, RunDiagnostic, RunEvent, SalesforceOAuthJob, ScratchOrgJob, SolutionTemplate } from './types'
import { refreshProviderReadiness } from './connectionReadiness'
import { connectionsReadyForPlan, guardedWorkflowPhase, resumeWorkflowPhase } from './workflowResume'

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
  const [salesforceConnectionError, setSalesforceConnectionError] = useState('')
  const [connections, setConnections] = useState<ConnectionSummary[]>([])
  const [deployments, setDeployments] = useState<DeploymentSummary[]>([])
  const [scratchJob, setScratchJob] = useState<ScratchOrgJob | null>(null)
  const [oauthJob, setOauthJob] = useState<SalesforceOAuthJob | null>(null)
  const [boxOauthJob, setBoxOauthJob] = useState<BoxOAuthJob | null>(null)
  const [connectionsLoading, setConnectionsLoading] = useState(false)
  const [boxConnectionLoading, setBoxConnectionLoading] = useState(false)
  const [boxConnectionError, setBoxConnectionError] = useState('')
  const [notice, setNotice] = useState('Loading the saved deployment plan…')
  const [toast, setToast] = useState<AppToastNotice | null>(null)
  const showToast = (message: string, tone = 'success') => setToast({ id: Date.now(), message, tone })
  const toastNotice = toast ? <AppToast key={toast.id} notice={toast} onDismiss={() => setToast(null)} /> : null
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
        const loadedConnections = connectionsResult.status === 'fulfilled' ? connectionsResult.value : []
        setActivePhase(resumeWorkflowPhase(planResult.value, loadedConnections))
        setNotice(resumeWorkflowPhase(planResult.value, loadedConnections) === 'Review' ? 'Saved BCL plan loaded. Review its selected providers before deployment.' : 'Saved BCL plan loaded. Connect the selected systems before continuing.')
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

  useEffect(() => {
    window.scrollTo(0, 0)
  }, [activePhase, screen])

  const beginValidation = () => {
    if (!connectionsReadyForPlan(plan, connections)) {
      setWorkflowPhase('Connect')
      setNotice('Connect and verify every selected system before validation.')
      return
    }
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
    setToast(null)
    setSalesforceConnectionError('')
    setScratchJob(null)
    setOauthJob(null)
    setConnectionDrawerOpen(true)
  }
  const closeSalesforceConnection = () => {
    setSalesforceConnectionError('')
    setConnectionDrawerOpen(false)
  }
  const openBoxConnection = () => {
    setToast(null)
    setBoxConnectionError('')
    setBoxConnectionDrawerOpen(true)
  }
  const closeBoxConnection = () => {
    setBoxConnectionError('')
    setBoxConnectionDrawerOpen(false)
  }
  const startBoxOAuth = async () => {
    setBoxConnectionError('')
    setBoxConnectionLoading(true)
    try {
      const response = await fetch('/api/connections/box/oauth/start', { method: 'POST', headers: { 'Content-Type': 'application/json' } })
      if (!response.ok) throw new Error(await responseError(response, 'Box login could not start.'))
      const started = await response.json() as { id: string; authorizeUrl: string }
      const popup = window.open(started.authorizeUrl, 'dispatch-box-login', 'width=520,height=720')
      if (!popup) {
        setBoxConnectionError('The browser blocked the Box login window. Allow popups for Dispatch, then try again.')
        setBoxConnectionLoading(false)
        return false
      }
      setBoxOauthJob({ id: started.id, status: 'pending', message: 'Complete Box login in the browser window.' })
      const poll = async () => {
        const statusResponse = await fetch(`/api/connections/box/oauth/${started.id}`)
        if (!statusResponse.ok) throw new Error(await responseError(statusResponse, 'Box login status is unavailable.'))
        const job = await statusResponse.json() as BoxOAuthJob
        setBoxOauthJob(job)
        if (job.status === 'active') {
          await refreshBoxConnection()
          const added = job.identity || job.alias || 'Box user'
          setNotice(`${added} is ready.`)
          showToast(`Connected ${added}.`)
          setBoxConnectionLoading(false)
          return
        }
        if (job.status === 'failed') {
          setBoxConnectionError(job.message)
          setBoxConnectionLoading(false)
          return
        }
        window.setTimeout(() => { void poll().catch(handleBoxOAuthError) }, 1000)
      }
      const handleBoxOAuthError = (error: unknown) => {
        const message = error instanceof Error ? error.message : 'Box login status is unavailable.'
        setBoxConnectionError(message)
        setBoxOauthJob({ id: started.id, status: 'failed', message })
        setBoxConnectionLoading(false)
      }
      void poll().catch(handleBoxOAuthError)
      return true
    } catch (error: unknown) {
      const message = error instanceof Error ? error.message : 'Box login could not start.'
      setBoxConnectionError(message)
      setBoxConnectionLoading(false)
      return false
    }
  }
  const selectBoxConnection = async (id: string) => {
    setBoxConnectionError('')
    setBoxConnectionLoading(true)
    try {
      const response = await fetch('/api/connections/box/selection', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ id }) })
      if (!response.ok) throw new Error(await responseError(response, 'Box connection could not be selected.'))
      const selection = await response.json() as ConnectionSummary
      setConnections((current) => [...current.filter((connection) => connection.name !== 'Box'), selection])
      setPlan((current) => ({ ...current, components: current.components.map((component) => component.id === 'box' ? { ...component, configured: true, verified: selection.verified, ready: selection.verified } : component) }))
      setNotice(`${selection.alias || 'Box'} selected.`)
      return true
    } catch (error: unknown) {
      const message = error instanceof Error ? error.message : 'Box connection could not be selected.'
      setBoxConnectionError(message)
      return false
    } finally {
      setBoxConnectionLoading(false)
    }
  }
  const removeBoxConnection = async (id: string) => {
    setBoxConnectionError('')
    setBoxConnectionLoading(true)
    try {
      const response = await fetch(`/api/connections/box/${encodeURIComponent(id)}`, { method: 'DELETE' })
      if (!response.ok) throw new Error(await responseError(response, 'Box connection could not be removed.'))
      const removed = connections.find((connection) => connection.name === 'Box')?.connections?.find((app) => app.id === id)
      const selection = await response.json() as ConnectionSummary
      setConnections((current) => [...current.filter((connection) => connection.name !== 'Box'), selection])
      setPlan((current) => ({ ...current, components: current.components.map((component) => component.id === 'box' ? { ...component, configured: selection.configured, verified: selection.verified, ready: selection.verified } : component) }))
      const removedName = removed?.identity || removed?.alias || 'Box connection'
      setNotice(selection.configured ? `${selection.alias || 'Box'} is still connected.` : 'Box connection removed.')
      showToast(`Removed ${removedName}.`)
      return true
    } catch (error: unknown) {
      const message = error instanceof Error ? error.message : 'Box connection could not be removed.'
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
      setNotice(`${selection.alias || 'Box'} is ready.`)
      showToast(`${selection.alias || 'Box'} is ready.`)
      return true
    } catch (error: unknown) {
      const message = error instanceof TypeError
        ? 'Dispatch’s local service is unavailable. Reopen Dispatch or wait for it to restart, then try again.'
        : error instanceof Error ? error.message : 'Box connection could not be verified.'
      showToast(message, 'error')
      return false
    } finally {
      setBoxConnectionLoading(false)
    }
  }
  const refreshBoxConnection = async () => {
    const response = await fetch('/api/connections')
    if (!response.ok) throw new Error('Box connection state could not be refreshed.')
    const nextConnections = await response.json() as ConnectionSummary[]
    setConnections(nextConnections)
    setPlan((current) => refreshProviderReadiness(current, nextConnections, 'box'))
  }
  const refreshSalesforceConnection = async () => {
    const response = await fetch('/api/connections')
    if (!response.ok) throw new Error('Salesforce connection state could not be refreshed.')
    const nextConnections = await response.json() as ConnectionSummary[]
    setConnections(nextConnections)
    setPlan((current) => refreshProviderReadiness(current, nextConnections, 'salesforce'))
  }
  const selectSalesforceOrg = async (id: string) => {
    setSalesforceConnectionError('')
    setConnectionsLoading(true)
    try {
      const response = await fetch('/api/connections/salesforce', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ id }) })
      if (!response.ok) throw new Error(await responseError(response, 'Salesforce org could not be selected.'))
      await refreshSalesforceConnection()
      setNotice('Salesforce org selected.')
      return true
    } catch (error: unknown) {
      const message = error instanceof Error ? error.message : 'Salesforce org could not be selected.'
      setSalesforceConnectionError(message)
      setNotice(message)
      return false
    } finally {
      setConnectionsLoading(false)
    }
  }
  const removeSalesforceOrg = async (id: string) => {
    setSalesforceConnectionError('')
    setConnectionsLoading(true)
    try {
      const response = await fetch(`/api/connections/salesforce/${encodeURIComponent(id)}`, { method: 'DELETE' })
      if (!response.ok) throw new Error(await responseError(response, 'Salesforce org could not be removed.'))
      const removed = connections.find((connection) => connection.name === 'Salesforce')?.orgs?.find((org) => org.id === id)
      await refreshSalesforceConnection()
      const removedName = removed?.username || removed?.alias || 'Salesforce org'
      setNotice('Salesforce org removed.')
      showToast(`Removed ${removedName}.`)
      return true
    } catch (error: unknown) {
      const message = error instanceof Error ? error.message : 'Salesforce org could not be removed.'
      setSalesforceConnectionError(message)
      setNotice(message)
      return false
    } finally {
      setConnectionsLoading(false)
    }
  }
  const startSalesforceOAuth = async (loginHost: 'production' | 'sandbox', role: 'org' | 'devhub') => {
    setSalesforceConnectionError('')
    setConnectionsLoading(true)
    try {
      const response = await fetch('/api/connections/salesforce/oauth/start', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ loginHost, role }) })
      if (!response.ok) throw new Error(await responseError(response, 'Salesforce login could not start.'))
      const started = await response.json() as { id: string; authorizeUrl: string }
      const popup = window.open(started.authorizeUrl, 'dispatch-salesforce-login', 'width=520,height=720')
      if (!popup) {
        setSalesforceConnectionError('The browser blocked the Salesforce login window. Allow popups for Dispatch, then try again.')
        setConnectionsLoading(false)
        return false
      }
      setOauthJob({ id: started.id, status: 'pending', message: 'Complete Salesforce login in the browser window.', role })
      const poll = async () => {
        const statusResponse = await fetch(`/api/connections/salesforce/oauth/${started.id}`)
        if (!statusResponse.ok) throw new Error(await responseError(statusResponse, 'Salesforce login status is unavailable.'))
        const job = await statusResponse.json() as SalesforceOAuthJob
        setOauthJob(job)
        if (job.status === 'active') {
          await refreshSalesforceConnection()
          const added = job.username || job.alias || (role === 'devhub' ? 'Salesforce Dev Hub' : 'Salesforce org')
          setNotice(role === 'devhub' ? 'Salesforce Dev Hub is ready.' : 'Salesforce org is ready.')
          showToast(`Connected ${added}.`)
          setConnectionsLoading(false)
          return
        }
        if (job.status === 'failed') {
          setSalesforceConnectionError(job.message)
          setConnectionsLoading(false)
          return
        }
        window.setTimeout(() => { void poll().catch(handleOAuthError) }, 1000)
      }
      const handleOAuthError = (error: unknown) => {
        const message = error instanceof Error ? error.message : 'Salesforce login status is unavailable.'
        setSalesforceConnectionError(message)
        setOauthJob({ id: started.id, status: 'failed', message, role })
        setConnectionsLoading(false)
      }
      void poll().catch(handleOAuthError)
      return true
    } catch (error: unknown) {
      const message = error instanceof Error ? error.message : 'Salesforce login could not start.'
      setSalesforceConnectionError(message)
      setNotice(message)
      setConnectionsLoading(false)
      return false
    }
  }
  const checkSalesforceAvailability = async () => {
    setSalesforceConnectionError('')
    setConnectionsLoading(true)
    try {
      const response = await fetch('/api/connections/salesforce/check', { method: 'POST' })
      if (!response.ok) throw new Error(await responseError(response, 'Salesforce org is unavailable.'))
      await refreshSalesforceConnection()
      setNotice('Salesforce org is ready.')
      showToast('The selected Salesforce org is ready.')
      return true
    } catch (error: unknown) {
      const message = error instanceof Error ? error.message : 'Salesforce org is unavailable.'
      setNotice(message)
      showToast(message, 'error')
      return false
    } finally {
      setConnectionsLoading(false)
    }
  }
  const createScratchOrg = (alias: string) => {
    setSalesforceConnectionError('')
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
          const added = nextJob.username || nextJob.alias || 'scratch org'
          setNotice(`Scratch org ${nextJob.alias ?? ''} is ready.`.trim())
          showToast(`Connected ${added}.`)
          setConnectionsLoading(false)
          return
        }
        if (nextJob.status === 'failed') {
          setSalesforceConnectionError(nextJob.message)
          setNotice(nextJob.message)
          setConnectionsLoading(false)
          return
        }
        window.setTimeout(() => { void poll().catch(handleScratchError) }, 1000)
      }
      const handleScratchError = (error: unknown) => {
        const message = error instanceof Error ? error.message : 'Scratch-org status is unavailable.'
        setScratchJob((current) => ({ id: current?.id ?? job.id, status: 'failed', message }))
        setSalesforceConnectionError(message)
        setNotice(message)
        setConnectionsLoading(false)
      }
      void poll().catch(handleScratchError)
    }).catch((error: Error) => {
      setScratchJob({ id: 'failed', status: 'failed', message: error.message })
      setSalesforceConnectionError(error.message)
      setNotice(error.message)
      setConnectionsLoading(false)
    })
  }
  const beginNewDeployment = () => { setScreen('workflow'); setRun(null); setRunEvents([]); setActivePhase('Choose'); setNotice('Select a quickstart to prepare its deployment package.') }
  const toggleSalesforce = () => setSelectedComponents((components) => components.includes('salesforce') ? ['box'] : ['box', 'salesforce'])
  const assemblePackage = () => {
    setAssembling(true); setNotice('Preparing the selected package…')
    void fetch('/api/packages', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ templateId: selectedTemplateID, components: selectedComponents }) }).then(async (response) => {
      if (!response.ok) throw new Error((await response.json().catch(() => ({ error: '' })) as { error?: string }).error || 'Package could not be assembled.')
      return (await response.json()) as DeploymentPlan
    }).then((nextPlan) => { setPlan({ ...nextPlan, strategy: nextPlan.strategy ?? 'reuse' }); setActivePhase('Connect'); setNotice('Package ready. Confirm both connections to continue.') }).catch((error: Error) => setNotice(error.message)).finally(() => setAssembling(false))
  }
  const savePlan = (onSaved?: (savedPlan: DeploymentPlan) => void) => {
    void fetch('/api/plan', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ templateId: plan.templateId, template: plan.template, repository: plan.repository, components: plan.components.map((component) => component.id), strategy: plan.strategy }) }).then(async (response) => {
      if (!response.ok) throw new Error('Plan could not be saved.')
      return (await response.json()) as DeploymentPlan
    }).then((savedPlan) => { setPlan(savedPlan); setNotice('Plan saved locally as BCL.'); onSaved?.(savedPlan) }).catch(() => setNotice('Plan could not be saved. Start the local Dispatch API and try again.'))
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
  const continueToReview = () => savePlan((savedPlan) => setWorkflowPhase(guardedWorkflowPhase('Review', savedPlan, connections)))
  const setWorkflowPhase = (phase: Phase) => {
    const nextPhase = guardedWorkflowPhase(phase, plan, connections)
    setScreen('workflow')
    setActivePhase(nextPhase)
    if (nextPhase !== phase) setNotice('Connect and verify every selected system before validation.')
  }
  const continueSavedDeployment = () => setWorkflowPhase(resumeWorkflowPhase(plan, connections))

  return <div className="app-shell"><Sidebar activeView={screen} onOverview={() => setScreen('overview')} onNewDeployment={beginNewDeployment}/><main id="workspace" className="workspace">{screen === 'overview' ? <OverviewPage plan={plan} connections={connections} deployments={deployments} run={run} onNewDeployment={beginNewDeployment} onContinue={continueSavedDeployment} onBoxConnection={openBoxConnection} onSalesforceConnection={openSalesforceConnection}/> : <><DeploymentHeader plan={plan} activePhase={activePhase} run={run} onPhaseChange={setWorkflowPhase}/>{activePhase === 'Choose' ? <ChoosePage templates={templates} selectedTemplateID={selectedTemplateID} selectedComponents={selectedComponents} assembling={assembling} notice={notice} onTemplateChange={setSelectedTemplateID} onToggleSalesforce={toggleSalesforce} onAssemble={assemblePackage}/> : activePhase === 'Connect' ? <ConnectPage plan={plan} connections={connections} notice={notice} onBoxConnection={openBoxConnection} onSalesforceConnection={openSalesforceConnection} onBack={() => setActivePhase('Choose')} onNext={() => setActivePhase('Configure')}/> : activePhase === 'Configure' ? <ConfigurePage plan={plan} connections={connections} notice={notice} componentSelections={componentSelections} onToggleProvider={toggleProvider} onToggleComponent={toggleDeploymentComponent} onStrategyChange={setStrategy} onBack={() => setActivePhase('Connect')} onNext={continueToReview}/> : activePhase === 'Deploy' ? <DeployPage plan={plan} run={run} events={runEvents} notice={notice} onApply={applyDeployment} onDiagnostics={openDiagnostics}/> : <ReviewPage plan={plan} notice={notice} onDeploy={beginValidation} onEditConnections={() => setActivePhase('Connect')} onBack={() => setActivePhase('Configure')}/>}</>}</main>{diagnosticRunID && <DiagnosticsDrawer diagnostic={diagnostic} onClose={() => setDiagnosticRunID(null)}/>} {connectionDrawerOpen && <SalesforceConnectionDrawer connection={connections.find((connection) => connection.name === 'Salesforce')} loading={connectionsLoading} error={salesforceConnectionError} oauthJob={oauthJob} scratchJob={scratchJob} onLogin={startSalesforceOAuth} onSelect={selectSalesforceOrg} onRemove={removeSalesforceOrg} onCheck={checkSalesforceAvailability} onCreateScratch={createScratchOrg} onClose={closeSalesforceConnection}/>} {boxConnectionDrawerOpen && <BoxConnectionDrawer connection={connections.find((connection) => connection.name === 'Box')} loading={boxConnectionLoading} error={boxConnectionError} oauthJob={boxOauthJob} onLogin={startBoxOAuth} onSelect={selectBoxConnection} onRemove={removeBoxConnection} onVerify={verifyBoxConnection} onClose={closeBoxConnection}/>}{toastNotice}</div>
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
