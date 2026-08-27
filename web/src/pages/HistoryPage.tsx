import { DeploymentHistoryTable } from '../components/DeploymentHistoryTable'
import { deploymentOutcome } from '../deploymentPresentation'
import type { DeploymentSummary } from '../types'

export function HistoryPage({ deployments }: { deployments: DeploymentSummary[] }) {
  const complete = deployments.filter((deployment) => deploymentOutcome(deployment).label === 'Complete').length
  const attention = deployments.filter((deployment) => deploymentOutcome(deployment).label === 'Needs attention').length
  return <section className="record-page history-page" aria-labelledby="history-title">
    <header className="record-page-heading"><div><p className="overview-eyebrow">Audit records</p><h1 id="history-title">Deployment history</h1><p>Review every recorded deployment and its provider outcome.</p></div></header>
    <section className="record-page-metrics" aria-label="History summary">
      <div><span>Total deployments</span><strong>{deployments.length}</strong></div>
      <div><span>Complete</span><strong>{complete}</strong></div>
      <div><span>Needs attention</span><strong>{attention}</strong></div>
    </section>
    <section className="record-page-section" aria-labelledby="all-deployments-title"><header><div><h2 id="all-deployments-title">All deployments</h2><p>Newest records appear first.</p></div><span>{deployments.length} recorded</span></header><DeploymentHistoryTable deployments={deployments} caption="All deployments" includeResult/></section>
  </section>
}
