import { useEffect, useMemo, useState } from 'react'
import { Alert } from '@unofficialbox/box-open-elements-react/alert'
import { Select } from '@unofficialbox/box-open-elements-react/select'
import '@unofficialbox/box-open-elements/metric-card'
import '@unofficialbox/box-open-elements/section'
import type { TableColumn, TableRow } from '@unofficialbox/box-open-elements/table'
import { BoeTable } from '../components/BoeTable'
import type { BoxComparison, BoxConnectionOption, BoxFolder, BoxFolderComparison, BoxFolderDiff, BoxMetadataTemplate, BoxMetadataTemplateDiff, ConnectionSummary } from '../types'

type ComparisonType = 'metadata_templates' | 'folders'
type ComparisonResult = { type: 'metadata_templates'; value: BoxComparison } | { type: 'folders'; value: BoxFolderComparison }

const statusPresentation = {
  changed: { label: 'Changed', tone: 'brand' },
  left_only: { label: 'Source only', tone: 'warning' },
  right_only: { label: 'Destination only', tone: 'brand' },
  equal: { label: 'Equal', tone: 'neutral' },
} as const

const metadataColumns: TableColumn[] = [
  { key: 'template', label: 'Metadata template' },
  { key: 'status', label: 'Result' },
]

const folderColumns: TableColumn[] = [
  { key: 'path', label: 'Folder path' },
  { key: 'status', label: 'Result' },
]

const comparisonTypeOptions = [
  { label: 'Metadata templates', value: 'metadata_templates' },
  { label: 'Folders (connected user)', value: 'folders' },
]

const formatCaptureTime = (value: string) => value ? new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'medium' }).format(new Date(value)) : 'Not captured'
const comparisonText = (value?: BoxMetadataTemplate | BoxFolder) => value ? JSON.stringify(value, null, 2) : ''
const selectableConnections = (connections: ConnectionSummary[]) => connections.find((connection) => connection.name === 'Box')?.connections?.filter((connection): connection is BoxConnectionOption & { id: string } => Boolean(connection.id)) ?? []

async function responseMessage(response: Response) {
  const payload = await response.json().catch(() => null) as { error?: string } | null
  return payload?.error?.trim() || 'The live Box comparison could not be completed.'
}

export function ComparePage({ connections }: { connections: ConnectionSummary[] }) {
  const boxConnections = useMemo(() => selectableConnections(connections), [connections])
  const [comparisonType, setComparisonType] = useState<ComparisonType>('metadata_templates')
  const [leftConnectionID, setLeftConnectionID] = useState('')
  const [rightConnectionID, setRightConnectionID] = useState('')
  const [result, setResult] = useState<ComparisonResult | null>(null)
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

  const changeComparisonType = (value: string) => {
    const next = value === 'folders' ? 'folders' : 'metadata_templates'
    setComparisonType(next)
    setResult(null)
    setError('')
  }

  const changeConnection = (side: 'left' | 'right', value: string) => {
    if (side === 'left') setLeftConnectionID(value)
    else setRightConnectionID(value)
    setResult(null)
    setError('')
  }

  const runComparison = async () => {
    if (!leftConnectionID || !rightConnectionID || leftConnectionID === rightConnectionID) {
      setError('Select two different saved Box connections.')
      return
    }
    setRunning(true)
    setError('')
    try {
      const endpoint = comparisonType === 'folders' ? '/api/box/comparisons/folders' : '/api/box/comparisons'
      const response = await fetch(endpoint, {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ leftConnectionId: leftConnectionID, rightConnectionId: rightConnectionID }),
      })
      if (!response.ok) throw new Error(await responseMessage(response))
      if (comparisonType === 'folders') {
        setResult({ type: 'folders', value: await response.json() as BoxFolderComparison })
      } else {
        setResult({ type: 'metadata_templates', value: await response.json() as BoxComparison })
      }
    } catch (caught) {
      setResult(null)
      setError(caught instanceof Error ? caught.message : 'The live Box comparison could not be completed.')
    } finally {
      setRunning(false)
    }
  }

  const options = boxConnections.map((connection) => ({ label: `${connection.alias || connection.identity || 'Box connection'}${connection.enterpriseId ? ` · ${connection.enterpriseId}` : ''}`, value: connection.id }))
  const canCompare = boxConnections.length >= 2 && leftConnectionID !== rightConnectionID && !running
  const comparison = result?.value

  return <section className="record-page compare-page" aria-labelledby="compare-title">
    <header className="record-page-heading"><div><p className="overview-eyebrow">Live environment inspection</p><h1 id="compare-title">Compare Box environments</h1><p>Query two Box enterprises now, normalize comparable objects, and review semantic differences before planning a deployment.</p></div></header>
    <section className="compare-controls" aria-labelledby="compare-settings-title">
      <header><div><h2 id="compare-settings-title">Comparison direction</h2><p>Source is shown on the left and destination on the right. This does not change the Box connection selected for deployments.</p></div><box-badge label="Live API" tone="success"></box-badge></header>
      {boxConnections.length < 2 ? <Alert heading="Two Box connections required" message="Add another saved Box connection in Settings before running a live enterprise comparison." tone="warning" open/> : <div className="compare-control-grid">
        <Select label="Box object type" value={comparisonType} options={comparisonTypeOptions} disabled={running} onValueChanged={(event) => changeComparisonType(event.detail.value)}/>
        <Select label="Source Box connection" value={leftConnectionID} options={options} disabled={running} onValueChanged={(event) => changeConnection('left', event.detail.value)}/>
        <Select label="Destination Box connection" value={rightConnectionID} options={options} disabled={running} onValueChanged={(event) => changeConnection('right', event.detail.value)}/>
        <box-button label={running ? 'Comparing live environments…' : 'Run live comparison'} tone="primary" disabled={!canCompare} onClick={() => void runComparison()}></box-button>
      </div>}
      {comparisonType === 'folders' ? <p className="compare-scope-note">Folder comparison reads each connected user’s All Files hierarchy to 4 levels and at most 500 folders per side. It counts direct child items but does not read file contents.</p> : null}
      {error ? <Alert className="compare-error" heading="Comparison could not be completed" message={error} tone="error" open/> : null}
    </section>

    {comparison ? <>
      <section className="compare-environments" aria-label="Live comparison identities">
        <EnvironmentSummary label="Source" environment={comparison.left}/>
        <div className="compare-direction" aria-hidden="true">→</div>
        <EnvironmentSummary label="Destination" environment={comparison.right}/>
      </section>
      <SummaryMetrics summary={comparison.summary}/>
      {result.type === 'folders' ? <FolderResults key={result.value.completedAt} comparison={result.value}/> : <MetadataResults key={result.value.completedAt} comparison={result.value}/>}
    </> : <section className="compare-empty-state"><h2>Ready for a live comparison</h2><p>{comparisonType === 'folders' ? 'Dispatch will read the connected users’ current folder hierarchies from Box. Folder IDs and timestamps are excluded; relative paths provide the cross-enterprise comparison key.' : 'Dispatch will authenticate both saved connections in parallel and read the current enterprise metadata-template schemas. Results are not loaded from the saved readiness snapshot.'}</p></section>}
  </section>
}

function EnvironmentSummary({ label, environment }: { label: string; environment: BoxComparison['left'] }) {
  return <article><span>{label}</span><strong>{environment.alias}</strong><p>{[environment.domain, environment.identity].filter(Boolean).join(' · ')}</p><small>Enterprise {environment.enterpriseId} · captured {formatCaptureTime(environment.capturedAt)}</small></article>
}

function SummaryMetrics({ summary }: { summary: BoxComparison['summary'] }) {
  return <section className="record-page-metrics compare-metrics" aria-label="Comparison summary">
    <box-metric-card heading="Changed" value={String(summary.changed)} message="Different on both sides"></box-metric-card>
    <box-metric-card heading="Source only" value={String(summary.leftOnly)} message="Missing from destination"></box-metric-card>
    <box-metric-card heading="Destination only" value={String(summary.rightOnly)} message={`${summary.equal} equal`}></box-metric-card>
  </section>
}

function MetadataResults({ comparison }: { comparison: BoxComparison }) {
  const [selectedKey, setSelectedKey] = useState(comparison.templates.find((template) => template.status !== 'equal')?.key ?? comparison.templates[0]?.key ?? '')
  const selected = comparison.templates.find((template) => template.key === selectedKey) ?? comparison.templates[0]
  const rows = useMemo<TableRow[]>(() => comparison.templates.map((template) => {
    const presentation = statusPresentation[template.status]
    return { id: template.key, cells: { template: template.displayName || template.key, status: { kind: 'badge', text: presentation.label, tone: presentation.tone } }, detail: `${template.key} · ${template.status === 'changed' ? `${template.changes.length} semantic changes` : presentation.label}` }
  }), [comparison.templates])

  return <section className="record-page-section compare-results" aria-labelledby="comparison-results-title"><header><div><h2 id="comparison-results-title">Metadata templates</h2><p>Matched by stable template key. Provider IDs, enterprise scope IDs, and option IDs are excluded from the comparison.</p></div><span>{comparison.templates.length} inspected</span></header>
    <div className="compare-browser">
      <section className="compare-template-list" aria-label="Metadata template comparison results"><BoeTable columns={metadataColumns} rows={rows} label="Metadata template comparison results" emptyText="No enterprise metadata templates were returned." selectionMode="single" selectedIds={selected ? [selected.key] : []} onSelectionChange={(ids) => setSelectedKey(ids[0] ?? '')}/></section>
      <section className="compare-template-detail" aria-live="polite">{selected ? <MetadataDiff template={selected} comparison={comparison}/> : <ComparisonEmpty title="No templates returned" message="Both live enterprises returned an empty metadata-template inventory."/>}</section>
    </div>
  </section>
}

function FolderResults({ comparison }: { comparison: BoxFolderComparison }) {
  const [selectedPath, setSelectedPath] = useState(comparison.folders.find((folder) => folder.status !== 'equal')?.path ?? comparison.folders[0]?.path ?? '')
  const selected = comparison.folders.find((folder) => folder.path === selectedPath) ?? comparison.folders[0]
  const rows = useMemo<TableRow[]>(() => comparison.folders.map((folder) => {
    const presentation = statusPresentation[folder.status]
    return { id: folder.path, cells: { path: folder.path, status: { kind: 'badge', text: presentation.label, tone: presentation.tone } }, detail: folder.status === 'changed' ? `${folder.changes.length} direct-item count changes` : presentation.label }
  }), [comparison.folders])
  const truncated = comparison.scope.leftTruncated || comparison.scope.rightTruncated

  return <section className="record-page-section compare-results" aria-labelledby="comparison-results-title"><header><div><h2 id="comparison-results-title">Folders</h2><p>Matched by relative path beneath each connected user’s {comparison.scope.rootLabel} root. Folder IDs and timestamps are excluded.</p></div><span>{comparison.folders.length} inspected</span></header>
    {truncated ? <Alert className="compare-scope-alert" heading="Folder scope limit reached" message={`Results stop at depth ${comparison.scope.maxDepth} and ${comparison.scope.maxFolders} folders per side. Paths outside that boundary are not represented.`} tone="warning" open/> : null}
    <div className="compare-browser">
      <section className="compare-template-list" aria-label="Folder comparison results"><BoeTable columns={folderColumns} rows={rows} label="Folder comparison results" emptyText="Neither connected user returned folders within the comparison scope." selectionMode="single" selectedIds={selected ? [selected.path] : []} onSelectionChange={(ids) => setSelectedPath(ids[0] ?? '')}/></section>
      <section className="compare-template-detail" aria-live="polite">{selected ? <FolderDiffView folder={selected} comparison={comparison}/> : <ComparisonEmpty title="No folders returned" message="Neither connected user returned folders within the comparison scope."/>}</section>
    </div>
  </section>
}

function MetadataDiff({ template, comparison }: { template: BoxMetadataTemplateDiff; comparison: BoxComparison }) {
  const presentation = statusPresentation[template.status]
  return <><DiffHeading eyebrow="Metadata template" title={template.displayName || template.key} code={template.key} label={presentation.label} tone={presentation.tone}/><ChangeList changes={template.changes}/><DiffViewer heading={`${template.key}.json`} left={template.left} right={template.right} comparison={comparison}/></>
}

function FolderDiffView({ folder, comparison }: { folder: BoxFolderDiff; comparison: BoxFolderComparison }) {
  const presentation = statusPresentation[folder.status]
  return <><DiffHeading eyebrow="Folder" title={folder.name} code={folder.path} label={presentation.label} tone={presentation.tone}/><ChangeList changes={folder.changes}/><DiffViewer heading="folder-structure.json" left={folder.left} right={folder.right} comparison={comparison}/></>
}

function DiffHeading({ eyebrow, title, code, label, tone }: { eyebrow: string; title: string; code: string; label: string; tone: 'brand' | 'warning' | 'neutral' }) {
  return <header className="compare-detail-heading"><div><span>{eyebrow}</span><h3>{title}</h3><code>{code}</code></div><box-badge label={label} tone={tone}></box-badge></header>
}

function ChangeList({ changes }: { changes: BoxMetadataTemplateDiff['changes'] }) {
  return changes.length > 0 ? <ul className="compare-change-list" aria-label="Semantic changes">{changes.map((change) => <li key={change.path}><code>{change.path}</code></li>)}</ul> : null
}

function DiffViewer({ heading, left, right, comparison }: { heading: string; left?: BoxMetadataTemplate | BoxFolder; right?: BoxMetadataTemplate | BoxFolder; comparison: Pick<BoxComparison, 'left' | 'right'> }) {
  return <div className="compare-diff-scroll"><box-diff-viewer heading={heading} before-label={comparison.left.alias} after-label={comparison.right.alias} before-text={comparisonText(left)} after-text={comparisonText(right)} mode="split"></box-diff-viewer></div>
}

function ComparisonEmpty({ title, message }: { title: string; message: string }) {
  return <div className="compare-empty"><h3>{title}</h3><p>{message}</p></div>
}
