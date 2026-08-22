import type { ConnectionSummary, PlanComponent } from '../types'
import { DetailList, DetailsRail } from './DetailsRail'

export function ConnectionDetailsPanel({ component, connection, onEdit }: { component: PlanComponent; connection?: ConnectionSummary; onEdit: () => void }) {
  const status = component.ready ? 'Verified' : component.configured ? 'Ready to verify' : 'Not connected'
  const rows: Array<[string, string]> = [
    ['Status', status],
    ['Connection', connection?.authType || (component.id === 'box' ? 'Not configured' : 'No org selected')],
    ['Selected', connection?.selection || '—'],
  ]
  if (connection?.status) rows.push(['Provider status', connection.status])
  if (connection?.expiresAt) rows.push(['Expires', connection.expiresAt])
  return <DetailsRail title={`${component.name} connection`} description={component.id === 'box' ? 'The Box credential selected for this deployment.' : 'The Salesforce org selected for this deployment.'}>
    <span className={`detail-provider-mark ${component.id}`}>{component.id === 'box' ? 'B' : 'SF'}</span>
    <DetailList rows={rows}/>
    <box-button label={`Edit ${component.name} connection`} tone="secondary" onClick={onEdit}></box-button>
  </DetailsRail>
}
