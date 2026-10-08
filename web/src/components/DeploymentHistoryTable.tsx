import { useMemo, useState } from 'react'
import type { TableColumn, TableRow, TableSortDetail } from '@unofficialbox/box-open-elements/table'
import type { DeploymentSummary } from '../types'
import { deploymentOutcome, displayProvider, displayStrategy, formatDeploymentDate } from '../deploymentPresentation'
import { BoeTable } from './BoeTable'

const baseColumns: TableColumn[] = [
  { key: 'deployment', label: 'Deployment', sortable: true },
  { key: 'systems', label: 'Systems', sortable: true },
  { key: 'strategy', label: 'Strategy', sortable: true },
  { key: 'completed', label: 'Completed', sortable: true },
]

const resultColumn: TableColumn = { key: 'result', label: 'Result', sortable: true }

export function DeploymentHistoryTable({ deployments, caption, includeResult = false, emptyMessage = 'Completed deployments will appear here.' }: { deployments: DeploymentSummary[]; caption: string; includeResult?: boolean; emptyMessage?: string }) {
  const [sort, setSort] = useState<TableSortDetail | null>(null)
  const columns = useMemo(() => includeResult ? [...baseColumns.slice(0, 3), resultColumn, baseColumns[3]] : baseColumns, [includeResult])
  const rows = useMemo<TableRow[]>(() => {
    const sorted = sort ? [...deployments].sort((left, right) => compareDeployments(left, right, sort)) : deployments
    return sorted.map((deployment) => {
      const outcome = deploymentOutcome(deployment)
      const name = deployment.name || deployment.id
      return {
        id: deployment.id,
        cells: {
          deployment: { kind: 'link', text: name, href: `#history/${encodeURIComponent(deployment.id)}` },
          systems: deployment.providers.length ? deployment.providers.map((provider) => displayProvider(provider.name)).join(', ') : 'Not recorded',
          strategy: displayStrategy(deployment.strategy),
          result: { kind: 'badge', text: outcome.label, tone: outcome.tone === 'info' ? 'neutral' : outcome.tone },
          completed: formatDeploymentDate(deployment.completedAt),
        },
      }
    })
  }, [deployments, sort])

  return <BoeTable className="deployment-history-table" columns={columns} rows={rows} label={caption} emptyText={emptyMessage} sortKey={sort?.key} sortDirection={sort?.direction} onSort={setSort}/>
}

function compareDeployments(left: DeploymentSummary, right: DeploymentSummary, sort: TableSortDetail) {
  const value = (deployment: DeploymentSummary) => {
    if (sort.key === 'deployment') return deployment.name || deployment.id
    if (sort.key === 'systems') return deployment.providers.map((provider) => displayProvider(provider.name)).join(', ')
    if (sort.key === 'strategy') return displayStrategy(deployment.strategy)
    if (sort.key === 'result') return deploymentOutcome(deployment).label
    if (sort.key === 'completed') return new Date(deployment.completedAt).valueOf() || 0
    return ''
  }
  const leftValue = value(left)
  const rightValue = value(right)
  const comparison = typeof leftValue === 'number' && typeof rightValue === 'number' ? leftValue - rightValue : String(leftValue).localeCompare(String(rightValue))
  return sort.direction === 'ascending' ? comparison : -comparison
}
