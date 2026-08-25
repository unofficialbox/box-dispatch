import { readinessLabel, type ConnectionSummary, type PlanComponent } from '../types'
import { DetailList, DetailsRail } from './DetailsRail'

export function ConnectionDetailsPanel({ component, connection, onEdit }: { component: PlanComponent; connection?: ConnectionSummary; onEdit: () => void }) {
  const rows: Array<[string, string]> = [
    ['Status', readinessLabel(component.ready)],
    ['Connection', connection?.authType || 'Not configured'],
    ['Selected', connection?.selection || '—'],
  ]
  if (connection?.expiresAt) rows.push(['Expires', connection.expiresAt])
  return <DetailsRail title={`${component.name} connection`}>
    <span className={`detail-provider-mark ${component.id}`}>{component.id === 'box' ? 'B' : 'SF'}</span>
    <DetailList rows={rows}/>
    <box-button label={`Edit ${component.name} connection`} tone="secondary" onClick={onEdit}></box-button>
  </DetailsRail>
}
