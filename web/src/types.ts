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

export type BoxConnectionInput = { clientId: string; clientSecret: string; subjectType: 'user' | 'enterprise'; subjectId: string }

export type DispatchRun = {
  id: string
  action: 'validate' | 'deploy'
  status: 'queued' | 'running' | 'completed' | 'failed'
  providers: ProviderSummary[]
}

export type RunDiagnostic = {
  title: string
  summary: string
  nextSteps: string[]
  cliHint: string
}

export type RunEvent = {
  sequence: number
  at: string
  type: 'status' | 'activity'
  provider?: string
  message: string
  status: DispatchRun['status']
}

export type SalesforceConnectionOption = {
  alias: string
  kind: string
  status: string
  expiresAt?: string
  selected: boolean
}

export type ConnectionSummary = {
  name: string
  configured: boolean
  verified: boolean
  authType?: string
  selection?: string
  status?: string
  expiresAt?: string
}

export type SolutionTemplate = {
  id: string
  name: string
  sector?: string
  description?: string
}
