import type { ConnectionSummary, DeploymentPlan, DispatchRun } from '../types'
import { DetailList, DetailsRail } from '../components/DetailsRail'

type Destination = { id: string; title: string; description: string; href?: string; providerID?: string }

export function SummaryPage({ plan, connections, run, onOpenProvider, onOverview }: { plan: DeploymentPlan; connections: ConnectionSummary[]; run: DispatchRun; onOpenProvider: (providerID: string) => void; onOverview: () => void }) {
  const boxReady = plan.components.some((component) => component.id === 'box') && connections.some((connection) => connection.name === 'Box' && connection.launchUrl)
  const salesforceReady = plan.components.some((component) => component.id === 'salesforce') && connections.some((connection) => connection.name === 'Salesforce' && connection.launchUrl)
  const experienceSite = run.resources?.find((resource) => resource.provider === 'salesforce' && resource.kind === 'experience_site' && resource.url)
  const destinations: Destination[] = [
    ...(boxReady ? [{ id: 'box', title: 'Box workspace', description: 'Review the deployed contract content and workspace structure.', providerID: 'box' }] : []),
    ...(salesforceReady ? [
      { id: 'box-settings', title: 'Box App & Settings', description: 'Configure the Box for Salesforce managed package.', href: '/api/connections/salesforce/open?destination=box-settings' },
      { id: 'clm-app', title: 'Contract Lifecycle Management', description: 'Open the Salesforce app and sample contract records.', href: '/api/connections/salesforce/open?destination=clm-app' },
    ] : []),
    ...(experienceSite?.url && experienceSite.id ? [{ id: 'experience-site', title: 'Experience Cloud site', description: 'Enter the published CLM experience with your Salesforce employee session.', href: `/api/connections/salesforce/open?destination=experience-site&site=${encodeURIComponent(experienceSite.id)}` }] : []),
  ]
  return <section className="summary-workspace" aria-labelledby="deployment-summary-title">
    <section className="summary-surface">
      <div className="summary-success-mark" aria-hidden="true">✓</div>
      <p className="summary-eyebrow">Deployment complete</p>
      <h2 id="deployment-summary-title">{plan.name} is ready</h2>
      <p className="summary-lede">Every selected system finished successfully. Open a destination to review the deployed experience.</p>
      <section className="summary-destinations" aria-labelledby="summary-destinations-title">
        <h3 id="summary-destinations-title">Open your deployment</h3>
        <ul>{destinations.map((destination) => <li key={destination.id}>
          <div><strong>{destination.title}</strong><span>{destination.description}</span></div>
          {destination.href ? <a className="summary-destination-link" href={destination.href} target="_blank" rel="noreferrer" aria-label={`Open ${destination.title}`}>Open</a> : <box-button label="Open" tone="primary" onClick={() => onOpenProvider(destination.providerID!)}></box-button>}
        </li>)}</ul>
      </section>
      <div className="summary-actions"><box-button label="Return to overview" onClick={onOverview}></box-button></div>
    </section>
    <DetailsRail title="Deployment summary" description="A final record of the completed run."><DetailList rows={[["Deployment", plan.name], ["Status", "Deployment complete"], ["Run ID", run.id], ["Systems", plan.components.map((component) => component.name).join(', ')], ["Strategy", plan.strategy === 'reuse' ? 'Reuse existing' : 'Create new']]}/></DetailsRail>
  </section>
}
