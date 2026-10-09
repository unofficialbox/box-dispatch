import '@unofficialbox/box-open-elements/card'
import '@unofficialbox/box-open-elements/link-button'
import '@unofficialbox/box-open-elements/section'
import type { ConnectionSummary, DeploymentPlan, DispatchRun } from '../types'
import { DetailList, DetailsRail } from '../components/DetailsRail'
import { deploymentOutcome } from '../deploymentPresentation'

type Destination = { id: string; title: string; description: string; href: string }
type DeploymentTarget = { providerID: string; name: string; domain?: string; idLabel: string; environmentID?: string; ready: boolean }

export function SummaryPage({ plan, connections, run, onOpenProvider, onViewChanges, onOverview }: { plan: DeploymentPlan; connections: ConnectionSummary[]; run: DispatchRun; onOpenProvider: (providerID: string) => void; onViewChanges: (runID: string) => void; onOverview: () => void }) {
  const outcome = deploymentOutcome(run)
  const complete = outcome.label === 'Complete'
  const needsAttention = outcome.label === 'Needs attention'
  const summaryEyebrow = complete ? 'Deployment complete' : needsAttention ? 'Deployment needs attention' : 'Deployment recorded'
  const summaryTitle = complete ? `${plan.name} is ready` : needsAttention ? `${plan.name} needs attention` : `${plan.name} was recorded`
  const summaryCopy = complete ? 'Every selected system finished successfully. Open a destination to review the deployed experience.' : needsAttention ? 'Some components remain or require manual work. Review the recorded changes and provider results before treating this deployment as complete.' : 'Provider results were not recorded, so this deployment cannot be confirmed complete. Review its audit details before continuing.'
  const includesBox = plan.components.some((component) => component.id === 'box')
  const includesSalesforce = plan.components.some((component) => component.id === 'salesforce')
  const boxSummary = connections.find((connection) => connection.name === 'Box')
  const salesforceSummary = connections.find((connection) => connection.name === 'Salesforce')
  const boxConnection = boxSummary?.connections?.find((connection) => connection.selected)
  const salesforceOrg = salesforceSummary?.orgs?.find((org) => org.selected)
  const boxReady = includesBox && Boolean(boxSummary?.launchUrl)
  const salesforceReady = includesSalesforce && Boolean(salesforceSummary?.launchUrl)
  const targets: DeploymentTarget[] = [
    ...(includesBox ? [{ providerID: 'box', name: 'Box', domain: boxConnection?.domain, idLabel: 'Enterprise ID', environmentID: boxConnection?.enterpriseId, ready: boxReady }] : []),
    ...(includesSalesforce ? [{ providerID: 'salesforce', name: 'Salesforce', domain: salesforceOrg?.domain, idLabel: 'Org ID', environmentID: salesforceOrg?.orgId, ready: salesforceReady }] : []),
  ]
  const experienceSite = run.resources?.find((resource) => resource.provider === 'salesforce' && resource.kind === 'experience_site' && resource.url)
  const destinations: Destination[] = [
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
      <box-section className="summary-targets" heading="Deployment targets" description="The provider environments configured by this deployment.">
        <ul>{targets.map((target) => <li key={target.providerID}>
          <div><strong>{target.name}</strong>{target.domain ? <span>{target.domain}</span> : <span>Domain not recorded</span>}{target.environmentID ? <small>{target.idLabel} {target.environmentID}</small> : null}</div>
          {target.ready ? <box-button label={`Open ${target.name}`} tone="neutral" onClick={() => onOpenProvider(target.providerID)}></box-button> : null}
        </li>)}</ul>
      </box-section>
      {destinations.length ? <box-section className="summary-destinations" heading="Open deployed applications" description="Launch a deployed Salesforce application.">
        <ul>{destinations.map((destination) => <li key={destination.id}>
          <div><strong>{destination.title}</strong><span>{destination.description}</span></div>
          <box-link-button className="summary-destination-link" href={destination.href} target="_blank" rel="noreferrer" label={`Open ${destination.title}`}></box-link-button>
        </li>)}</ul>
      </box-section> : null}
      <div className="summary-actions"><box-button label="Review changes" tone="neutral" onClick={() => onViewChanges(run.id)}></box-button><box-button label="Return to overview" onClick={onOverview}></box-button></div>
    </section></box-card>
    <DetailsRail title="Deployment summary" description="A final record of the deployment run."><DetailList rows={[["Deployment", plan.name], ["Status", outcome.label], ["Run ID", run.id], ["Systems", plan.components.map((component) => component.name).join(', ')], ["Strategy", plan.strategy === 'reuse' ? 'Reuse existing' : 'Create new']]}/></DetailsRail>
  </section>
}
