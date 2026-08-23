export type Phase = 'Choose' | 'Connect' | 'Configure' | 'Review' | 'Deploy'

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
  templateId: string
  template: string
  repository: string
  strategy: 'reuse' | 'create_new'
  components: PlanComponent[]
}

export type BoxConnectionInput = { alias: string; clientId: string; clientSecret: string; subjectType: 'user' | 'enterprise'; subjectId: string }

export type DispatchRun = {
  id: string
  action: 'validate' | 'deploy'
  status: 'queued' | 'running' | 'completed' | 'failed'
  providers: ProviderSummary[]
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

export type SalesforceConnectionOption = {
  alias: string
  kind: string
  status: string
  expiresAt?: string
  selected: boolean
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
  status: 'queued' | 'creating' | 'active' | 'failed'
  message: string
  alias?: string
  username?: string
  orgId?: string
  expirationDate?: string
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
}

export type SolutionTemplate = {
  id: string
  name: string
  sector?: string
  description?: string
}
