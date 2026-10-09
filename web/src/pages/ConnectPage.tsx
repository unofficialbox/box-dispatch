import { useState } from 'react'
import { ConnectionDetailsPanel } from '../components/ConnectionDetailsPanel'
import { ProviderLogo } from '../components/ProviderLogo'
import { SelectableResourceRow } from '../components/SelectableResourceRow'
import type { ConnectionSummary, DeploymentPlan } from '../types'

export function ConnectPage({ plan, connections, refreshing = false, notice, onBoxConnection, onSalesforceConnection, onOpenProvider, onBack, onNext }: { plan: DeploymentPlan; connections: ConnectionSummary[]; refreshing?: boolean; notice: string; onBoxConnection: () => void; onSalesforceConnection: () => void; onOpenProvider: (providerID: string) => void; onBack: () => void; onNext: () => void }) {
  const [selectedID, setSelectedID] = useState(plan.components[0]?.id ?? 'box')
  const selected = plan.components.find((component) => component.id === selectedID) ?? plan.components[0]
  const selectedConnection = connections.find((connection) => connection.name.toLowerCase() === selected?.id)
  const openConnection = (id: string) => { if (id === 'box') onBoxConnection(); else onSalesforceConnection() }
  return <section className="connect-stage" aria-label="Connect systems">
    <header className="task-heading"><div><h2>Confirm connections</h2><p>Use a verified connection for every selected system.</p></div></header>
    <box-split-view className="connect-workspace" label="System connections and selected connection details" ratio={0.64}>
      <div slot="primary" className="connection-list" role="group" aria-label="Selected system connections">{plan.components.map((component) => {
        const connection = connections.find((candidate) => candidate.name.toLowerCase() === component.id)
        const statusLabel = refreshing ? 'Checking' : component.ready ? 'Ready' : 'Needs attention'
        const statusTone = refreshing ? 'info' : component.ready ? 'success' : 'error'
        const connectionMeta = refreshing ? 'Refreshing live connection status' : component.ready ? [connection?.selection, connection?.authType, 'Verified and ready'].filter(Boolean).join(' · ') : 'Connect and verify this system before validation'
        const actions = <div className="connection-buttons">{component.ready && connection?.launchUrl ? <box-button label="Open" aria-label={`Open ${component.name}`} tone="neutral" onClick={() => onOpenProvider(component.id)}></box-button> : null}<box-button label={component.ready ? 'Manage' : 'Connect'} tone="neutral" onClick={() => openConnection(component.id)}></box-button></div>
        return <SelectableResourceRow key={component.id} className="connection-resource-row" label={component.name} meta={connectionMeta} value={component.id} selected={component.id === selectedID} icon={<ProviderLogo provider={component.id}/>} status={<box-badge label={statusLabel} tone={statusTone}></box-badge>} actions={actions} onSelect={setSelectedID}/>
      })}</div>
      {selected && <ConnectionDetailsPanel component={selected} connection={selectedConnection} onEdit={() => openConnection(selected.id)}/>}
    </box-split-view>
    <footer className="connect-footnote"><p className="notice" role="status">{refreshing ? 'Checking the selected provider connections now…' : notice}</p><div className="stage-navigation"><box-button label="Back" tone="neutral" onClick={onBack}></box-button><box-button label={refreshing ? 'Checking connections…' : 'Continue to configure'} tone="primary" disabled={refreshing} onClick={onNext}></box-button></div></footer>
  </section>
}
