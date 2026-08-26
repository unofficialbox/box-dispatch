import { useEffect, useState } from 'react'
import './App.css'
import { AppToast, type AppToastNotice } from './components/AppToast'
import { BoxConnectionDrawer, DiagnosticsDrawer, SalesforceConnectionDrawer } from './components/Drawers'
import { DeploymentHeader } from './components/DeploymentHeader'
import { DeploymentConfirmationDialog } from './components/DeploymentConfirmationDialog'
import { ValidationChangesDrawer } from './components/ValidationChangesDrawer'
import { Sidebar } from './components/Sidebar'
import { ChoosePage } from './pages/ChoosePage'
import { ConnectPage } from './pages/ConnectPage'
import { ConfigurePage } from './pages/ConfigurePage'
import { DeployPage } from './pages/DeployPage'
import { OverviewPage } from './pages/OverviewPage'
import { ReviewPage } from './pages/ReviewPage'
import { SummaryPage } from './pages/SummaryPage'
import type { BoxOAuthJob, ConnectionSummary, DeploymentPlan, DeploymentSummary, DispatchRun, Phase, RunDiagnostic, RunEvent, SalesforceOAuthJob, ScratchOrgJob, SolutionTemplate, ValidationFileChange, ValidationChanges } from './types'
import { refreshProviderReadiness } from './connectionReadiness'
import { scratchOrgRequest } from './scratchOrg'
import { connectionsReadyForPlan, guardedWorkflowPhase, resumeWorkflowPhase } from './workflowResume'

const fallbackPlan: DeploymentPlan = { exists: false, name: '', templateId: 'clm', template: 'CLM deployment', repository: 'https://github.com/unofficialbox/box-bedrock-for-clm', strategy: 'reuse', components: [{ id: 'box', name: 'Box', configured: true, verified: true, ready: true }, { id: 'salesforce', name: 'Salesforce', configured: true, verified: true, ready: true }] }
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
  const [deploymentName, setDeploymentName] = useState('')
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
  const [checkingConnections, setCheckingConnections] = useState(false)
  const [deploymentConfirmationOpen, setDeploymentConfirmationOpen] = useState(false)
  const [changesRunID, setChangesRunID] = useState<string | null>(null)
  const [validationChanges, setValidationChanges] = useState<ValidationFileChange[]>([])
  const [validationChangesLoading, setValidationChangesLoading] = useState(false)
  const [validationChangesError, setValidationChangesError] = useState('')

  useEffect(() => {
    window.scrollTo(0, 0)
  }, [activePhase, screen])
  const [notice, setNotice] = useState('Loading the saved deployment plan…')
  const [toast, setToast] = useState<AppToastNotice | null>(null)
  const showToast = (message: string, tone = 'success') => setToast({ id: Date.now(), message, tone })
  const toastNotice = toast ? <AppToast key={toast.id} notice={toast} onDismiss={() => setToast(null)} /> : null
  const activeRunID = run && (run.status === 'queued' || run.status === 'running') ? run.id : null
  const packagePreparing = scratchJob?.status === 'preparing' && (scratchJob.packageStatus === 'checking' || scratchJob.packageStatus === 'installing')

  useEffect(() => {
    const controller = new AbortController()
    void Promise.allSettled([fetchJSON<DeploymentPlan>('/api/plan', controller.signal), fetchJSON<SolutionTemplate[]>('/api/templates', controller.signal), fetchJSON<ConnectionSummary[]>('/api/connections', controller.signal), fetchJSON<DeploymentSummary[]>('/api/deployments', controller.signal), fetchJSON<DispatchRun[]>('/api/runs', controller.signal), fetchJSON<ScratchOrgJob>('/api/salesforce/scratch-orgs/latest', controller.signal)]).then(([planResult, templatesResult, connectionsResult, deploymentsResult, runsResult, scratchResult]) => {
      if (templatesResult.status === 'fulfilled' && templatesResult.value.length > 0) {
        setTemplates(templatesResult.value)
        setSelectedTemplateID((current) => templatesResult.value.some((template) => template.id === current) ? current : templatesResult.value[0].id)
      }
      if (planResult.status === 'fulfilled' && planResult.value.exists) {
        setPlan(planResult.value)
        setDeploymentName(planResult.value.name)
        const loadedConnections = connectionsResult.status === 'fulfilled' ? connectionsResult.value : []
        const latestRun = runsResult.status === 'fulfilled' ? runsResult.value.find((candidate) => !candidate.deployment || candidate.deployment === planResult.value.name) ?? null : null
        const resumedPhase = resumeWorkflowPhase(planResult.value, loadedConnections, latestRun)
        setRun(latestRun)
        setActivePhase(resumedPhase)
        setNotice(resumedPhase === 'Deploy' ? 'Deployment activity restored.' : resumedPhase === 'Summary' ? 'Completed deployment restored.' : resumedPhase === 'Review' ? 'Saved deployment loaded. Authentication will be checked first.' : 'Saved deployment loaded. Connect the selected systems to continue.')
      } else {
        setActivePhase('Choose')
        setNotice('Choose a supported solution to start a deployment.')
      }
      if (connectionsResult.status === 'fulfilled') setConnections(connectionsResult.value)
      if (deploymentsResult.status === 'fulfilled') setDeployments(deploymentsResult.value)
      if (scratchResult.status === 'fulfilled') setScratchJob(scratchResult.value)
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
        if (event.status === 'completed' && run?.action === 'deploy') setActivePhase('Summary')
        void fetch(`/api/runs/${activeRunID}`).then(async (response) => {
          if (!response.ok) throw new Error('Run details are unavailable.')
          return (await response.json()) as DispatchRun
        }).then(setRun).catch(() => undefined)
        if (event.status === 'completed') {
          void fetch('/api/deployments').then(async (response) => response.ok ? await response.json() as DeploymentSummary[] : []).then((records) => { if (records.length > 0) setDeployments(records) }).catch(() => undefined)
        }
        stream.close()
      }
    }
    stream.addEventListener('dispatch', receiveEvent)
    return () => stream.close()
  }, [activeRunID, run?.action])

  useEffect(() => {
    window.scrollTo(0, 0)
  }, [activePhase, screen])

  const beginValidation = async () => {
    if (checkingConnections) return
    setCheckingConnections(true)
    setNotice('Checking live authentication before validation…')
    let authenticationPassed = false
    try {
      await checkSelectedConnections(plan)
      authenticationPassed = true
      const response = await fetch('/api/runs', { method: 'POST' })
      if (!response.ok) throw new Error(await responseError(response, 'Validation could not start.'))
      const nextRun = await response.json() as DispatchRun
      setRun(nextRun)
      setRunEvents([])
      setActivePhase('Deploy')
      setNotice('')
    } catch (error: unknown) {
      if (!authenticationPassed) {
        handleConnectionCheckFailure(error)
      } else {
        setNotice(error instanceof Error ? error.message : 'Validation could not start. Assemble a package in Dispatch, then try again.')
      }
    } finally {
      setCheckingConnections(false)
    }
  }
  const applyDeployment = () => {
    if (!run || packagePreparing) return
    setDeploymentConfirmationOpen(false)
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
  const openValidationChanges = (runID: string) => {
    setChangesRunID(runID)
    setValidationChanges([])
    setValidationChangesError('')
    setValidationChangesLoading(true)
    void fetch(`/api/runs/${runID}/changes`).then(async (response) => {
      if (!response.ok) throw new Error('Validation changes are unavailable.')
      return (await response.json()) as ValidationChanges
    }).then((changes) => setValidationChanges(changes.files)).catch((error: unknown) => setValidationChangesError(error instanceof Error ? error.message : 'Validation changes are unavailable.')).finally(() => setValidationChangesLoading(false))
  }
  const openSalesforceConnection = () => {
    setToast(null)
    setSalesforceConnectionError('')
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
  const openDestination = (launchUrl: string, label: string) => {
    if (!launchUrl) {
      showToast(`${label} is not ready to open.`, 'error')
      return
    }
    let target: URL
    try {
      target = new URL(launchUrl, window.location.origin)
    } catch {
      showToast(`${label} did not provide a valid destination.`, 'error')
      return
    }
    const salesforceSessionLaunch = launchUrl.startsWith('/')
      && target.origin === window.location.origin
      && target.pathname === '/api/connections/salesforce/open'
    if (target.protocol !== 'https:' && !salesforceSessionLaunch) {
      showToast(`${label} did not provide a secure destination.`, 'error')
      return
    }
    const opened = window.open(target.toString(), '_blank')
    if (opened) opened.opener = null
    else showToast(`Allow popups for Dispatch to open ${label}.`, 'error')
  }
  const openProvider = (providerID: string) => {
    const connection = connections.find((candidate) => candidate.name.toLowerCase() === providerID.toLowerCase())
    if (!connection?.launchUrl) {
      showToast(`${connection?.name || 'Provider'} is not ready to open.`, 'error')
      return
    }
    openDestination(connection.launchUrl, connection.name)
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
  const createScratchOrg = (alias: string, installManagedPackage: boolean) => {
    setSalesforceConnectionError('')
    setConnectionsLoading(true)
    setScratchJob({ id: 'starting', status: 'queued', message: 'Starting the scratch-org request…', alias })
    void fetch('/api/salesforce/scratch-orgs', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(scratchOrgRequest(alias, plan.name || deploymentName, installManagedPackage)) }).then(async (response) => {
      if (!response.ok) throw new Error(await responseError(response, 'Scratch org creation could not start.'))
      return response.json() as Promise<ScratchOrgJob>
    }).then(setScratchJob).catch((error: Error) => {
      setScratchJob({ id: 'failed', status: 'failed', message: error.message })
      setSalesforceConnectionError(error.message)
      setNotice(error.message)
      setConnectionsLoading(false)
    })
  }

  const scratchJobID = scratchJob?.id
  const scratchJobStatus = scratchJob?.status
  useEffect(() => {
    if (!scratchJobID || scratchJobID === 'starting' || scratchJobID === 'failed' || scratchJobStatus === 'active' || scratchJobStatus === 'failed') return
    let stopped = false
    let connectionLoaded = scratchJobStatus === 'preparing'
    const poll = async () => {
      try {
        const response = await fetch(`/api/salesforce/scratch-orgs/${scratchJobID}`)
        if (!response.ok) throw new Error(await responseError(response, 'Scratch-org status is unavailable.'))
        const nextJob = await response.json() as ScratchOrgJob
        if (stopped) return
        setScratchJob(nextJob)
        if (nextJob.status === 'preparing' && !connectionLoaded) {
          connectionLoaded = true
          const connectionsResponse = await fetch('/api/connections')
          if (connectionsResponse.ok) setConnections(await connectionsResponse.json() as ConnectionSummary[])
          setConnectionsLoading(false)
          showToast(`Connected ${nextJob.username || nextJob.alias || 'scratch org'}. Salesforce setup is continuing in the background.`)
        }
        if (nextJob.status === 'active') {
          const connectionsResponse = await fetch('/api/connections')
          if (connectionsResponse.ok) setConnections(await connectionsResponse.json() as ConnectionSummary[])
          setConnectionsLoading(false)
          if (nextJob.packageStatus === 'failed') {
            setSalesforceConnectionError(nextJob.packageMessage || nextJob.message)
            showToast(nextJob.packageMessage || nextJob.message, 'error')
          } else {
            setNotice(`Scratch org ${nextJob.alias ?? ''} is ready.`.trim())
            showToast(nextJob.message)
          }
          return
        }
        if (nextJob.status === 'failed') {
          setSalesforceConnectionError(nextJob.message)
          setNotice(nextJob.message)
          setConnectionsLoading(false)
          return
        }
        window.setTimeout(() => { if (!stopped) void poll() }, 1000)
      } catch (error: unknown) {
        const message = error instanceof Error ? error.message : 'Scratch-org status is unavailable.'
        setScratchJob((current) => ({ id: current?.id ?? scratchJobID, status: 'failed', message }))
        setSalesforceConnectionError(message)
        setConnectionsLoading(false)
      }
    }
    const timer = window.setTimeout(() => { void poll() }, 250)
    return () => { stopped = true; window.clearTimeout(timer) }
  }, [scratchJobID, scratchJobStatus])
  const beginNewDeployment = () => { setScreen('workflow'); setRun(null); setRunEvents([]); setDeploymentName(''); setActivePhase('Choose'); setNotice('Name the deployment, then confirm the solution and systems.') }
  const toggleSalesforce = () => setSelectedComponents((components) => components.includes('salesforce') ? ['box'] : ['box', 'salesforce'])
  const assemblePackage = () => {
    if (!deploymentName.trim()) { setNotice('Enter a deployment name to continue.'); return }
    setAssembling(true); setNotice('Preparing the selected package…')
    void fetch('/api/packages', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ name: deploymentName.trim(), templateId: selectedTemplateID, components: selectedComponents }) }).then(async (response) => {
      if (!response.ok) throw new Error((await response.json().catch(() => ({ error: '' })) as { error?: string }).error || 'Package could not be assembled.')
      return (await response.json()) as DeploymentPlan
    }).then((nextPlan) => {
      const preparedPlan = { ...nextPlan, strategy: nextPlan.strategy ?? 'reuse' }
      setPlan(preparedPlan)
      setDeploymentName(preparedPlan.name)
      setActivePhase('Connect')
      setNotice(preparedPlan.components.every((component) => component.ready) ? 'All selected connections are verified.' : 'Package ready. Connect the remaining systems to continue.')
    }).catch((error: Error) => setNotice(error.message)).finally(() => setAssembling(false))
  }
  const persistPlan = async (currentPlan: DeploymentPlan) => {
    const response = await fetch('/api/plan', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ name: currentPlan.name, templateId: currentPlan.templateId, template: currentPlan.template, repository: currentPlan.repository, components: currentPlan.components.map((component) => component.id), strategy: currentPlan.strategy }) })
    if (!response.ok) throw new Error(await responseError(response, 'Plan could not be saved.'))
    return await response.json() as DeploymentPlan
  }
  const checkSelectedConnections = async (currentPlan: DeploymentPlan) => {
    let failedCheck: Error | null = null
    for (const component of currentPlan.components) {
      const provider = component.id === 'box' ? 'Box' : component.id === 'salesforce' ? 'Salesforce' : ''
      if (!provider) continue
      try {
        const response = await fetch(`/api/connections/${component.id}/check`, { method: 'POST' })
        if (!response.ok) throw new Error(await responseError(response, `${provider} authentication could not be verified.`))
      } catch (error: unknown) {
        failedCheck ??= error instanceof Error ? error : new Error(`${provider} authentication could not be verified.`)
      }
    }

    const response = await fetch('/api/connections')
    if (!response.ok) {
      if (failedCheck) throw failedCheck
      throw new Error('Connection readiness could not be refreshed.')
    }
    const nextConnections = await response.json() as ConnectionSummary[]
    const checkedPlan = currentPlan.components.reduce((nextPlan, component) => {
      if (component.id !== 'box' && component.id !== 'salesforce') return nextPlan
      return refreshProviderReadiness(nextPlan, nextConnections, component.id)
    }, currentPlan)
    setConnections(nextConnections)
    setPlan(checkedPlan)

    if (failedCheck) throw failedCheck
    if (!connectionsReadyForPlan(checkedPlan, nextConnections)) {
      throw new Error('One or more selected connections could not be verified.')
    }
    return checkedPlan
  }
  const handleConnectionCheckFailure = (error: unknown) => {
    const message = error instanceof TypeError
      ? 'Dispatch’s local service is unavailable. Reopen Dispatch or wait for it to restart, then try again.'
      : error instanceof Error ? error.message : 'Live authentication could not be verified.'
    setScreen('workflow')
    setActivePhase('Connect')
    setNotice(message)
    showToast(message, 'error')
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
  const continueToReview = async () => {
    if (checkingConnections) return
    setCheckingConnections(true)
    setNotice('Saving the plan and checking live authentication…')
    let planSaved = false
    try {
      const savedPlan = await persistPlan(plan)
      planSaved = true
      setPlan(savedPlan)
      await checkSelectedConnections(savedPlan)
      setScreen('workflow')
      setActivePhase('Review')
      setNotice('Authentication is current. Review the plan, then validate.')
    } catch (error: unknown) {
      if (planSaved) {
        handleConnectionCheckFailure(error)
      } else {
        setNotice(error instanceof Error ? error.message : 'Plan could not be saved. Make sure Dispatch is running and try again.')
      }
    } finally {
      setCheckingConnections(false)
    }
  }
  const setWorkflowPhase = (phase: Phase) => {
    if (phase === 'Review') {
      void continueToReview()
      return
    }
    const nextPhase = guardedWorkflowPhase(phase, plan, connections)
    setScreen('workflow')
    setActivePhase(nextPhase)
    if (nextPhase !== phase) {
      setNotice('Connect and verify every selected system before validation.')
      return
    }
    if (nextPhase === 'Choose') setNotice('Confirm the solution and systems.')
    if (nextPhase === 'Connect') setNotice(plan.components.every((component) => component.ready) ? 'All selected connections are verified.' : 'Connect the remaining systems to continue.')
    if (nextPhase === 'Configure') setNotice('Confirm the strategy and included components.')
  }
  const continueSavedDeployment = () => setWorkflowPhase(resumeWorkflowPhase(plan, connections, run))

  return <div className="app-shell"><Sidebar activeView={screen} onOverview={() => setScreen('overview')} onNewDeployment={beginNewDeployment}/><main id="workspace" className="workspace">{screen === 'overview' ? <OverviewPage plan={plan} connections={connections} deployments={deployments} run={run} onNewDeployment={beginNewDeployment} onContinue={continueSavedDeployment} onBoxConnection={openBoxConnection} onSalesforceConnection={openSalesforceConnection}/> : <><DeploymentHeader plan={plan} draftName={activePhase === 'Choose' ? deploymentName : undefined} activePhase={activePhase} run={run} onPhaseChange={setWorkflowPhase}/>{activePhase === 'Choose' ? <ChoosePage templates={templates} selectedTemplateID={selectedTemplateID} selectedComponents={selectedComponents} deploymentName={deploymentName} assembling={assembling} notice={notice} onTemplateChange={setSelectedTemplateID} onToggleSalesforce={toggleSalesforce} onDeploymentNameChange={setDeploymentName} onAssemble={assemblePackage}/> : activePhase === 'Connect' ? <ConnectPage plan={plan} connections={connections} notice={notice} onBoxConnection={openBoxConnection} onSalesforceConnection={openSalesforceConnection} onOpenProvider={openProvider} onBack={() => setWorkflowPhase('Choose')} onNext={() => setWorkflowPhase('Configure')}/> : activePhase === 'Configure' ? <ConfigurePage plan={plan} connections={connections} notice={notice} checkingConnections={checkingConnections} componentSelections={componentSelections} onToggleProvider={toggleProvider} onToggleComponent={toggleDeploymentComponent} onStrategyChange={setStrategy} onBack={() => setWorkflowPhase('Connect')} onNext={continueToReview}/> : activePhase === 'Deploy' ? <DeployPage plan={plan} run={run} events={runEvents} notice={notice} onApply={() => setDeploymentConfirmationOpen(true)} onDiagnostics={openDiagnostics} onViewChanges={openValidationChanges}/> : activePhase === 'Summary' && run ? <SummaryPage plan={plan} connections={connections} run={run} onOpenProvider={openProvider} onOverview={() => setScreen('overview')}/> : <ReviewPage plan={plan} notice={notice} checkingConnections={checkingConnections} onDeploy={beginValidation} onEditConnections={() => setWorkflowPhase('Connect')} onBack={() => setWorkflowPhase('Configure')}/>}</>}</main>{diagnosticRunID && <DiagnosticsDrawer diagnostic={diagnostic} onClose={() => setDiagnosticRunID(null)}/>} {changesRunID && <ValidationChangesDrawer files={validationChanges} loading={validationChangesLoading} error={validationChangesError} onClose={() => setChangesRunID(null)}/>} {connectionDrawerOpen && <SalesforceConnectionDrawer connection={connections.find((connection) => connection.name === 'Salesforce')} loading={connectionsLoading} error={salesforceConnectionError} oauthJob={oauthJob} scratchJob={scratchJob} onLogin={startSalesforceOAuth} onSelect={selectSalesforceOrg} onRemove={removeSalesforceOrg} onOpen={() => openProvider('salesforce')} onCreateScratch={createScratchOrg} onClose={closeSalesforceConnection}/>} {boxConnectionDrawerOpen && <BoxConnectionDrawer connection={connections.find((connection) => connection.name === 'Box')} loading={boxConnectionLoading} error={boxConnectionError} oauthJob={boxOauthJob} onLogin={startBoxOAuth} onSelect={selectBoxConnection} onRemove={removeBoxConnection} onVerify={verifyBoxConnection} onOpen={() => openProvider('box')} onClose={closeBoxConnection}/>} {deploymentConfirmationOpen && <DeploymentConfirmationDialog plan={plan} packagePreparing={Boolean(packagePreparing)} packageMessage={scratchJob?.packageMessage} onCancel={() => setDeploymentConfirmationOpen(false)} onConfirm={applyDeployment}/>} {toastNotice}</div>
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
