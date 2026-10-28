import { useEffect, useMemo, useRef, useState } from 'react'
import { Alert } from '@unofficialbox/box-open-elements-react/alert'
import { Select } from '@unofficialbox/box-open-elements-react/select'
import { TextField } from '@unofficialbox/box-open-elements-react/text-field'
import '@unofficialbox/box-open-elements/metric-card'
import '@unofficialbox/box-open-elements/section'
import type { TableColumn, TableRow } from '@unofficialbox/box-open-elements/table'
import { BoeTable } from '../components/BoeTable'
import type { BoxAIAgent, BoxAIAgentComparison, BoxAIAgentDiff, BoxComparison, BoxConnectionOption, BoxFolder, BoxFolderComparison, BoxFolderDiff, BoxMetadataTemplate, BoxMetadataTemplateDiff, ConnectionSummary, DeploymentPlan } from '../types'

type ComparisonType = 'metadata_templates' | 'folders' | 'ai_agents'
type ComparisonResult = { type: 'metadata_templates'; value: BoxComparison } | { type: 'folders'; value: BoxFolderComparison } | { type: 'ai_agents'; value: BoxAIAgentComparison }
type ComparisonStatus = BoxMetadataTemplateDiff['status']
type ResultFilter = 'actionable' | ComparisonStatus | 'all'

const statusPresentation = {
  changed: { label: 'Changed', tone: 'brand' },
  left_only: { label: 'Missing', tone: 'warning' },
  right_only: { label: 'Extra', tone: 'brand' },
  equal: { label: 'Equal', tone: 'neutral' },
} as const

const metadataColumns: TableColumn[] = [
  { key: 'template', label: 'Metadata template' },
  { key: 'status', label: 'Result' },
  { key: 'plan', label: 'Plan', align: 'center', hostContent: true },
]

const folderColumns: TableColumn[] = [
  { key: 'path', label: 'Folder path' },
  { key: 'status', label: 'Result' },
]

const aiAgentColumns: TableColumn[] = [
  { key: 'agent', label: 'Custom agent' },
  { key: 'capabilities', label: 'Capabilities' },
  { key: 'status', label: 'Result' },
  { key: 'plan', label: 'Plan', align: 'center', hostContent: true },
]

const comparisonTypeOptions = [
  { label: 'Metadata templates', value: 'metadata_templates' },
  { label: 'Folders (connected user)', value: 'folders' },
  { label: 'AI Studio custom agents', value: 'ai_agents' },
]

const resultFilterOptions = [
  { label: 'Needs action', value: 'actionable' },
  { label: 'Changed', value: 'changed' },
  { label: 'Missing', value: 'left_only' },
  { label: 'Extra', value: 'right_only' },
  { label: 'Equal', value: 'equal' },
  { label: 'All results', value: 'all' },
]

const statusPriority: Record<ComparisonStatus, number> = { changed: 0, left_only: 1, right_only: 2, equal: 3 }
const portableMetadataFieldTypes = new Set(['string', 'float', 'date', 'enum', 'multiSelect'])
const matchesResultFilter = (status: ComparisonStatus, filter: ResultFilter) => filter === 'all' || (filter === 'actionable' ? status === 'changed' || status === 'left_only' : status === filter)
const compareByAttention = <T extends { status: ComparisonStatus }>(left: T, right: T) => statusPriority[left.status] - statusPriority[right.status]

const comparisonText = (value?: BoxMetadataTemplate | BoxFolder | BoxAIAgent) => value ? JSON.stringify(value, null, 2) : ''
const selectableConnections = (connections: ConnectionSummary[]) => connections.find((connection) => connection.name === 'Box')?.connections?.filter((connection): connection is BoxConnectionOption & { id: string } => Boolean(connection.id)) ?? []

async function responseMessage(response: Response) {
  const payload = await response.json().catch(() => null) as { error?: string } | null
  return payload?.error?.trim() || 'The live Box comparison could not be completed.'
}

export function ComparePage({ connections, onPromotionPrepared }: { connections: ConnectionSummary[]; onPromotionPrepared?: (plan: DeploymentPlan) => void }) {
  const boxConnections = useMemo(() => selectableConnections(connections), [connections])
  const [comparisonType, setComparisonType] = useState<ComparisonType>('metadata_templates')
  const [leftConnectionID, setLeftConnectionID] = useState('')
  const [rightConnectionID, setRightConnectionID] = useState('')
  const [result, setResult] = useState<ComparisonResult | null>(null)
  const [running, setRunning] = useState(false)
  const [error, setError] = useState('')

  const prepareAIAgentPromotion = async (comparison: BoxAIAgentComparison, name: string, agentKeys: string[]) => {
    const response = await fetch('/api/box/promotions/ai-agents', {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name, leftConnectionId: comparison.left.connectionId, rightConnectionId: comparison.right.connectionId, agentKeys }),
    })
    if (!response.ok) throw new Error(await responseMessage(response))
    const prepared = await response.json() as DeploymentPlan
    onPromotionPrepared?.(prepared)
  }

  const prepareMetadataPromotion = async (comparison: BoxComparison, name: string, templateKeys: string[]) => {
    const response = await fetch('/api/box/promotions/metadata-templates', {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name, leftConnectionId: comparison.left.connectionId, rightConnectionId: comparison.right.connectionId, templateKeys }),
    })
    if (!response.ok) throw new Error(await responseMessage(response))
    const prepared = await response.json() as DeploymentPlan
    onPromotionPrepared?.(prepared)
  }

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
    const next: ComparisonType = value === 'folders' || value === 'ai_agents' ? value : 'metadata_templates'
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
      const endpoint = comparisonType === 'folders' ? '/api/box/comparisons/folders' : comparisonType === 'ai_agents' ? '/api/box/comparisons/ai-agents' : '/api/box/comparisons'
      const response = await fetch(endpoint, {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ leftConnectionId: leftConnectionID, rightConnectionId: rightConnectionID }),
      })
      if (!response.ok) throw new Error(await responseMessage(response))
      if (comparisonType === 'folders') {
        setResult({ type: 'folders', value: await response.json() as BoxFolderComparison })
      } else if (comparisonType === 'ai_agents') {
        setResult({ type: 'ai_agents', value: await response.json() as BoxAIAgentComparison })
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
    <header className="record-page-heading compare-heading"><h1 id="compare-title">Compare Box environments</h1></header>
    <section className="compare-controls" aria-label="Comparison settings">
      {boxConnections.length < 2 ? <Alert heading="Two Box connections required" message="Add another saved Box connection in Settings before running a live enterprise comparison." tone="warning" open/> : <div className="compare-control-grid">
        <Select label="Box object type" value={comparisonType} options={comparisonTypeOptions} disabled={running} onValueChanged={(event) => changeComparisonType(event.detail.value)}/>
        <Select label="Source Box connection" value={leftConnectionID} options={options} disabled={running} onValueChanged={(event) => changeConnection('left', event.detail.value)}/>
        <Select label="Destination Box connection" value={rightConnectionID} options={options} disabled={running} onValueChanged={(event) => changeConnection('right', event.detail.value)}/>
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
      <SummaryMetrics summary={comparison.summary}/>
      {result.type === 'folders' ? <FolderResults key={result.value.completedAt} comparison={result.value}/> : result.type === 'ai_agents' ? <AIAgentResults key={result.value.completedAt} comparison={result.value} onPrepare={prepareAIAgentPromotion}/> : <MetadataResults key={result.value.completedAt} comparison={result.value} onPrepare={prepareMetadataPromotion}/>}
    </> : null}
  </section>
}

function EnvironmentSummary({ label, environment }: { label: string; environment: BoxComparison['left'] }) {
  return <article><span>{label}</span><strong>{environment.alias}</strong><p>{environment.domain || environment.identity}</p></article>
}

function SummaryMetrics({ summary }: { summary: BoxComparison['summary'] }) {
  return <section className="record-page-metrics compare-metrics" aria-label="Comparison summary">
    <box-metric-card heading="Changed" value={String(summary.changed)}></box-metric-card>
    <box-metric-card heading="Missing" value={String(summary.leftOnly)}></box-metric-card>
    <box-metric-card heading="Extra" value={String(summary.rightOnly)}></box-metric-card>
    <box-metric-card heading="Equal" value={String(summary.equal)}></box-metric-card>
  </section>
}

function MetadataResults({ comparison, onPrepare }: { comparison: BoxComparison; onPrepare: (comparison: BoxComparison, name: string, templateKeys: string[]) => Promise<void> }) {
  const [selectedKey, setSelectedKey] = useState(comparison.templates.find((template) => template.status !== 'equal')?.key ?? comparison.templates[0]?.key ?? '')
  const [resultFilter, setResultFilter] = useState<ResultFilter>('actionable')
  const [query, setQuery] = useState('')
  const deployableKeys = useMemo(() => comparison.templates.filter(isDeployableMetadataTemplate).map((template) => template.key), [comparison.templates])
  const [plannedKeys, setPlannedKeys] = useState<string[]>([])
  const [deploymentName, setDeploymentName] = useState(`Metadata templates to ${comparison.right.alias}`)
  const [preparing, setPreparing] = useState(false)
  const [promotionError, setPromotionError] = useState('')
  const filteredTemplates = useMemo(() => {
    const needle = query.trim().toLocaleLowerCase()
    return [...comparison.templates].sort((left, right) => compareByAttention(left, right) || (left.displayName || left.key).localeCompare(right.displayName || right.key)).filter((template) => matchesResultFilter(template.status, resultFilter) && (!needle || `${template.displayName} ${template.key}`.toLocaleLowerCase().includes(needle)))
  }, [comparison.templates, query, resultFilter])
  const selected = filteredTemplates.find((template) => template.key === selectedKey) ?? filteredTemplates[0]
  const rows = useMemo<TableRow[]>(() => filteredTemplates.map((template) => {
    const presentation = statusPresentation[template.status]
    return { id: template.key, cells: { template: template.displayName || template.key, status: { kind: 'badge', text: presentation.label, tone: presentation.tone }, plan: isDeployableMetadataTemplate(template) ? '' : 'Not deployable' }, detail: `${template.key} · ${template.status === 'changed' ? `${template.changes.length} semantic changes` : presentation.label}` }
  }), [filteredTemplates])

  const togglePlannedTemplate = (key: string) => {
    setPlannedKeys((current) => current.includes(key) ? current.filter((candidate) => candidate !== key) : [...current, key])
    setPromotionError('')
  }
  const prepare = async () => {
    if (!deploymentName.trim() || plannedKeys.length === 0 || preparing) return
    setPreparing(true)
    setPromotionError('')
    try {
      await onPrepare(comparison, deploymentName.trim(), plannedKeys)
    } catch (caught) {
      setPromotionError(caught instanceof Error ? caught.message : 'The metadata template deployment plan could not be prepared.')
    } finally {
      setPreparing(false)
    }
  }

  return <section className="record-page-section compare-results" aria-labelledby="comparison-results-title"><header><h2 id="comparison-results-title">Metadata templates</h2></header>
    {deployableKeys.length > 0 ? <PromotionPlan count={plannedKeys.length} available={deployableKeys.length} deploymentName={deploymentName} preparing={preparing} onNameChange={setDeploymentName} onPrepare={() => void prepare()}/> : null}
    {promotionError ? <Alert className="compare-error" heading="Deployment plan could not be prepared" message={promotionError} tone="error" open/> : null}
    <div className="compare-browser">
      <section className="compare-template-list" aria-label="Metadata template comparison results"><ComparisonListControls query={query} onQueryChange={setQuery} resultFilter={resultFilter} onFilterChange={setResultFilter} visible={filteredTemplates.length} total={comparison.templates.length}/><BoeTable columns={metadataColumns} rows={rows} label="Metadata template comparison results" emptyText="No metadata templates match these filters." selectionMode="single" selectedIds={selected ? [selected.key] : []} onSelectionChange={(ids) => setSelectedKey(ids[0] ?? '')}>{filteredTemplates.map((template) => {
        if (!isDeployableMetadataTemplate(template)) return null
        const planned = plannedKeys.includes(template.key)
        const label = template.displayName || template.key
        return <span slot={`cell-${template.key}-plan`} className="compare-plan-action" key={template.key}>{planned ? <PlanIconButton icon="cart-1" label={`Remove ${label} from deployment plan`} onAction={() => togglePlannedTemplate(template.key)}/> : <PlanIconButton icon="plus" label={`Add ${label} to deployment plan`} onAction={() => togglePlannedTemplate(template.key)}/>}</span>
      })}</BoeTable></section>
      <section className="compare-template-detail" aria-live="polite">{selected ? <MetadataDiff template={selected} comparison={comparison}/> : <ComparisonEmpty title="No matching templates" message="Change the result filter or search to see more metadata templates."/>}</section>
    </div>
  </section>
}

function FolderResults({ comparison }: { comparison: BoxFolderComparison }) {
  const [selectedPath, setSelectedPath] = useState(comparison.folders.find((folder) => folder.status !== 'equal')?.path ?? comparison.folders[0]?.path ?? '')
  const [resultFilter, setResultFilter] = useState<ResultFilter>('actionable')
  const [query, setQuery] = useState('')
  const filteredFolders = useMemo(() => {
    const needle = query.trim().toLocaleLowerCase()
    return [...comparison.folders].sort((left, right) => compareByAttention(left, right) || left.path.localeCompare(right.path)).filter((folder) => matchesResultFilter(folder.status, resultFilter) && (!needle || `${folder.name} ${folder.path}`.toLocaleLowerCase().includes(needle)))
  }, [comparison.folders, query, resultFilter])
  const selected = filteredFolders.find((folder) => folder.path === selectedPath) ?? filteredFolders[0]
  const rows = useMemo<TableRow[]>(() => filteredFolders.map((folder) => {
    const presentation = statusPresentation[folder.status]
    return { id: folder.path, cells: { path: folder.path, status: { kind: 'badge', text: presentation.label, tone: presentation.tone } }, detail: folder.status === 'changed' ? `${folder.changes.length} direct-item count changes` : presentation.label }
  }), [filteredFolders])
  const truncated = comparison.scope.leftTruncated || comparison.scope.rightTruncated

  return <section className="record-page-section compare-results" aria-labelledby="comparison-results-title"><header><h2 id="comparison-results-title">Folders</h2></header>
    {truncated ? <Alert className="compare-scope-alert" heading="Folder scope limit reached" message={`Results stop at depth ${comparison.scope.maxDepth} and ${comparison.scope.maxFolders} folders per side. Paths outside that boundary are not represented.`} tone="warning" open/> : null}
    <div className="compare-browser">
      <section className="compare-template-list" aria-label="Folder comparison results"><ComparisonListControls query={query} onQueryChange={setQuery} resultFilter={resultFilter} onFilterChange={setResultFilter} visible={filteredFolders.length} total={comparison.folders.length}/><BoeTable columns={folderColumns} rows={rows} label="Folder comparison results" emptyText="No folders match these filters." selectionMode="single" selectedIds={selected ? [selected.path] : []} onSelectionChange={(ids) => setSelectedPath(ids[0] ?? '')}/></section>
      <section className="compare-template-detail" aria-live="polite">{selected ? <FolderDiffView folder={selected} comparison={comparison}/> : <ComparisonEmpty title="No matching folders" message="Change the result filter or search to see more folders."/>}</section>
    </div>
  </section>
}

function AIAgentResults({ comparison, onPrepare }: { comparison: BoxAIAgentComparison; onPrepare: (comparison: BoxAIAgentComparison, name: string, agentKeys: string[]) => Promise<void> }) {
  const [selectedKey, setSelectedKey] = useState(comparison.agents.find((agent) => agent.status !== 'equal')?.key ?? comparison.agents[0]?.key ?? '')
  const [resultFilter, setResultFilter] = useState<ResultFilter>('actionable')
  const [query, setQuery] = useState('')
  const deployableKeys = useMemo(() => comparison.agents.filter(isDeployableAIAgent).map((agent) => agent.key), [comparison.agents])
  const [plannedKeys, setPlannedKeys] = useState<string[]>([])
  const [deploymentName, setDeploymentName] = useState(`AI agents to ${comparison.right.alias}`)
  const [preparing, setPreparing] = useState(false)
  const [promotionError, setPromotionError] = useState('')
  const filteredAgents = useMemo(() => {
    const needle = query.trim().toLocaleLowerCase()
    return [...comparison.agents].sort((left, right) => compareByAttention(left, right) || (left.name || left.key).localeCompare(right.name || right.key)).filter((agent) => matchesResultFilter(agent.status, resultFilter) && (!needle || `${agent.name} ${agent.key} ${agent.capabilities.join(' ')}`.toLocaleLowerCase().includes(needle)))
  }, [comparison.agents, query, resultFilter])
  const selected = filteredAgents.find((agent) => agent.key === selectedKey) ?? filteredAgents[0]
  const rows = useMemo<TableRow[]>(() => filteredAgents.map((agent) => {
    const presentation = statusPresentation[agent.status]
    return {
      id: agent.key,
      cells: { agent: agent.name || agent.key, capabilities: agent.capabilities.join(', ') || 'None', status: { kind: 'badge', text: presentation.label, tone: presentation.tone }, plan: isDeployableAIAgent(agent) ? '' : 'Not deployable' },
      detail: agent.status === 'changed' ? `${agent.changes.length} semantic changes` : presentation.label,
    }
  }), [filteredAgents])

  const togglePlannedAgent = (key: string) => {
    setPlannedKeys((current) => current.includes(key) ? current.filter((candidate) => candidate !== key) : [...current, key])
    setPromotionError('')
  }
  const prepare = async () => {
    if (!deploymentName.trim() || plannedKeys.length === 0 || preparing) return
    setPreparing(true)
    setPromotionError('')
    try {
      await onPrepare(comparison, deploymentName.trim(), plannedKeys)
    } catch (caught) {
      setPromotionError(caught instanceof Error ? caught.message : 'The AI agent promotion plan could not be prepared.')
    } finally {
      setPreparing(false)
    }
  }

  return <section className="record-page-section compare-results" aria-labelledby="comparison-results-title"><header><h2 id="comparison-results-title">AI Studio custom agents</h2></header>
    {deployableKeys.length > 0 ? <PromotionPlan count={plannedKeys.length} available={deployableKeys.length} deploymentName={deploymentName} preparing={preparing} onNameChange={setDeploymentName} onPrepare={() => void prepare()}/> : <Alert className="compare-scope-alert" heading="No eligible source-only custom agents" message="Promotion currently supports unrestricted AI Studio custom agents with one Ask or Extract capability. Changed agents require an update workflow, destination-only agents already exist, and restricted agents retain provider-specific audience principals that cannot be copied safely." tone="info" open/>}
    {promotionError ? <Alert className="compare-error" heading="Deployment plan could not be prepared" message={promotionError} tone="error" open/> : null}
    <div className="compare-browser">
      <section className="compare-template-list" aria-label="AI agent comparison results"><ComparisonListControls query={query} onQueryChange={setQuery} resultFilter={resultFilter} onFilterChange={setResultFilter} visible={filteredAgents.length} total={comparison.agents.length}/><BoeTable columns={aiAgentColumns} rows={rows} label="AI agent comparison results" emptyText="No custom agents match these filters." selectionMode="single" selectedIds={selected ? [selected.key] : []} onSelectionChange={(ids) => setSelectedKey(ids[0] ?? '')}>{filteredAgents.map((agent) => {
        if (!isDeployableAIAgent(agent)) return null
        const planned = plannedKeys.includes(agent.key)
        const label = agent.name || agent.key
        return <span slot={`cell-${agent.key}-plan`} className="compare-plan-action" key={agent.key}>{planned ? <PlanIconButton icon="cart-1" label={`Remove ${label} from deployment plan`} onAction={() => togglePlannedAgent(agent.key)}/> : <PlanIconButton icon="plus" label={`Add ${label} to deployment plan`} onAction={() => togglePlannedAgent(agent.key)}/>}</span>
      })}</BoeTable></section>
      <section className="compare-template-detail" aria-live="polite">{selected ? <AIAgentDiffView agent={selected} comparison={comparison}/> : <ComparisonEmpty title="No matching AI agents" message="Change the result filter or search to see more custom agents."/>}</section>
    </div>
  </section>
}

function PlanIconButton({ icon, label, onAction }: { icon: 'plus' | 'cart-1'; label: string; onAction: () => void }) {
  const ref = useRef<HTMLElementTagNameMap['box-icon-button']>(null)
  const onActionRef = useRef(onAction)
  useEffect(() => { onActionRef.current = onAction }, [onAction])
  useEffect(() => {
    const button = ref.current
    if (!button) return
    const handleClick = (event: Event) => {
      event.preventDefault()
      onActionRef.current()
    }
    button.addEventListener('click', handleClick)
    return () => button.removeEventListener('click', handleClick)
  }, [])
  return <box-icon-button ref={ref} className="compare-plan-icon" icon={icon} label={label}></box-icon-button>
}

function ComparisonListControls({ query, onQueryChange, resultFilter, onFilterChange, visible, total }: { query: string; onQueryChange: (value: string) => void; resultFilter: ResultFilter; onFilterChange: (value: ResultFilter) => void; visible: number; total: number }) {
  return <div className="compare-list-controls">
    <TextField label="Filter results" value={query} placeholder="Search by name or key" onValueChanged={(event) => onQueryChange(event.detail.value)}/>
    <Select label="Show" value={resultFilter} options={resultFilterOptions} onValueChanged={(event) => onFilterChange(event.detail.value as ResultFilter)}/>
    <p aria-live="polite">Showing {visible} of {total}</p>
  </div>
}

function PromotionPlan({ count, available, deploymentName, preparing, onNameChange, onPrepare }: { count: number; available: number; deploymentName: string; preparing: boolean; onNameChange: (value: string) => void; onPrepare: () => void }) {
  const invalidName = deploymentName.length > 80
  return <section className="compare-promotion-plan" aria-label="Deployment plan"><strong>{count} of {available} selected</strong><TextField label="Deployment name" value={deploymentName} required invalid={invalidName} errorMessage={invalidName ? 'Use 80 characters or fewer.' : undefined} onValueChanged={(event) => onNameChange(event.detail.value)}/><box-button label={preparing ? 'Preparing deployment…' : 'Prepare deployment'} tone="primary" disabled={preparing || count === 0 || !deploymentName.trim() || invalidName} onClick={onPrepare}></box-button></section>
}

function isDeployableAIAgent(agent: BoxAIAgentDiff) {
  const capabilities = agent.left?.capabilities ?? agent.capabilities
  return agent.status === 'left_only' && Boolean(agent.left) && capabilities.length === 1 && (capabilities[0] === 'Ask' || capabilities[0] === 'Extract') && (agent.left?.allowedEntityTypes?.length ?? 0) === 0
}

function isDeployableMetadataTemplate(template: BoxMetadataTemplateDiff) {
  if (!template.left || !template.left.fields.every((field) => portableMetadataFieldTypes.has(field.type))) return false
  if (template.status === 'left_only') return true
  if (template.status !== 'changed' || !template.right) return false
  const desiredFields = new Map(template.left.fields.map((field) => [field.key, field]))
  return template.right.fields.every((current) => {
    const desired = desiredFields.get(current.key)
    if (!desired || desired.type !== current.type) return false
    const desiredOptions = new Set(desired.options ?? [])
    return (current.options ?? []).every((option) => desiredOptions.has(option))
  })
}

function MetadataDiff({ template, comparison }: { template: BoxMetadataTemplateDiff; comparison: BoxComparison }) {
  return <DiffViewer heading={`${template.key}.json`} left={template.left} right={template.right} comparison={comparison}/>
}

function FolderDiffView({ folder, comparison }: { folder: BoxFolderDiff; comparison: BoxFolderComparison }) {
  return <DiffViewer heading="folder-structure.json" left={folder.left} right={folder.right} comparison={comparison}/>
}

function AIAgentDiffView({ agent, comparison }: { agent: BoxAIAgentDiff; comparison: BoxAIAgentComparison }) {
  return <DiffViewer heading={`${agent.key.replaceAll(' ', '-')}.json`} left={agent.left} right={agent.right} comparison={comparison}/>
}

function DiffViewer({ heading, left, right, comparison }: { heading: string; left?: BoxMetadataTemplate | BoxFolder | BoxAIAgent; right?: BoxMetadataTemplate | BoxFolder | BoxAIAgent; comparison: Pick<BoxComparison, 'left' | 'right'> }) {
  return <div className="compare-diff-scroll"><box-diff-viewer heading={heading} before-label={comparison.left.alias} after-label={comparison.right.alias} before-text={comparisonText(left)} after-text={comparisonText(right)} mode="split"></box-diff-viewer></div>
}

function ComparisonEmpty({ title, message }: { title: string; message: string }) {
  return <div className="compare-empty"><h3>{title}</h3><p>{message}</p></div>
}
