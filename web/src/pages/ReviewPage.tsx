import { DetailList, DetailsRail } from '../components/DetailsRail'
import type { DeploymentPlan } from '../types'

export function ReviewPage({ plan, notice, checkingConnections, onDeploy, onEditConnections, onBack }: { plan: DeploymentPlan; notice: string; checkingConnections: boolean; onDeploy: () => void; onEditConnections: () => void; onBack: () => void }) {
  const ready = plan.components.every((component) => component.ready)
  const status = ready ? 'Ready' : 'Not ready'
  return <section className="review-layout review-layout-single" aria-label="Review deployment plan">
    <article className="task-surface review-surface"><header className="task-heading"><div><h2>{ready ? 'Review and validate' : 'Connections need attention'}</h2><p>{ready ? 'Confirm the package and connections, then validate before applying changes.' : 'Verify every selected system before validation.'}</p></div><span className={`status ${ready ? 'ready' : 'running'}`}>{status}</span></header><div className="review-details"><PlanGroup title="Package" rows={[["Source", plan.repository]]}/>{plan.components.map((component) => <PlanGroup key={component.id} title={component.name} rows={[["Connection", component.ready ? 'Ready' : 'Not ready']]}/>)}</div><p className="notice" role="status">{notice}</p><footer className="action-row"><div className="stage-navigation"><box-button label="Back" tone="neutral" onClick={onBack}></box-button><box-button label={checkingConnections ? 'Checking connections…' : 'Validate deployment'} tone="primary" disabled={!ready || checkingConnections} onClick={onDeploy}></box-button></div></footer></article>
    <DetailsRail title="Plan summary"><DetailList rows={[["Deployment", plan.name], ["Solution", plan.template], ["Strategy", plan.strategy === 'reuse' ? 'Reuse existing' : 'Create new'], ['Systems', plan.components.map((component) => component.name).join(', ')], ['Connections', status]]}/><box-button label="Edit connections" tone="neutral" onClick={onEditConnections}></box-button></DetailsRail>
  </section>
}

function PlanGroup({ title, rows }: { title: string; rows: [string, string][] }) {
  return <section className="plan-group"><h3>{title}</h3>{rows.map(([label, value]) => <div className="plan-row" key={label}><span>{label}</span><strong className={value === 'Ready' ? 'plan-value-ready' : undefined}>{value}</strong></div>)}</section>
}
