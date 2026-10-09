import { useEffect, useRef, useState } from 'react'
import { Alert } from '@unofficialbox/box-open-elements-react/alert'
import { EmptyProviderConnection, ProviderConnectionPanel, ProviderConnectionRow } from '../components/ProviderConnectionPanel'
import { SolutionTileGroup } from '../components/SolutionTileGroup'
import { StrategyTileGroup } from '../components/StrategyTileGroup'
import type { ConnectionSummary, DeploymentDefaults } from '../types'

type SettingsPageProps = {
  defaults: DeploymentDefaults
  connections: ConnectionSummary[]
  onSaveDefaults: (defaults: Pick<DeploymentDefaults, 'templateId' | 'strategy' | 'components'>) => Promise<DeploymentDefaults>
  onBoxConnection: () => void
  onSalesforceConnection: () => void
  onRemoveBoxConnection: (id: string) => Promise<boolean>
  onRemoveSalesforceConnection: (id: string) => Promise<boolean>
  boxConnectionsBusy?: boolean
  salesforceConnectionsBusy?: boolean
}

const defaultSolutions = [
  { id: 'clm', name: 'Contract Lifecycle Management (CLM)', description: 'Contract workflows across Box and Salesforce.', available: true },
  { id: 'citizen-services', name: 'Citizen Services', description: 'Digital intake and case workflows for public services.', available: false },
  { id: 'life-sciences-etmf', name: 'Life Sciences eTMF', description: 'Trial master file workflows and regulated content.', available: false },
  { id: 'insurance-claims', name: 'Insurance Claims Management', description: 'Claims intake, evidence, and resolution workflows.', available: false },
] as const

function DefaultSolutionList({ selectedID, disabled = false, readOnly = false, onSelect }: { selectedID: string; disabled?: boolean; readOnly?: boolean; onSelect?: (id: string) => void }) {
  const options = defaultSolutions.map((solution) => ({
    id: solution.id,
    label: solution.name,
    description: solution.description,
    disabled: !solution.available,
    disabledReason: solution.available ? undefined : 'Not available yet',
    status: { label: solution.available && selectedID === solution.id ? 'Selected' : solution.available ? 'Available' : 'Coming soon', tone: solution.available && selectedID === solution.id ? 'success' as const : 'neutral' as const },
  }))
  return <SolutionTileGroup className="settings-default-solution-list" legend="Default solution" name="default-solution" options={options} value={selectedID} disabled={disabled || readOnly} onChange={onSelect}/>
}

function DefaultSystemSwitch({ checked, disabled, label, description, onChange }: { checked: boolean; disabled?: boolean; label: string; description: string; onChange?: (checked: boolean) => void }) {
  const ref = useRef<HTMLElement>(null)
  useEffect(() => {
    const element = ref.current
    if (!element || !onChange) return
    const handleChange = (event: Event) => onChange((event as CustomEvent<{ checked: boolean }>).detail.checked)
    element.addEventListener('checked-changed', handleChange)
    return () => element.removeEventListener('checked-changed', handleChange)
  }, [onChange])
  return <box-switch ref={ref} checked={checked} disabled={disabled} label={label} description={description}></box-switch>
}

function BoxConnections({ connection, busy, onManage, onRemove }: { connection: ConnectionSummary; busy: boolean; onManage: () => void; onRemove: (id: string) => Promise<boolean> }) {
  const records = connection.connections ?? []
  return <ProviderConnectionPanel provider="box" title="Box connections" count={records.length} onManage={onManage}>
    {records.length ? records.map((record) => {
      const name = record.alias || record.identity || 'Box connection'
      const id = record.id
      return <ProviderConnectionRow key={id || `${record.alias}-${record.identity}`} primary={name} details={[record.domain || '', `${record.identity || record.subjectType || 'Box account'}${record.clientIdHint ? ` · ${record.clientIdHint}` : ''}`].filter(Boolean)} selected={record.selected} ready={record.status?.trim().toLowerCase() === 'ready'} removeLabel={`Remove ${name}`} removeDisabled={busy || !id} onRemove={id ? () => { void onRemove(id) } : undefined}/>
    }) : <EmptyProviderConnection provider="Box"/>}
  </ProviderConnectionPanel>
}

function SalesforceConnections({ connection, busy, onManage, onRemove }: { connection: ConnectionSummary; busy: boolean; onManage: () => void; onRemove: (id: string) => Promise<boolean> }) {
  const records = connection.orgs ?? []
  return <ProviderConnectionPanel provider="salesforce" title="Salesforce environments" count={records.length} onManage={onManage}>
    {records.length ? records.map((record) => {
      const details = [record.kind || 'Org', record.orgId ? `Org ID ${record.orgId}` : '', record.expiresAt ? `Expires ${record.expiresAt}` : ''].filter(Boolean).join(' · ')
      const name = record.alias || record.username || 'Salesforce org'
      const id = record.id
      return <ProviderConnectionRow key={id || `${record.alias}-${record.orgId}`} primary={name} details={[record.domain || '', record.username || details, record.username ? details : ''].filter(Boolean)} selected={record.selected} ready={record.status?.trim().toLowerCase() === 'ready'} removeLabel={`Remove ${name}`} removeDisabled={busy || !id} onRemove={id ? () => { void onRemove(id) } : undefined}/>
    }) : <EmptyProviderConnection provider="Salesforce"/>}
  </ProviderConnectionPanel>
}

export function SettingsPage({ defaults, connections, onSaveDefaults, onBoxConnection, onSalesforceConnection, onRemoveBoxConnection, onRemoveSalesforceConnection, boxConnectionsBusy = false, salesforceConnectionsBusy = false }: SettingsPageProps) {
  const box = connections.find((connection) => connection.name.toLowerCase() === 'box') ?? { name: 'Box', configured: false, verified: false }
  const salesforce = connections.find((connection) => connection.name.toLowerCase() === 'salesforce') ?? { name: 'Salesforce', configured: false, verified: false }
  const otherConnections = connections.filter((connection) => !['box', 'salesforce'].includes(connection.name.toLowerCase()))
  const [editingDefaults, setEditingDefaults] = useState(false)
  const [savingDefaults, setSavingDefaults] = useState(false)
  const [defaultsError, setDefaultsError] = useState('')
  const [draftTemplateID, setDraftTemplateID] = useState(defaults.templateId)
  const [draftStrategy, setDraftStrategy] = useState<DeploymentDefaults['strategy']>(defaults.strategy)
  const [draftComponents, setDraftComponents] = useState(defaults.components)
  const beginEditingDefaults = () => {
    setDraftTemplateID(defaults.templateId)
    setDraftStrategy(defaults.strategy)
    setDraftComponents(defaults.components)
    setDefaultsError('')
    setEditingDefaults(true)
  }
  const cancelEditingDefaults = () => {
    setDefaultsError('')
    setEditingDefaults(false)
  }
  const saveDefaults = async () => {
    setSavingDefaults(true)
    setDefaultsError('')
    try {
      await onSaveDefaults({ templateId: draftTemplateID, strategy: draftStrategy, components: draftComponents })
      setEditingDefaults(false)
    } catch (error: unknown) {
      setDefaultsError(error instanceof Error ? error.message : 'Deployment defaults could not be saved.')
    } finally {
      setSavingDefaults(false)
    }
  }
  return <section className="record-page settings-page" aria-labelledby="settings-title">
    <header className="record-page-heading"><div><p className="overview-eyebrow">Workspace configuration</p><h1 id="settings-title">Settings</h1><p>Manage provider connections and the starting configuration for new deployments.</p></div></header>
    <section className="record-page-section settings-section" aria-labelledby="connections-title"><header><div><h2 id="connections-title">Connections</h2><p>Every environment saved on this device. Remove entries here or open Manage to select and configure them.</p></div><span>{connections.reduce((count, connection) => count + (connection.connections?.length ?? connection.orgs?.length ?? (connection.configured ? 1 : 0)), 0)} saved</span></header><div className="settings-provider-grid"><BoxConnections connection={box} busy={boxConnectionsBusy} onManage={onBoxConnection} onRemove={onRemoveBoxConnection}/><SalesforceConnections connection={salesforce} busy={salesforceConnectionsBusy} onManage={onSalesforceConnection} onRemove={onRemoveSalesforceConnection}/>{otherConnections.map((connection) => <ProviderConnectionPanel key={connection.name} provider={connection.name} title={`${connection.name} connections`} count={connection.configured ? 1 : 0} onManage={() => undefined}><ProviderConnectionRow primary={connection.alias || connection.selection || connection.name} details={[connection.authType || 'Provider connection']} ready={connection.verified}/></ProviderConnectionPanel>)}</div></section>
    <section className="record-page-section settings-section defaults-settings" aria-labelledby="defaults-title"><header><div><h2 id="defaults-title">Defaults</h2><p>Starting configuration for new deployments. Each deployment can override these choices.</p></div>{editingDefaults ? null : <box-button label="Edit defaults" tone="neutral" onClick={beginEditingDefaults}></box-button>}</header>{editingDefaults ? <div className="settings-defaults-editor"><DefaultSolutionList selectedID={draftTemplateID} disabled={savingDefaults} onSelect={setDraftTemplateID}/><section className="settings-default-strategy"><StrategyTileGroup value={draftStrategy} disabled={savingDefaults} legend="Default deployment strategy" onChange={setDraftStrategy}/></section><fieldset className="settings-default-systems"><legend>Default systems</legend><div><span><strong>Box</strong><small>Required for every deployment</small></span><DefaultSystemSwitch checked disabled label="Box" description="Required"/></div><div><span><strong>Salesforce</strong><small>CRM records and workflows</small></span><DefaultSystemSwitch checked={draftComponents.includes('salesforce')} disabled={savingDefaults} label="Salesforce" description="Optional" onChange={(checked) => setDraftComponents(checked ? ['box', 'salesforce'] : ['box'])}/></div></fieldset><div className="settings-default-source"><span>Source</span><a href={defaults.repository} target="_blank" rel="noreferrer">{defaults.repository}</a></div>{defaultsError && <Alert className="settings-defaults-error" heading="Defaults could not be saved" message={defaultsError} tone="error" open/>}<footer><box-button label="Cancel" tone="neutral" disabled={savingDefaults} onClick={cancelEditingDefaults}></box-button><box-button label={savingDefaults ? 'Saving defaults…' : 'Save defaults'} tone="primary" disabled={savingDefaults} onClick={() => { void saveDefaults() }}></box-button></footer></div> : <div className="settings-defaults-view"><DefaultSolutionList selectedID={defaults.templateId} readOnly/></div>}</section>
  </section>
}
