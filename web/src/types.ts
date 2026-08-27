export type Phase = 'Choose' | 'Connect' | 'Configure' | 'Review' | 'Deploy' | 'Summary'

export type ProviderSummary = { name: string; status: string }

export type DeploymentSummary = {
  id: string
  name: string
  strategy: string
  completedAt: string
  providers: ProviderSummary[]
}

export type PlanComponent = {
  id: string
  name: string
  configured: boolean
  verified: boolean
  ready: boolean
}

export type DeploymentPlan = {
  exists: boolean
  name: string
  templateId: string
  template: string
  repository: string
  strategy: 'reuse' | 'create_new'
  components: PlanComponent[]
}

export type DeploymentDefaults = {
  templateId: string
  template: string
  repository: string
  strategy: 'reuse' | 'create_new'
  components: string[]
}

export type Readiness = 'Ready' | 'Not ready'

export function readinessLabel(ready: boolean): Readiness {
  return ready ? 'Ready' : 'Not ready'
}

export type DispatchRun = {
  id: string
  deployment?: string
  changeCount?: number
  action: 'validate' | 'deploy'
  status: 'queued' | 'running' | 'completed' | 'failed'
  providers: ProviderSummary[]
  resources?: ResourceReference[]
}

export type ResourceReference = {
  provider: string
  component: string
  kind: string
  name: string
  id: string
  url?: string
}

export type RunDiagnostic = {
  title: string
  summary: string
  provider?: string
  code?: string
  nextSteps: string[]
  technicalDetail?: string
}

export type RunEvent = {
  sequence: number
  at: string
  type: 'status' | 'activity'
  provider?: string
  message: string
  status: DispatchRun['status']
  component?: string
  progressState?: 'activity' | 'queued' | 'running' | 'completed' | 'failed'
  current?: number
  total?: number
}

export type ValidationFileChange = {
  component: string
  path: string
  kind: 'add' | 'update'
  before?: string
  after?: string
  previewable: boolean
}

export type ValidationChanges = { files: ValidationFileChange[] }

export type SalesforceConnectionOption = {
  id?: string
  alias: string
  kind: string
  status: string
  expiresAt?: string
  selected: boolean
  devHub?: boolean
  username?: string
  orgId?: string
}

export type SalesforceRESTInput = {
  instanceUrl: string
  accessToken: string
  devHubUrl: string
  devHubAccessToken: string
  clientId: string
  clientSecret: string
}

export type ScratchOrgJob = {
  id: string
  status: 'queued' | 'creating' | 'preparing' | 'active' | 'failed'
  message: string
  alias?: string
  username?: string
  orgId?: string
  expirationDate?: string
  packageStatus?: 'checking' | 'installing' | 'complete' | 'failed' | 'skipped'
  packageMessage?: string
  packageRequestId?: string
}

export type BoxConnectionOption = {
  id?: string
  alias: string
  status: string
  selected: boolean
  identity?: string
  subjectType?: string
  clientIdHint?: string
  subjectIdHint?: string
}

export type BoxOAuthJob = {
  id: string
  status: 'pending' | 'active' | 'failed'
  message: string
  alias?: string
  identity?: string
  account?: string
}

export type ConnectionSummary = {
  name: string
  configured: boolean
  verified: boolean
  authType?: string
  selection?: string
  status?: string
  expiresAt?: string
  alias?: string
  subjectType?: string
  clientIdHint?: string
  subjectIdHint?: string
  restConfigured?: boolean
  devHubConfigured?: boolean
  oauthConfigured?: boolean
  launchUrl?: string
  orgs?: SalesforceConnectionOption[]
  connections?: BoxConnectionOption[]
}

export type SalesforceOAuthStart = {
  id: string
  authorizeUrl: string
  loginHost: string
  role: 'org' | 'devhub'
}

export type SalesforceOAuthJob = {
  id: string
  status: 'pending' | 'active' | 'failed'
  message: string
  alias?: string
  username?: string
  orgId?: string
  role?: 'org' | 'devhub'
}

export type SolutionTemplate = {
  id: string
  name: string
  sector?: string
  description?: string
}
