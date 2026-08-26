import { readinessLabel, type ConnectionSummary, type PlanComponent } from '../types'
import { DetailList, DetailsRail } from './DetailsRail'
import { ProviderLogo } from './ProviderLogo'

export function ConnectionDetailsPanel({ component, connection, onEdit }: { component: PlanComponent; connection?: ConnectionSummary; onEdit: () => void }) {
  const rows: Array<[string, string]> = [
    ['Status', readinessLabel(component.ready)],
    ['Connection', connection?.authType || 'Not configured'],
    ['Selected', connection?.selection || '—'],
  ]
  if (connection?.expiresAt) rows.push(['Expires', connection.expiresAt])
  return <DetailsRail title={`${component.name} connection`}>
    <ProviderLogo provider={component.id} size="standard"/>
    <DetailList rows={rows}/>
    <box-button label={`Edit ${component.name} connection`} tone="neutral" onClick={onEdit}></box-button>
  </DetailsRail>
}
