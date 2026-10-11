import { useEffect, useRef, useState } from 'react'
import '@unofficialbox/box-open-elements/app-shell'
import { Alert } from '@unofficialbox/box-open-elements-react/alert'
import './App.css'
import { AppToast, type AppToastNotice } from './components/AppToast'
import { BoxConnectionDrawer, DiagnosticsDrawer, SalesforceConnectionDrawer } from './components/Drawers'
import { DeploymentHeader } from './components/DeploymentHeader'
import { DeploymentConfirmationDialog } from './components/DeploymentConfirmationDialog'
import { ValidationChangesDrawer } from './components/ValidationChangesDrawer'
import { Sidebar, type AppView } from './components/Sidebar'
import { ChoosePage } from './pages/ChoosePage'
import { ConnectPage } from './pages/ConnectPage'
import { ConfigurePage } from './pages/ConfigurePage'
import { DeployPage } from './pages/DeployPage'
import { OverviewPage } from './pages/OverviewPage'
import { HelpPage } from './pages/HelpPage'
import { HistoryPage } from './pages/HistoryPage'
import { ReviewPage } from './pages/ReviewPage'
import { SettingsPage } from './pages/SettingsPage'
import { SummaryPage } from './pages/SummaryPage'
import type { BoxOAuthJob, ConnectionSummary, DeploymentDefaults, DeploymentPlan, DeploymentSummary, DispatchRun, Phase, RunDiagnostic, RunEvent, SalesforceOAuthJob, ScratchOrgJob, SolutionTemplate, ValidationFileChange, ValidationChanges } from './types'
import { refreshProviderReadiness } from './connectionReadiness'
import { markConnectionsChecking, markPlanConnectionsChecking, refreshPlanReadiness, revalidateConnections } from './connectionRefresh'
import { watchLoginJob } from './loginPolling'
import { scratchOrgRequest } from './scratchOrg'
import { connectionsReadyForPlan, guardedWorkflowPhase, resumeWorkflowPhase } from './workflowResume'

const fallbackPlan: DeploymentPlan = { exists: false, name: '', templateId: 'clm', template: 'CLM deployment', repository: 'https://github.com/unofficialbox/box-bedrock-for-clm', strategy: 'reuse', components: [{ id: 'box', name: 'Box', configured: false, verified: false, ready: false }, { id: 'salesforce', name: 'Salesforce', configured: false, verified: false, ready: false }] }
const fallbackDeploymentDefaults: DeploymentDefaults = { templateId: 'clm', template: 'Contract Lifecycle Management', repository: 'https://github.com/unofficialbox/box-bedrock-for-clm', strategy: 'reuse', components: ['box', 'salesforce'] }
const fallbackTemplates: SolutionTemplate[] = [
  { id: 'clm', name: 'Contract Lifecycle Management', sector: 'Legal operations', description: 'Content-centric contract workflows with Box and intelligent agents.' },
  { id: 'lifesciences', name: 'Life Sciences', sector: 'Regulated content', description: 'Accelerate document-heavy life sciences processes and insight.' },
  { id: 'citizen-services', name: 'Citizen Services', sector: 'Public sector', description: 'Modernize constituent intake, case content, and service delivery.' },
  { id: 'new', name: 'Create a New Solution', sector: 'Starter', description: 'Begin with the Box Dispatch reference architecture and shape your own solution.' },
]

const viewFromHash = (): AppView => {
  const hash = window.location.hash.toLowerCase()
  if (hash === '#history' || hash.startsWith('#history/')) return 'history'
  if (hash === '#help') return 'help'
  if (hash === '#settings') return 'settings'
  if (hash === '#workspace') return 'workflow'
  return 'overview'
}

const historyDeploymentIDFromHash = () => {
  const prefix = '#history/'
  if (!window.location.hash.toLowerCase().startsWith(prefix)) return null
  try {
    return decodeURIComponent(window.location.hash.slice(prefix.length)) || null
  } catch {
    return null
  }
}

const workflowResumeNotice = (phase: Phase, run: DispatchRun | null) => {
  if (phase === 'Summary') return 'Completed deployment restored.'
  if (phase === 'Deploy' && run?.action === 'validate' && run.status === 'completed') return 'Validation results restored. Review changes or continue to deployment.'
  if (phase === 'Deploy') return 'Deployment activity restored.'
  if (phase === 'Review') return 'Authentication is current. Review the plan, then validate.'
  return 'Saved deployment loaded. Connect the selected systems to continue.'
}

function App() {
  const [screen, setScreen] = useState<AppView>(viewFromHash)
  const [activePhase, setActivePhase] = useState<Phase>('Review')
  const [plan, setPlan] = useState<DeploymentPlan>(fallbackPlan)
  const [deploymentDefaults, setDeploymentDefaults] = useState<DeploymentDefaults>(fallbackDeploymentDefaults)
  const [deploymentName, setDeploymentName] = useState('')
  const [templates, setTemplates] = useState<SolutionTemplate[]>(fallbackTemplates)
  const [selectedTemplateID, setSelectedTemplateID] = useState('clm')
  const [selectedComponents, setSelectedComponents] = useState<string[]>(['box', 'salesforce'])
  const [assembling, setAssembling] = useState(false)
  const [run, setRun] = useState<DispatchRun | null>(null)
  const [streamNotice, setStreamNotice] = useState('')
  const [runEvents, setRunEvents] = useState<RunEvent[]>([])
  const [diagnostic, setDiagnostic] = useState<RunDiagnostic | null>(null)
  const [diagnosticError, setDiagnosticError] = useState('')
  const [diagnosticLoading, setDiagnosticLoading] = useState(false)
  const [diagnosticRetry, setDiagnosticRetry] = useState(0)
  const [diagnosticRunID, setDiagnosticRunID] = useState<string | null>(null)
  const [connectionDrawerOpen, setConnectionDrawerOpen] = useState(false)
  const [boxConnectionDrawerOpen, setBoxConnectionDrawerOpen] = useState(false)
  const [salesforceConnectionError, setSalesforceConnectionError] = useState('')
  const [connections, setConnections] = useState<ConnectionSummary[]>([])
  const [deployments, setDeployments] = useState<DeploymentSummary[]>([])
  const [historyDeploymentID, setHistoryDeploymentID] = useState<string | null>(historyDeploymentIDFromHash)
  const [scratchJob, setScratchJob] = useState<ScratchOrgJob | null>(null)
  const [oauthJob, setOauthJob] = useState<SalesforceOAuthJob | null>(null)
  const [boxOauthJob, setBoxOauthJob] = useState<BoxOAuthJob | null>(null)
  const boxLoginAttempt = useRef<AbortController | null>(null)
  const salesforceLoginAttempt = useRef<AbortController | null>(null)
  useEffect(() => () => { boxLoginAttempt.current?.abort(); salesforceLoginAttempt.current?.abort() }, [])
  const [connectionsLoading, setConnectionsLoading] = useState(false)
  const [boxConnectionLoading, setBoxConnectionLoading] = useState(false)
  const [boxConnectionError, setBoxConnectionError] = useState('')
  const [checkingConnections, setCheckingConnections] = useState(false)
  const [connectionsRefreshing, setConnectionsRefreshing] = useState(false)
  const [dataErrors, setDataErrors] = useState<Record<string, string>>({})
  const [dataRetry, setDataRetry] = useState(0)
  const [connectionRetry, setConnectionRetry] = useState(0)
  const [connectionError, setConnectionError] = useState('')
  const [connectionsKnown, setConnectionsKnown] = useState(false)
  const [initialDataLoaded, setInitialDataLoaded] = useState(false)
  const workflowResumeRequested = useRef(viewFromHash() === 'workflow')
  const workflowNavigationVersion = useRef(0)
  const workflowDraftGeneration = useRef(0)
  const [deploymentConfirmationOpen, setDeploymentConfirmationOpen] = useState(false)
  const [deploymentStarting, setDeploymentStarting] = useState(false)
  const deploymentStartPending = useRef(false)
  const [changeReview, setChangeReview] = useState<{ url: string; stage: 'validation' | 'deployment' } | null>(null)
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
  const activeRunID = run?.id ?? null
  const packagePreparing = scratchJob?.status === 'preparing' && (scratchJob.packageStatus === 'checking' || scratchJob.packageStatus === 'installing')

  const navigateTo = (view: AppView) => {
    if (view === 'workflow') {
      workflowResumeRequested.current = false
      workflowNavigationVersion.current += 1
    }
    const hash = view === 'overview' ? '' : view === 'workflow' ? '#workspace' : `#${view}`
    const nextURL = `${window.location.pathname}${window.location.search}${hash}`
    window.history.pushState(null, '', nextURL)
    setScreen(view)
    if (view === 'history') setHistoryDeploymentID(null)
  }

  useEffect(() => {
    const syncView = () => {
      const nextScreen = viewFromHash()
      if (nextScreen === 'workflow') workflowResumeRequested.current = true
      setScreen(nextScreen)
      setHistoryDeploymentID(historyDeploymentIDFromHash())
    }
    window.addEventListener('hashchange', syncView)
    window.addEventListener('popstate', syncView)
    return () => {
      window.removeEventListener('hashchange', syncView)
      window.removeEventListener('popstate', syncView)
    }
  }, [])

  useEffect(() => {
    const controller = new AbortController()
    const draftGenerationAtStart = workflowDraftGeneration.current
    // oxlint-disable-next-line react/set-state-in-effect
    setInitialDataLoaded(false)
    if (viewFromHash() === 'workflow') workflowResumeRequested.current = true
    void Promise.allSettled([fetchJSON<DeploymentPlan>('/api/plan', controller.signal), fetchJSON<SolutionTemplate[]>('/api/templates', controller.signal), fetchJSON<DeploymentDefaults>('/api/defaults', controller.signal), fetchJSON<DeploymentSummary[]>('/api/deployments', controller.signal), fetchJSON<DispatchRun[]>('/api/runs', controller.signal), fetchJSON<ScratchOrgJob>('/api/salesforce/scratch-orgs/latest', controller.signal)]).then(([planResult, templatesResult, defaultsResult, deploymentsResult, runsResult, scratchResult]) => {
      if (controller.signal.aborted) return
      const failed: Record<string, string> = {}
      for (const [key, label, result] of [
        ['plan', 'Saved deployment plan', planResult], ['templates', 'Solution catalog', templatesResult],
        ['defaults', 'Workspace defaults', defaultsResult], ['deployments', 'Deployment history', deploymentsResult], ['runs', 'Run history', runsResult],
      ] as const) {
        if (result.status === 'rejected') failed[key] = `${label} could not be loaded.`
      }
      setDataErrors(failed)
      const ownsDraft = workflowDraftGeneration.current === draftGenerationAtStart
      if (templatesResult.status === 'fulfilled' && templatesResult.value.length > 0) {
        setTemplates(templatesResult.value)
        if (ownsDraft) setSelectedTemplateID((current) => templatesResult.value.some((template) => template.id === current) ? current : templatesResult.value[0].id)
      }
      if (defaultsResult.status === 'fulfilled') {
        setDeploymentDefaults(defaultsResult.value)
        if (ownsDraft) {
          setSelectedTemplateID(defaultsResult.value.templateId)
          setSelectedComponents(defaultsResult.value.components)
        }
      }
      if (ownsDraft && planResult.status === 'fulfilled' && planResult.value.exists) {
        setPlan(markPlanConnectionsChecking(planResult.value))
        setDeploymentName(planResult.value.name)
        const latestRun = runsResult.status === 'fulfilled' ? runsResult.value.find((candidate) => !candidate.deployment || candidate.deployment === planResult.value.name) ?? null : null
        const resumedPhase = resumeWorkflowPhase(planResult.value, [], latestRun)
        setRun(latestRun)
        setActivePhase(resumedPhase)
        setNotice(workflowResumeNotice(resumedPhase, latestRun))
      } else if (ownsDraft) {
        setActivePhase('Choose')
        setNotice('Choose a supported solution to start a deployment.')
      }
      if (deploymentsResult.status === 'fulfilled') setDeployments(deploymentsResult.value)
      if (scratchResult.status === 'fulfilled') setScratchJob(scratchResult.value)
      setInitialDataLoaded(true)
    })
    return () => controller.abort()
  }, [dataRetry])

  const connectionSurface = screen === 'overview' || screen === 'settings'
    ? screen
    : screen === 'workflow' && ['Connect', 'Configure', 'Summary'].includes(activePhase)
      ? `${screen}:${activePhase}`
      : ''

  useEffect(() => {
    if (!initialDataLoaded || !connectionSurface) return
    const controller = new AbortController()
    const shouldResumeWorkflow = screen === 'workflow' && workflowResumeRequested.current
    const navigationVersionAtRefreshStart = workflowNavigationVersion.current
    if (shouldResumeWorkflow) workflowResumeRequested.current = false
    // This effect is the route-entry boundary for a live external-system refresh.
    // oxlint-disable-next-line react/set-state-in-effect
    setConnectionsRefreshing(true)
    setConnectionError('')
    setConnections((current) => markConnectionsChecking(current))
    setPlan((current) => markPlanConnectionsChecking(current))
    void revalidateConnections(fetch, controller.signal).then((nextConnections) => {
      const refreshedPlan = refreshPlanReadiness(plan, nextConnections)
      if (controller.signal.aborted) return
      setConnectionsKnown(true)
      setConnections(nextConnections)
      setPlan((current) => refreshPlanReadiness(current, nextConnections))
      if (shouldResumeWorkflow && workflowNavigationVersion.current === navigationVersionAtRefreshStart) {
        const resumedPhase = resumeWorkflowPhase(refreshedPlan, nextConnections, run)
        setActivePhase(resumedPhase)
        setNotice(workflowResumeNotice(resumedPhase, run))
      }
    }).catch((error: unknown) => {
      if (controller.signal.aborted) return
      const message = error instanceof Error ? error.message : 'Connection readiness could not be refreshed.'
      setConnectionsKnown(true)
      setConnectionError('Saved connections could not be loaded. Retry when Dispatch’s local service is available.')
      setNotice(message)
      showToast(message, 'error')
    }).finally(() => {
      if (!controller.signal.aborted) setConnectionsRefreshing(false)
    })
    return () => controller.abort()
  // Re-run only when the rendered connection surface changes. `screen` and
  // `plan` and `run` are snapshots for a route-entry resume decision.
  // oxlint-disable-next-line react-hooks/exhaustive-deps
  }, [connectionSurface, initialDataLoaded, connectionRetry])

  useEffect(() => {
    if (!activeRunID) return
    const controller = new AbortController()
    let stream = new EventSource(`/api/runs/${activeRunID}/events`)
    let stopped = false
    let disconnected = false
    let fetching = false
    let terminalObserved = false
    let retryDelay = 1000
    let timer: number | undefined
    // oxlint-disable-next-line react/set-state-in-effect
    setStreamNotice('')
    const stopStream = () => {
      stopped = true
      window.clearTimeout(timer)
      stream.close()
    }
    const acceptRun = (next: DispatchRun) => {
      if (controller.signal.aborted || stopped) return
      if (terminalObserved && (next.status === 'queued' || next.status === 'running')) return
      setRun(next)
      if (next.status !== 'completed' && next.status !== 'failed') return
      setStreamNotice(terminalObserved ? '' : 'Run status is confirmed, but activity history is incomplete because live updates were interrupted. Displayed activity may reflect an earlier state.')
      if (next.status === 'completed' && next.action === 'deploy') setActivePhase('Summary')
      stopStream()
      if (next.status === 'completed') {
        void fetchJSON<DeploymentSummary[]>('/api/deployments', controller.signal)
          .then((records) => { if (!controller.signal.aborted) setDeployments(records) }).catch(() => undefined)
      }
    }
    const reconcile = async () => {
      if (stopped || fetching) return
      fetching = true
      try {
        const next = await fetchJSON<DispatchRun>(`/api/runs/${activeRunID}`, controller.signal)
        acceptRun(next)
        retryDelay = 1000
      } catch {
        if (!controller.signal.aborted && !stopped) setStreamNotice('Live updates are disconnected and run status is unavailable. Dispatch will keep retrying; the run may still be active.')
        retryDelay = Math.min(retryDelay * 2, 10_000)
      } finally {
        fetching = false
        if (disconnected && !stopped && !controller.signal.aborted) timer = window.setTimeout(() => {
          // HTTP failures can permanently close EventSource. Resume the stream
          // at the same bounded retry boundary as read-only status checks.
          if (stream.readyState === EventSource.CLOSED) {
            const onError = stream.onerror
            stream.close()
            stream = new EventSource(`/api/runs/${activeRunID}/events`)
            stream.addEventListener('dispatch', receiveEvent)
            stream.onerror = onError
          }
          void reconcile()
        }, retryDelay)
      }
    }
    const receiveEvent = (message: MessageEvent<string>) => {
      if (stopped || controller.signal.aborted) return
      try {
        const event = JSON.parse(message.data) as RunEvent
        setRunEvents((events) => events.some((existing) => existing.sequence === event.sequence) ? events : [...events, event])
        disconnected = false
        window.clearTimeout(timer)
        setStreamNotice('')
        if (event.type === 'status' && (event.status === 'completed' || event.status === 'failed')) {
          terminalObserved = true
          disconnected = true
          setRun((current) => current ? { ...current, status: event.status } : current)
          if (event.status === 'completed' && run?.action === 'deploy') setActivePhase('Summary')
          void reconcile()
        }
      } catch {
        disconnected = true
        setStreamNotice('Live updates could not be read. Checking the run status…')
        void reconcile()
      }
    }
    stream.onerror = () => {
      if (stopped || controller.signal.aborted) return
      const wasDisconnected = disconnected
      disconnected = true
      setStreamNotice('Live updates are disconnected. Dispatch is checking the run status and reconnecting…')
      if (!wasDisconnected) { window.clearTimeout(timer); void reconcile() }
    }
    stream.addEventListener('dispatch', receiveEvent)
    return () => { stopStream(); controller.abort() }
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
  const applyDeployment = async () => {
    if (!run || packagePreparing || deploymentStartPending.current) return
    deploymentStartPending.current = true
    setDeploymentStarting(true)
    setDeploymentConfirmationOpen(false)
    try {
      const response = await fetch(`/api/runs/${run.id}/deploy`, { method: 'POST' })
      if (!response.ok) throw new Error(await responseError(response, 'Deployment could not start. Try again.'))
      const nextRun = await response.json() as DispatchRun
      setRun(nextRun)
      setRunEvents([])
      setNotice('')
    } catch (error: unknown) {
      setNotice(error instanceof Error ? error.message : 'Deployment could not start. Try again.')
    } finally {
      deploymentStartPending.current = false
      setDeploymentStarting(false)
    }
  }
  const openDiagnostics = (runID: string) => {
    setDiagnostic(null)
    setDiagnosticError('')
    setDiagnosticLoading(true)
    setDiagnosticRunID(runID)
  }
  useEffect(() => {
    if (!diagnosticRunID) return
    const controller = new AbortController()
    // oxlint-disable-next-line react/set-state-in-effect
    setDiagnosticLoading(true)
    setDiagnosticError('')
    void fetchJSON<RunDiagnostic>(`/api/runs/${encodeURIComponent(diagnosticRunID)}/diagnostics`, controller.signal)
      .then((result) => { if (!controller.signal.aborted) setDiagnostic(result) })
      .catch(() => { if (!controller.signal.aborted) setDiagnosticError('Diagnostic guidance is unavailable. Check that Dispatch is running, then retry.') })
      .finally(() => { if (!controller.signal.aborted) setDiagnosticLoading(false) })
    return () => controller.abort()
  }, [diagnosticRunID, diagnosticRetry])
  const openChanges = (url: string, stage: 'validation' | 'deployment') => {
    setChangeReview({ url, stage })
    setValidationChanges([])
    setValidationChangesError('')
    setValidationChangesLoading(true)
    void fetch(url).then(async (response) => {
      if (!response.ok) throw new Error('Deployment changes are unavailable.')
      return (await response.json()) as ValidationChanges
    }).then((changes) => setValidationChanges(changes.files)).catch((error: unknown) => setValidationChangesError(error instanceof Error ? error.message : 'Deployment changes are unavailable.')).finally(() => setValidationChangesLoading(false))
  }
  const openValidationChanges = (runID: string) => openChanges(`/api/runs/${encodeURIComponent(runID)}/changes`, 'validation')
  const openCompletedRunChanges = (runID: string) => openChanges(`/api/runs/${encodeURIComponent(runID)}/changes`, 'deployment')
  const openDeploymentChanges = (deploymentID: string) => openChanges(`/api/deployments/${encodeURIComponent(deploymentID)}/changes`, 'deployment')
  const openSalesforceConnection = () => {
    setToast(null)
    setSalesforceConnectionError('')
    setOauthJob(null)
    setConnectionDrawerOpen(true)
  }
  const closeSalesforceConnection = () => {
    const attempt = salesforceLoginAttempt.current
    attempt?.abort()
    salesforceLoginAttempt.current = null
    if (attempt) setConnectionsLoading(false)
    setOauthJob(null)
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
    const attempt = boxLoginAttempt.current
    attempt?.abort()
    boxLoginAttempt.current = null
    if (attempt) setBoxConnectionLoading(false)
    setBoxOauthJob(null)
    setBoxConnectionError('')
    setBoxConnectionDrawerOpen(false)
  }
  const startBoxOAuth = async () => {
    boxLoginAttempt.current?.abort()
    const controller = new AbortController()
    boxLoginAttempt.current = controller
    setBoxConnectionError('')
    setBoxConnectionLoading(true)
    try {
      const response = await fetch('/api/connections/box/oauth/start', { method: 'POST', headers: { 'Content-Type': 'application/json' }, signal: controller.signal })
      if (!response.ok) throw new Error(await responseError(response, 'Box login could not start.'))
      const started = await response.json() as { id: string; authorizeUrl: string }
      if (controller.signal.aborted) return false
      const popup = window.open(started.authorizeUrl, 'dispatch-box-login', 'width=520,height=720')
      if (!popup) throw new Error('The browser blocked the Box login window. Allow popups for Dispatch, then try again.')
      setBoxOauthJob({ id: started.id, status: 'pending', message: 'Complete Box login in the browser window.' })
      void watchLoginJob<BoxOAuthJob>(`/api/connections/box/oauth/${started.id}`, popup, controller, async (job) => {
        if (boxLoginAttempt.current !== controller || controller.signal.aborted) return
        setBoxOauthJob(job)
        if (job.status === 'active') {
          await refreshBoxConnection(controller.signal)
          if (boxLoginAttempt.current !== controller || controller.signal.aborted) return
          const added = job.identity || job.alias || 'Box user'
          setNotice(`${added} is ready.`)
          showToast(`Connected ${added}.`)
        } else if (job.status === 'failed') {
          setBoxConnectionError(job.message)
        }
      }).catch((error: unknown) => {
        if (boxLoginAttempt.current !== controller || (controller.signal.aborted && controller.signal.reason?.name === 'AbortError')) return
        const message = error instanceof Error ? error.message : 'Box login status is unavailable.'
        setBoxConnectionError(message)
        setBoxOauthJob({ id: started.id, status: 'failed', message })
      }).finally(() => {
        if (boxLoginAttempt.current === controller) {
          boxLoginAttempt.current = null
          setBoxConnectionLoading(false)
        }
      })
      return true
    } catch (error: unknown) {
      if (boxLoginAttempt.current !== controller || controller.signal.aborted) return false
      boxLoginAttempt.current = null
      setBoxConnectionError(error instanceof Error ? error.message : 'Box login could not start.')
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
  const refreshBoxConnection = async (signal?: AbortSignal) => {
    const response = await fetch('/api/connections', { signal })
    if (!response.ok) throw new Error('Box connection state could not be refreshed.')
    const nextConnections = await response.json() as ConnectionSummary[]
    if (signal?.aborted) return
    setConnections(nextConnections)
    setPlan((current) => refreshProviderReadiness(current, nextConnections, 'box'))
  }
  const refreshSalesforceConnection = async (signal?: AbortSignal) => {
    const response = await fetch('/api/connections', { signal })
    if (!response.ok) throw new Error('Salesforce connection state could not be refreshed.')
    const nextConnections = await response.json() as ConnectionSummary[]
    if (signal?.aborted) return
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
    salesforceLoginAttempt.current?.abort()
    const controller = new AbortController()
    salesforceLoginAttempt.current = controller
    setSalesforceConnectionError('')
    setConnectionsLoading(true)
    try {
      const response = await fetch('/api/connections/salesforce/oauth/start', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ loginHost, role }), signal: controller.signal })
      if (!response.ok) throw new Error(await responseError(response, 'Salesforce login could not start.'))
      const started = await response.json() as { id: string; authorizeUrl: string }
      if (controller.signal.aborted) return false
      const popup = window.open(started.authorizeUrl, 'dispatch-salesforce-login', 'width=520,height=720')
      if (!popup) throw new Error('The browser blocked the Salesforce login window. Allow popups for Dispatch, then try again.')
      setOauthJob({ id: started.id, status: 'pending', message: 'Complete Salesforce login in the browser window.', role })
      void watchLoginJob<SalesforceOAuthJob>(`/api/connections/salesforce/oauth/${started.id}`, popup, controller, async (job) => {
        if (salesforceLoginAttempt.current !== controller || controller.signal.aborted) return
        setOauthJob(job)
        if (job.status === 'active') {
          await refreshSalesforceConnection(controller.signal)
          if (salesforceLoginAttempt.current !== controller || controller.signal.aborted) return
          const added = job.username || job.alias || (role === 'devhub' ? 'Salesforce Dev Hub' : 'Salesforce org')
          setNotice(role === 'devhub' ? 'Salesforce Dev Hub is ready.' : 'Salesforce org is ready.')
          showToast(`Connected ${added}.`)
        } else if (job.status === 'failed') {
          setSalesforceConnectionError(job.message)
        }
      }).catch((error: unknown) => {
        if (salesforceLoginAttempt.current !== controller || (controller.signal.aborted && controller.signal.reason?.name === 'AbortError')) return
        const message = error instanceof Error ? error.message : 'Salesforce login status is unavailable.'
        setSalesforceConnectionError(message)
        setOauthJob({ id: started.id, status: 'failed', message, role })
      }).finally(() => {
        if (salesforceLoginAttempt.current === controller) {
          salesforceLoginAttempt.current = null
          setConnectionsLoading(false)
        }
      })
      return true
    } catch (error: unknown) {
      if (salesforceLoginAttempt.current !== controller || controller.signal.aborted) return false
      salesforceLoginAttempt.current = null
      setSalesforceConnectionError(error instanceof Error ? error.message : 'Salesforce login could not start.')
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
  const beginNewDeployment = () => {
    if (deploymentStartPending.current) {
      navigateTo('workflow')
      setNotice('Deployment is starting. Wait for its status before starting another deployment.')
      return
    }
    workflowDraftGeneration.current += 1
    navigateTo('workflow')
    setRun(null)
    setRunEvents([])
    setDeploymentName('')
    setSelectedTemplateID(deploymentDefaults.templateId)
    setSelectedComponents(deploymentDefaults.components)
    setActivePhase('Choose')
    setNotice('Name the deployment, then confirm the solution and systems.')
  }
  const toggleSalesforce = () => setSelectedComponents((components) => components.includes('salesforce') ? ['box'] : ['box', 'salesforce'])
  const assemblePackage = () => {
    if (!deploymentName.trim()) { setNotice('Enter a deployment name to continue.'); return }
    setAssembling(true); setNotice('Preparing the selected package…')
    void fetch('/api/packages', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ name: deploymentName.trim(), templateId: selectedTemplateID, components: selectedComponents, strategy: deploymentDefaults.strategy }) }).then(async (response) => {
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
  const saveDeploymentDefaults = async (nextDefaults: Pick<DeploymentDefaults, 'templateId' | 'strategy' | 'components'>) => {
    const response = await fetch('/api/defaults', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(nextDefaults) })
    if (!response.ok) throw new Error(await responseError(response, 'Deployment defaults could not be saved.'))
    const saved = await response.json() as DeploymentDefaults
    setDeploymentDefaults(saved)
    setSelectedTemplateID(saved.templateId)
    setSelectedComponents(saved.components)
    showToast('Deployment defaults saved.')
    return saved
  }
  const checkSelectedConnections = async (currentPlan: DeploymentPlan) => {
    setConnections((current) => markConnectionsChecking(current))
    setPlan(markPlanConnectionsChecking(currentPlan))
    const nextConnections = await revalidateConnections()
    const checkedPlan = refreshPlanReadiness(currentPlan, nextConnections)
    setConnections(nextConnections)
    setPlan(checkedPlan)

    if (!connectionsReadyForPlan(checkedPlan, nextConnections)) {
      throw new Error('One or more selected connections could not be verified.')
    }
    return checkedPlan
  }
  const handleConnectionCheckFailure = (error: unknown) => {
    const message = error instanceof TypeError
      ? 'Dispatch’s local service is unavailable. Reopen Dispatch or wait for it to restart, then try again.'
      : error instanceof Error ? error.message : 'Live authentication could not be verified.'
    navigateTo('workflow')
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
      navigateTo('workflow')
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
    navigateTo('workflow')
    setActivePhase(nextPhase)
    if (nextPhase !== phase) {
      setNotice('Connect and verify every selected system before validation.')
      return
    }
    if (nextPhase === 'Choose') setNotice('Confirm the solution and systems.')
    if (nextPhase === 'Connect') setNotice(plan.components.every((component) => component.ready) ? 'All selected connections are verified.' : 'Connect the remaining systems to continue.')
    if (nextPhase === 'Configure') setNotice('Confirm the strategy and included providers.')
  }
  const continueSavedDeployment = () => setWorkflowPhase(resumeWorkflowPhase(plan, connections, run))

  const workflow = <><DeploymentHeader plan={plan} draftName={activePhase === 'Choose' ? deploymentName : undefined} activePhase={activePhase} run={run}/>{activePhase === 'Choose' ? <ChoosePage templates={templates} selectedTemplateID={selectedTemplateID} selectedComponents={selectedComponents} deploymentName={deploymentName} assembling={assembling} notice={notice} onTemplateChange={setSelectedTemplateID} onToggleSalesforce={toggleSalesforce} onDeploymentNameChange={setDeploymentName} onAssemble={assemblePackage}/> : activePhase === 'Connect' ? <ConnectPage plan={plan} connections={connections} refreshing={connectionsRefreshing} notice={notice} onBoxConnection={openBoxConnection} onSalesforceConnection={openSalesforceConnection} onOpenProvider={openProvider} onBack={() => setWorkflowPhase('Choose')} onNext={() => setWorkflowPhase('Configure')}/> : activePhase === 'Configure' ? <ConfigurePage plan={plan} connections={connections} notice={notice} refreshing={connectionsRefreshing} checkingConnections={checkingConnections} onToggleProvider={toggleProvider} onStrategyChange={setStrategy} onBack={() => setWorkflowPhase('Connect')} onNext={continueToReview}/> : activePhase === 'Deploy' ? <DeployPage retrying={checkingConnections} onRetryValidation={beginValidation} onEditConnections={() => setWorkflowPhase('Connect')} starting={deploymentStarting} plan={plan} run={run} events={runEvents} notice={notice} onApply={() => { if (!deploymentStartPending.current) setDeploymentConfirmationOpen(true) }} onDiagnostics={openDiagnostics} onViewChanges={openValidationChanges}/> : activePhase === 'Summary' && run ? <SummaryPage plan={plan} connections={connections} run={run} onOpenProvider={openProvider} onViewChanges={openCompletedRunChanges} onOverview={() => navigateTo('overview')}/> : <ReviewPage plan={plan} notice={notice} checkingConnections={checkingConnections} onDeploy={beginValidation} onEditConnections={() => setWorkflowPhase('Connect')} onBack={() => setWorkflowPhase('Configure')}/>}</>
  const content = screen === 'overview'
    ? <OverviewPage plan={plan} connections={connections} connectionsRefreshing={connectionsRefreshing} deployments={deployments} run={run} onNewDeployment={beginNewDeployment} onContinue={continueSavedDeployment} onBoxConnection={openBoxConnection} onSalesforceConnection={openSalesforceConnection} onOpenProvider={openProvider} onViewHistory={() => navigateTo('history')}/>
    : screen === 'history'
      ? <HistoryPage deployments={deployments} selectedDeploymentID={historyDeploymentID} onCloseDeployment={() => navigateTo('history')} onOpenDestination={openDestination} onViewChanges={openDeploymentChanges}/>
      : screen === 'settings'
        ? <SettingsPage defaults={deploymentDefaults} connections={connections} connectionsRefreshing={connectionsRefreshing} onSaveDefaults={saveDeploymentDefaults} onBoxConnection={openBoxConnection} onSalesforceConnection={openSalesforceConnection} onRemoveBoxConnection={removeBoxConnection} onRemoveSalesforceConnection={removeSalesforceOrg} boxConnectionsBusy={boxConnectionLoading} salesforceConnectionsBusy={connectionsLoading}/>
        : screen === 'help' ? <HelpPage/> : workflow

  const requiredData = screen === 'history' ? ['deployments']
    : screen === 'settings' ? ['defaults']
    : screen === 'workflow' ? ['plan', 'runs', 'templates', 'defaults']
    : screen === 'overview' ? ['plan', 'runs', 'deployments'] : []
  const needsConnections = screen === 'overview' || screen === 'settings' || (screen === 'workflow' && ['Connect', 'Configure', 'Summary'].includes(activePhase))
  const loadErrors = requiredData.map((key) => dataErrors[key]).filter(Boolean)
  if (needsConnections && connectionError) loadErrors.push(connectionError)
  const waitingForData = screen !== 'help' && (!initialDataLoaded || (needsConnections && !connectionsKnown))
  const visibleContent = loadErrors.length > 0
    ? <section className="task-surface" aria-label="Workspace data unavailable"><Alert heading="Workspace data unavailable" message={`${loadErrors.join(' ')} Existing records may still be present. Check that Dispatch is running, then retry.`} tone="error" open/><box-button label="Retry loading" tone="primary" onClick={() => { if (requiredData.some((key) => dataErrors[key])) setDataRetry((current) => current + 1); if (needsConnections && connectionError) setConnectionRetry((current) => current + 1) }}></box-button></section>
    : waitingForData ? <section className="task-surface" role="status">Loading workspace data…</section> : content

  return <box-app-shell className="app-shell" heading="Box Dispatch" nav-label="Application navigation"><Sidebar activeView={screen} onOverview={() => navigateTo('overview')} onNewDeployment={beginNewDeployment} onHistory={() => navigateTo('history')} onSettings={() => navigateTo('settings')}/><div id="workspace" className="workspace">{streamNotice && <p className="notice" role="status">{streamNotice}</p>}{visibleContent}</div>{diagnosticRunID && <DiagnosticsDrawer diagnostic={diagnostic} loading={diagnosticLoading} error={diagnosticError} onRetry={() => setDiagnosticRetry((current) => current + 1)} onClose={() => setDiagnosticRunID(null)}/>} {changeReview && <ValidationChangesDrawer files={validationChanges} loading={validationChangesLoading} error={validationChangesError} stage={changeReview.stage} onClose={() => setChangeReview(null)}/>} {connectionDrawerOpen && <SalesforceConnectionDrawer connection={connections.find((connection) => connection.name === 'Salesforce')} loading={connectionsLoading} error={salesforceConnectionError} oauthJob={oauthJob} scratchJob={scratchJob} onLogin={startSalesforceOAuth} onSelect={selectSalesforceOrg} onRemove={removeSalesforceOrg} onOpen={() => openProvider('salesforce')} onCreateScratch={createScratchOrg} onClose={closeSalesforceConnection}/>} {boxConnectionDrawerOpen && <BoxConnectionDrawer connection={connections.find((connection) => connection.name === 'Box')} loading={boxConnectionLoading} error={boxConnectionError} oauthJob={boxOauthJob} onLogin={startBoxOAuth} onSelect={selectBoxConnection} onRemove={removeBoxConnection} onOpen={() => openProvider('box')} onClose={closeBoxConnection}/>} {deploymentConfirmationOpen && <DeploymentConfirmationDialog plan={plan} packagePreparing={Boolean(packagePreparing)} packageMessage={scratchJob?.packageMessage} onCancel={() => setDeploymentConfirmationOpen(false)} onConfirm={applyDeployment}/>} {toastNotice}</box-app-shell>
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
