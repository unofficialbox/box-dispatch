import '@unofficialbox/box-open-elements/card'
import '@unofficialbox/box-open-elements/link-button'
import '@unofficialbox/box-open-elements/section'
import type { ConnectionSummary, DeploymentPlan, DispatchRun } from '../types'
import { DetailList, DetailsRail } from '../components/DetailsRail'
import { deploymentOutcome } from '../deploymentPresentation'

type Destination = { id: string; title: string; description: string; href?: string; providerID?: string }

export function SummaryPage({ plan, connections, run, onOpenProvider, onViewChanges, onOverview }: { plan: DeploymentPlan; connections: ConnectionSummary[]; run: DispatchRun; onOpenProvider: (providerID: string) => void; onViewChanges: (runID: string) => void; onOverview: () => void }) {
  const outcome = deploymentOutcome(run)
  const complete = outcome.label === 'Complete'
  const needsAttention = outcome.label === 'Needs attention'
  const summaryEyebrow = complete ? 'Deployment complete' : needsAttention ? 'Deployment needs attention' : 'Deployment recorded'
  const summaryTitle = complete ? `${plan.name} is ready` : needsAttention ? `${plan.name} needs attention` : `${plan.name} was recorded`
  const summaryCopy = complete ? 'Every selected system finished successfully. Open a destination to review the deployed experience.' : needsAttention ? 'Some components remain or require manual work. Review the recorded changes and provider results before treating this deployment as complete.' : 'Provider results were not recorded, so this deployment cannot be confirmed complete. Review its audit details before continuing.'
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
    <box-card className={`summary-surface${needsAttention ? ' summary-surface-attention' : complete ? '' : ' summary-surface-recorded'}`}><section>
      <div className="summary-success-mark" aria-hidden="true">{complete ? '✓' : needsAttention ? '!' : 'i'}</div>
      <p className="summary-eyebrow">{summaryEyebrow}</p>
      <h2 id="deployment-summary-title">{summaryTitle}</h2>
      <p className="summary-lede">{summaryCopy}</p>
      <box-section className="summary-destinations" heading="Open your deployment" description="Launch a deployed workspace or application.">
        <ul>{destinations.map((destination) => <li key={destination.id}>
          <div><strong>{destination.title}</strong><span>{destination.description}</span></div>
          {destination.href ? <box-link-button className="summary-destination-link" href={destination.href} target="_blank" rel="noreferrer" label={`Open ${destination.title}`}></box-link-button> : <box-button label="Open" tone="primary" onClick={() => onOpenProvider(destination.providerID!)}></box-button>}
        </li>)}</ul>
      </box-section>
      <div className="summary-actions"><box-button label="Review changes" tone="neutral" onClick={() => onViewChanges(run.id)}></box-button><box-button label="Return to overview" onClick={onOverview}></box-button></div>
    </section></box-card>
    <DetailsRail title="Deployment summary" description="A final record of the deployment run."><DetailList rows={[["Deployment", plan.name], ["Status", outcome.label], ["Run ID", run.id], ["Systems", plan.components.map((component) => component.name).join(', ')], ["Strategy", plan.strategy === 'reuse' ? 'Reuse existing' : 'Create new']]}/></DetailsRail>
  </section>
}
