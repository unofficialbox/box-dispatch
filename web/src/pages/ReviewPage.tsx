import { DetailList, DetailsRail } from '../components/DetailsRail'
import type { DeploymentPlan } from '../types'

export function ReviewPage({ plan, notice, onDeploy, onEditConnections, onBack }: { plan: DeploymentPlan; notice: string; onDeploy: () => void; onEditConnections: () => void; onBack: () => void }) {
  const ready = plan.components.every((component) => component.ready)
  const status = ready ? 'Ready' : 'Not ready'
  return <section className="review-layout review-layout-single" aria-label="Review deployment plan">
    <article className="task-surface review-surface"><header className="task-heading"><div><h2>{ready ? 'Review and validate' : 'Connections need attention'}</h2><p>{ready ? 'Confirm the plan, then run a safe validation before applying changes.' : 'Verify every selected system before validation.'}</p></div><span className={`status ${ready ? 'ready' : 'running'}`}>{status}</span></header><div className="review-summary"><div><span>Solution</span><strong>{plan.template}</strong></div><div><span>Strategy</span><strong>{plan.strategy === 'reuse' ? 'Reuse existing' : 'Create new'}</strong></div><div><span>Systems</span><strong>{plan.components.map((component) => component.name).join(' + ')}</strong></div></div><div className="review-details"><PlanGroup title="Package" rows={[["Source", plan.repository]]}/>{plan.components.map((component) => <PlanGroup key={component.id} title={component.name} rows={[["Connection", component.ready ? 'Ready' : 'Not ready']]}/>)}</div><p className="notice" role="status">{notice}</p><footer className="action-row"><div className="stage-navigation"><box-button label="Back" tone="secondary" onClick={onBack}></box-button><box-button label="Validate deployment" tone="primary" disabled={!ready} onClick={onDeploy}></box-button></div></footer></article>
    <DetailsRail title="Plan summary"><DetailList rows={[["Strategy", plan.strategy === 'reuse' ? 'Reuse existing' : 'Create new'], ['Systems', plan.components.map((component) => component.name).join(', ')], ['Connections', status]]}/><box-button label="Edit connections" tone="secondary" onClick={onEditConnections}></box-button></DetailsRail>
  </section>
}

function PlanGroup({ title, rows }: { title: string; rows: [string, string][] }) {
  return <section className="plan-group"><h3>{title}</h3>{rows.map(([label, value]) => <div className="plan-row" key={label}><span>{label}</span><strong>{value}</strong></div>)}</section>
}

