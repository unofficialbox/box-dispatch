import { useState } from 'react'
import { ConnectionDetailsPanel } from '../components/ConnectionDetailsPanel'
import { ProviderLogo } from '../components/ProviderLogo'
import { readinessLabel, type ConnectionSummary, type DeploymentPlan } from '../types'

export function ConnectPage({ plan, connections, notice, onBoxConnection, onSalesforceConnection, onOpenProvider, onBack, onNext }: { plan: DeploymentPlan; connections: ConnectionSummary[]; notice: string; onBoxConnection: () => void; onSalesforceConnection: () => void; onOpenProvider: (providerID: string) => void; onBack: () => void; onNext: () => void }) {
  const [selectedID, setSelectedID] = useState(plan.components[0]?.id ?? 'box')
  const selected = plan.components.find((component) => component.id === selectedID) ?? plan.components[0]
  const selectedConnection = connections.find((connection) => connection.name.toLowerCase() === selected?.id)
  const openConnection = (id: string) => { if (id === 'box') onBoxConnection(); else onSalesforceConnection() }
  return <section className="connect-stage" aria-label="Connect systems">
    <header className="task-heading"><div><h2>Confirm connections</h2><p>Use a verified connection for every selected system.</p></div></header>
    <box-split-view className="connect-workspace" label="System connections and selected connection details" ratio={0.64}>
      <div slot="primary" className="connection-list" role="group" aria-label="Selected system connections">{plan.components.map((component) => {
        const connection = connections.find((candidate) => candidate.name.toLowerCase() === component.id)
        return <box-card className={`connection-card ${component.ready ? 'connection-ready' : 'connection-attention'} ${component.id === selectedID ? 'selected' : ''}`} key={component.id}><article className="connection-row"><button className="connection-summary" type="button" aria-pressed={component.id === selectedID} onClick={() => setSelectedID(component.id)}><span className="connection-state" aria-hidden="true">{component.ready ? '✓' : '!'}</span><ProviderLogo provider={component.id}/><span className="connection-summary-copy"><strong>{component.name}</strong><small>{component.ready ? 'Verified and ready' : 'Connection required'}</small></span></button><div className="connection-action"><strong>{readinessLabel(component.ready)}</strong><div className="connection-buttons">{component.ready && connection?.launchUrl ? <box-button label="Open" aria-label={`Open ${component.name}`} tone="neutral" onClick={() => onOpenProvider(component.id)}></box-button> : null}<box-button label={component.ready ? 'Manage' : 'Connect'} tone="neutral" onClick={() => openConnection(component.id)}></box-button></div></div></article></box-card>
      })}</div>
      {selected && <ConnectionDetailsPanel component={selected} connection={selectedConnection} onEdit={() => openConnection(selected.id)}/>}
    </box-split-view>
    <footer className="connect-footnote"><p className="notice" role="status">{notice}</p><div className="stage-navigation"><box-button label="Back" tone="neutral" onClick={onBack}></box-button><box-button label="Continue to configure" tone="primary" onClick={onNext}></box-button></div></footer>
  </section>
}
