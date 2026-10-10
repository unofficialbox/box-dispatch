import { useEffect, useRef, useState } from 'react'
import type { BoxElementEventMap } from '@unofficialbox/box-open-elements/native-types'
import { DetailList, DetailsRail } from '../components/DetailsRail'
import { ProviderLogo } from '../components/ProviderLogo'
import { SelectableResourceRow } from '../components/SelectableResourceRow'
import { StrategyTileGroup } from '../components/StrategyTileGroup'
import { readinessLabel, type ConnectionSummary, type DeploymentPlan } from '../types'

type ProviderID = 'box' | 'salesforce'

const componentCatalog: Record<ProviderID, string[]> = {
  box: ['Workspace structure', 'Metadata templates', 'Doc Gen templates', 'Sample content'],
  salesforce: ['CLM Contract', 'CLM Clause', 'Layouts', 'Permission sets'],
}

type Props = {
  plan: DeploymentPlan
  connections: ConnectionSummary[]
  notice: string
  checkingConnections: boolean
  refreshing?: boolean
  componentSelections: Record<string, string[]>
  onToggleProvider: (provider: ProviderID, included: boolean) => void
  onToggleComponent: (provider: ProviderID, component: string, included: boolean) => void
  onStrategyChange: (strategy: DeploymentPlan['strategy']) => void
  onBack: () => void
  onNext: () => void
}

export function ConfigurePage({ plan, connections, notice, checkingConnections, refreshing = false, componentSelections, onToggleProvider, onToggleComponent, onStrategyChange, onBack, onNext }: Props) {
  const [selectedProviderID, setSelectedProviderID] = useState<ProviderID>('box')
  const salesforceIncluded = plan.components.some((component) => component.id === 'salesforce')
  const selectedConnection = connections.find((connection) => connection.name === (selectedProviderID === 'box' ? 'Box' : 'Salesforce'))
  const selectedComponents = componentSelections[selectedProviderID] ?? []
  const selectedTitle = selectedProviderID === 'box' ? 'Box content' : 'Salesforce metadata'
  return <section className="configure-stage" aria-label="Configure deployment">
    <header className="task-heading"><div><h2>Configure deployment</h2><p>Choose a strategy, providers, and components.</p></div></header>
    <box-split-view className="configure-workspace" label="Deployment configuration and selected provider details" ratio={0.66}>
      <div slot="primary">
        <section className="strategy-section"><StrategyTileGroup value={plan.strategy} legend="Deployment strategy" onChange={onStrategyChange}/></section>
        <div className="configuration-list"><ProviderConfiguration id="box" title="Box content" description="Deploy the selected content model and workspace structure to Box." fallback="Required for every Dispatch solution" connection={connections.find((connection) => connection.name === 'Box')} refreshing={refreshing} included required selected={selectedProviderID === 'box'} onSelect={() => setSelectedProviderID('box')} onToggle={onToggleProvider}/><ProviderConfiguration id="salesforce" title="Salesforce metadata" description="Deploy the selected objects, fields, layouts, and supported setup to Salesforce." fallback={salesforceIncluded ? 'Selected for this deployment' : 'Not included in this deployment'} connection={connections.find((connection) => connection.name === 'Salesforce')} refreshing={refreshing} included={salesforceIncluded} selected={selectedProviderID === 'salesforce'} onSelect={() => setSelectedProviderID('salesforce')} onToggle={onToggleProvider}/></div>
        <ComponentScope provider={selectedProviderID} title={selectedTitle} included={selectedProviderID === 'box' || salesforceIncluded} selectedComponents={selectedComponents} onToggle={onToggleComponent}/>
      </div>
      <DetailsRail title={`${selectedTitle} details`}><ProviderLogo provider={selectedProviderID} size="standard"/><DetailList rows={[["Connection", selectedConnection?.selection || selectedConnection?.authType || 'Not configured'], ['Status', refreshing ? 'Checking live status' : readinessLabel(Boolean(selectedConnection?.verified))], ['Components selected', `${selectedComponents.length} of ${componentCatalog[selectedProviderID].length}`], ['Strategy', plan.strategy === 'reuse' ? 'Reuse existing' : 'Create new']]}/></DetailsRail>
    </box-split-view>
    <footer className="configuration-footer"><p className="notice" role="status">{refreshing ? 'Checking the selected provider connections now…' : notice}</p><div className="stage-navigation"><box-button label="Back" tone="neutral" onClick={onBack}></box-button><box-button label={refreshing || checkingConnections ? 'Checking connections…' : 'Review plan'} tone="primary" disabled={refreshing || checkingConnections} onClick={onNext}></box-button></div></footer>
  </section>
}

function ProviderConfiguration({ id, title, description, fallback, connection, refreshing, included, required = false, selected, onSelect, onToggle }: { id: ProviderID; title: string; description: string; fallback: string; connection?: ConnectionSummary; refreshing: boolean; included: boolean; required?: boolean; selected: boolean; onSelect: () => void; onToggle: (provider: ProviderID, included: boolean) => void }) {
  const ref = useRef<HTMLElementTagNameMap['box-switch']>(null)
  useEffect(() => {
    const switchElement = ref.current
    if (!switchElement || required) return
    const handleChange = (event: BoxElementEventMap['box-switch']['checked-changed']) => onToggle(id, event.detail.checked)
    switchElement.addEventListener('checked-changed', handleChange)
    return () => switchElement.removeEventListener('checked-changed', handleChange)
  }, [id, onToggle, required])

  const ready = Boolean(connection?.verified)
  const statusLabel = refreshing ? 'Checking' : ready ? 'Ready' : 'Needs attention'
  const meta = `${description}\n${connectionDetailsText(connection, fallback)}`
  const toggle = <box-switch ref={ref} checked={included} disabled={required} label={included ? 'Included' : 'Not included'} description={required ? 'Required' : 'Optional'}></box-switch>
  return <SelectableResourceRow className={`configuration-resource-row ${included ? 'included' : 'excluded'}`} label={title} meta={meta} value={id} selected={selected} icon={<ProviderLogo provider={id} size="standard"/>} status={<box-badge label={statusLabel} tone={refreshing ? 'info' : ready ? 'success' : 'error'}></box-badge>} actions={toggle} onSelect={onSelect}/>
}

function ComponentScope({ provider, title, included, selectedComponents, onToggle }: { provider: ProviderID; title: string; included: boolean; selectedComponents: string[]; onToggle: (provider: ProviderID, component: string, included: boolean) => void }) {
  return <section className="component-scope" aria-labelledby="component-scope-title"><div><h3 id="component-scope-title">{title} components</h3><p>{included ? 'Select the supported components that Dispatch should include.' : 'Include this system to select its components.'}</p></div>{componentCatalog[provider].map((component) => <ComponentToggle key={component} provider={provider} component={component} checked={selectedComponents.includes(component)} disabled={!included} onToggle={onToggle}/>)}</section>
}

function ComponentToggle({ provider, component, checked, disabled, onToggle }: { provider: ProviderID; component: string; checked: boolean; disabled: boolean; onToggle: (provider: ProviderID, component: string, included: boolean) => void }) {
  const ref = useRef<HTMLElementTagNameMap['box-switch']>(null)
  useEffect(() => {
    const switchElement = ref.current
    if (!switchElement) return
    const handleChange = (event: BoxElementEventMap['box-switch']['checked-changed']) => onToggle(provider, component, event.detail.checked)
    switchElement.addEventListener('checked-changed', handleChange)
    return () => switchElement.removeEventListener('checked-changed', handleChange)
  }, [component, onToggle, provider])
  return <div className="component-scope-row"><span>{component}</span><box-switch ref={ref} checked={checked} disabled={disabled} label={checked ? 'Included' : 'Not included'}></box-switch></div>
}

function connectionDetailsText(connection: ConnectionSummary | undefined, fallback: string) {
  if (!connection) return fallback
  if (!connection.configured) return 'Connection · Not configured'
  const parts = [connection.selection, connection.authType, readinessLabel(connection.verified), connection.expiresAt ? `Expires ${connection.expiresAt}` : ''].filter(Boolean)
  return `Connection · ${parts.join(' · ')}`
}
