import { useEffect, useMemo, useState } from 'react'
import { Alert } from '@unofficialbox/box-open-elements-react/alert'
import { Select } from '@unofficialbox/box-open-elements-react/select'
import '@unofficialbox/box-open-elements/metric-card'
import '@unofficialbox/box-open-elements/section'
import type { TableColumn, TableRow } from '@unofficialbox/box-open-elements/table'
import { BoeTable } from '../components/BoeTable'
import type { BoxComparison, BoxConnectionOption, BoxMetadataTemplate, BoxMetadataTemplateDiff, ConnectionSummary } from '../types'

const statusPresentation = {
  changed: { label: 'Changed', tone: 'brand' },
  left_only: { label: 'Source only', tone: 'warning' },
  right_only: { label: 'Destination only', tone: 'brand' },
  equal: { label: 'Equal', tone: 'neutral' },
} as const

const columns: TableColumn[] = [
  { key: 'template', label: 'Metadata template' },
  { key: 'status', label: 'Result' },
]

const formatCaptureTime = (value: string) => value ? new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'medium' }).format(new Date(value)) : 'Not captured'

const comparisonText = (template?: BoxMetadataTemplate) => template ? JSON.stringify(template, null, 2) : ''

const selectableConnections = (connections: ConnectionSummary[]) => connections.find((connection) => connection.name === 'Box')?.connections?.filter((connection): connection is BoxConnectionOption & { id: string } => Boolean(connection.id)) ?? []

async function responseMessage(response: Response) {
  const payload = await response.json().catch(() => null) as { error?: string } | null
  return payload?.error?.trim() || 'The live Box comparison could not be completed.'
}

export function ComparePage({ connections }: { connections: ConnectionSummary[] }) {
  const boxConnections = useMemo(() => selectableConnections(connections), [connections])
  const [leftConnectionID, setLeftConnectionID] = useState('')
  const [rightConnectionID, setRightConnectionID] = useState('')
  const [comparison, setComparison] = useState<BoxComparison | null>(null)
  const [selectedTemplateKey, setSelectedTemplateKey] = useState('')
  const [running, setRunning] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    void import('@unofficialbox/box-open-elements/diff-viewer')
  }, [])

  useEffect(() => {
    const ids = boxConnections.map((connection) => connection.id)
    // Keep the operator's direction when possible while repairing selections
    // after connection data refreshes on route entry.
    // oxlint-disable-next-line react/set-state-in-effect
    setLeftConnectionID((current) => ids.includes(current) ? current : ids[0] ?? '')
    // oxlint-disable-next-line react/set-state-in-effect
    setRightConnectionID((current) => ids.includes(current) && current !== (ids[0] ?? '') ? current : ids.find((id) => id !== (ids[0] ?? '')) ?? '')
  }, [boxConnections])

  const runComparison = async () => {
    if (!leftConnectionID || !rightConnectionID || leftConnectionID === rightConnectionID) {
      setError('Select two different saved Box connections.')
      return
    }
    setRunning(true)
    setError('')
    try {
      const response = await fetch('/api/box/comparisons', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ leftConnectionId: leftConnectionID, rightConnectionId: rightConnectionID }),
      })
      if (!response.ok) throw new Error(await responseMessage(response))
      const next = await response.json() as BoxComparison
      setComparison(next)
      setSelectedTemplateKey(next.templates.find((template) => template.status !== 'equal')?.key ?? next.templates[0]?.key ?? '')
    } catch (caught) {
      setComparison(null)
      setSelectedTemplateKey('')
      setError(caught instanceof Error ? caught.message : 'The live Box comparison could not be completed.')
    } finally {
      setRunning(false)
    }
  }

  const selected = comparison?.templates.find((template) => template.key === selectedTemplateKey) ?? comparison?.templates[0]
  const rows = useMemo<TableRow[]>(() => comparison?.templates.map((template) => {
    const presentation = statusPresentation[template.status]
    return {
      id: template.key,
      cells: {
        template: template.displayName || template.key,
        status: { kind: 'badge', text: presentation.label, tone: presentation.tone },
      },
      detail: `${template.key} · ${template.status === 'changed' ? `${template.changes.length} semantic changes` : presentation.label}`,
    }
  }) ?? [], [comparison])
  const options = boxConnections.map((connection) => ({ label: `${connection.alias || connection.identity || 'Box connection'}${connection.enterpriseId ? ` · ${connection.enterpriseId}` : ''}`, value: connection.id }))
  const canCompare = boxConnections.length >= 2 && leftConnectionID !== rightConnectionID && !running

  return <section className="record-page compare-page" aria-labelledby="compare-title">
    <header className="record-page-heading"><div><p className="overview-eyebrow">Live environment inspection</p><h1 id="compare-title">Compare Box environments</h1><p>Query two Box enterprises now, normalize their metadata templates, and review the semantic differences before planning a deployment.</p></div></header>
    <section className="compare-controls" aria-labelledby="compare-settings-title">
      <header><div><h2 id="compare-settings-title">Comparison direction</h2><p>Source is shown on the left and destination on the right. This does not change the Box connection selected for deployments.</p></div><box-badge label="Live API" tone="success"></box-badge></header>
      {boxConnections.length < 2 ? <Alert heading="Two Box connections required" message="Add another saved Box connection in Settings before running a live enterprise comparison." tone="warning" open/> : <div className="compare-control-grid">
        <Select label="Source Box connection" value={leftConnectionID} options={options} onValueChanged={(event) => setLeftConnectionID(event.detail.value)}/>
        <Select label="Destination Box connection" value={rightConnectionID} options={options} onValueChanged={(event) => setRightConnectionID(event.detail.value)}/>
        <box-button label={running ? 'Comparing live environments…' : 'Run live comparison'} tone="primary" disabled={!canCompare} onClick={() => void runComparison()}></box-button>
      </div>}
      {error ? <Alert className="compare-error" heading="Comparison could not be completed" message={error} tone="error" open/> : null}
    </section>

    {comparison ? <>
      <section className="compare-environments" aria-label="Live comparison identities">
        <EnvironmentSummary label="Source" environment={comparison.left}/>
        <div className="compare-direction" aria-hidden="true">→</div>
        <EnvironmentSummary label="Destination" environment={comparison.right}/>
      </section>
      <section className="record-page-metrics compare-metrics" aria-label="Comparison summary">
        <box-metric-card heading="Changed" value={String(comparison.summary.changed)} message="Different on both sides"></box-metric-card>
        <box-metric-card heading="Source only" value={String(comparison.summary.leftOnly)} message="Missing from destination"></box-metric-card>
        <box-metric-card heading="Destination only" value={String(comparison.summary.rightOnly)} message={`${comparison.summary.equal} equal`}></box-metric-card>
      </section>
      <section className="record-page-section compare-results" aria-labelledby="comparison-results-title"><header><div><h2 id="comparison-results-title">Metadata templates</h2><p>Matched by stable template key. Provider IDs, enterprise scope IDs, and option IDs are excluded from the comparison.</p></div><span>{comparison.templates.length} inspected</span></header>
        <div className="compare-browser">
          <section className="compare-template-list" aria-label="Metadata template comparison results">
            <BoeTable columns={columns} rows={rows} label="Metadata template comparison results" emptyText="No enterprise metadata templates were returned." selectionMode="single" selectedIds={selected ? [selected.key] : []} onSelectionChange={(ids) => setSelectedTemplateKey(ids[0] ?? '')}/>
          </section>
          <section className="compare-template-detail" aria-live="polite">
            {selected ? <TemplateDiff template={selected} comparison={comparison}/> : <div className="compare-empty"><h3>No templates returned</h3><p>Both live enterprises returned an empty metadata-template inventory.</p></div>}
          </section>
        </div>
      </section>
    </> : <section className="compare-empty-state"><h2>Ready for a live comparison</h2><p>Dispatch will authenticate both saved connections in parallel and read the current enterprise metadata-template schemas. Results are not loaded from the saved readiness snapshot.</p></section>}
  </section>
}

function EnvironmentSummary({ label, environment }: { label: string; environment: BoxComparison['left'] }) {
  return <article><span>{label}</span><strong>{environment.alias}</strong><p>{[environment.domain, environment.identity].filter(Boolean).join(' · ')}</p><small>Enterprise {environment.enterpriseId} · captured {formatCaptureTime(environment.capturedAt)}</small></article>
}

function TemplateDiff({ template, comparison }: { template: BoxMetadataTemplateDiff; comparison: BoxComparison }) {
  const presentation = statusPresentation[template.status]
  return <><header className="compare-detail-heading"><div><span>Metadata template</span><h3>{template.displayName || template.key}</h3><code>{template.key}</code></div><box-badge label={presentation.label} tone={presentation.tone}></box-badge></header>
    {template.changes.length > 0 ? <ul className="compare-change-list" aria-label="Semantic changes">{template.changes.map((change) => <li key={change.path}><code>{change.path}</code></li>)}</ul> : null}
    <div className="compare-diff-scroll"><box-diff-viewer heading={`${template.key}.json`} before-label={comparison.left.alias} after-label={comparison.right.alias} before-text={comparisonText(template.left)} after-text={comparisonText(template.right)} mode="split"></box-diff-viewer></div>
  </>
}
