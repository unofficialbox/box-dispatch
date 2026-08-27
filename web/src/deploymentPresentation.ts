import type { DeploymentSummary } from './types'

export const displayStrategy = (strategy: string) => strategy === 'create_new' ? 'Create new' : 'Reuse existing'

export const displayProvider = (provider: string) => {
  const normalized = provider.trim().toLowerCase()
  if (normalized === 'box') return 'Box'
  if (normalized === 'salesforce') return 'Salesforce'
  return provider
}

const dateFormatter = new Intl.DateTimeFormat(undefined, { month: 'short', day: 'numeric', year: 'numeric', hour: 'numeric', minute: '2-digit' })

export const formatDeploymentDate = (value?: string) => {
  if (!value) return 'Not recorded'
  const date = new Date(value)
  if (Number.isNaN(date.valueOf())) return value
  return dateFormatter.format(date)
}

export const deploymentOutcome = (deployment: DeploymentSummary) => {
  const statuses = deployment.providers.map((provider) => provider.status.trim().toLowerCase())
  if (statuses.length > 0 && statuses.every((status) => ['completed', 'present', 'ready', 'succeeded', 'success'].includes(status))) return { label: 'Complete', tone: 'success' as const }
  if (statuses.length > 0) return { label: 'Needs attention', tone: 'error' as const }
  return { label: 'Recorded', tone: 'info' as const }
}
