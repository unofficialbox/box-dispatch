import { useState } from 'react'
import { ConnectionDetailsPanel } from '../components/ConnectionDetailsPanel'
import { readinessLabel, type ConnectionSummary, type DeploymentPlan } from '../types'

export function ConnectPage({ plan, connections, notice, onBoxConnection, onSalesforceConnection, onBack, onNext }: { plan: DeploymentPlan; connections: ConnectionSummary[]; notice: string; onBoxConnection: () => void; onSalesforceConnection: () => void; onBack: () => void; onNext: () => void }) {
  const [selectedID, setSelectedID] = useState(plan.components[0]?.id ?? 'box')
  const selected = plan.components.find((component) => component.id === selectedID) ?? plan.components[0]
  const selectedConnection = connections.find((connection) => connection.name.toLowerCase() === selected?.id)
  const openConnection = (id: string) => { if (id === 'box') onBoxConnection(); else onSalesforceConnection() }
  return <section className="connect-stage" aria-label="Connect systems">
    <header className="task-heading"><div><h2>Confirm connections</h2><p>Each selected system must be verified before validation.</p></div></header>
    <box-split-view className="connect-workspace" label="System connections and selected connection details" ratio={0.64}>
      <div slot="primary" className="connection-list">{plan.components.map((component) => <box-card className={`connection-card ${component.ready ? 'connection-ready' : 'connection-attention'} ${component.id === selectedID ? 'selected' : ''}`} key={component.id}><article className="connection-row" role="button" tabIndex={0} aria-pressed={component.id === selectedID} onClick={() => setSelectedID(component.id)} onKeyDown={(event) => { if (event.key === 'Enter' || event.key === ' ') { event.preventDefault(); setSelectedID(component.id) } }}><span className="connection-state" aria-hidden="true">{component.ready ? '✓' : '!'}</span><div><h3>{component.name}</h3><small>{component.ready ? 'Verified and ready' : 'Connection required'}</small></div><div className="connection-action"><strong>{readinessLabel(component.ready)}</strong><box-button label={component.ready ? 'Manage' : 'Connect'} tone="secondary" onClick={(event) => { event.stopPropagation(); openConnection(component.id) }}></box-button></div></article></box-card>)}</div>
      {selected && <ConnectionDetailsPanel component={selected} connection={selectedConnection} onEdit={() => openConnection(selected.id)}/>}
    </box-split-view>
    <footer className="connect-footnote"><p className="notice" role="status">{notice}</p><div className="stage-navigation"><box-button label="Back" tone="secondary" onClick={onBack}></box-button><box-button label="Continue to configure" tone="primary" onClick={onNext}></box-button></div></footer>
  </section>
}

