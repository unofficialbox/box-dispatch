import type { DeploymentPlan } from '../types'

export function DeploymentConfirmationDialog({ plan, packagePreparing, packageMessage, onCancel, onConfirm }: { plan: DeploymentPlan; packagePreparing: boolean; packageMessage?: string; onCancel: () => void; onConfirm: () => void }) {
  const systems = plan.components.map((component) => component.name).join(', ')
  return <div className="confirmation-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) onCancel() }}>
    <section className="deployment-confirmation" role="dialog" aria-modal="true" aria-labelledby="deployment-confirmation-title" aria-describedby="deployment-confirmation-description">
      <header><span className="confirmation-mark" aria-hidden="true">✓</span><div><p className="eyebrow">Ready to apply</p><h2 id="deployment-confirmation-title">Start deployment?</h2></div></header>
      <p id="deployment-confirmation-description">Dispatch will apply the validated changes to the selected environments. Salesforce can take several minutes to finish.</p>
      <dl><div><dt>Deployment</dt><dd>{plan.name}</dd></div><div><dt>Systems</dt><dd>{systems}</dd></div><div><dt>Strategy</dt><dd>{plan.strategy === 'reuse' ? 'Reuse existing' : 'Create new'}</dd></div></dl>
      {packagePreparing && <div className="confirmation-wait" role="status"><span className="confirmation-pulse" aria-hidden="true"></span><div><strong>Salesforce setup is still running</strong><p>{packageMessage || 'Box for Salesforce is installing in the background. Deployment will be available when it finishes.'}</p></div></div>}
      <footer><box-button label="Cancel" tone="neutral" onClick={onCancel}></box-button><box-button label={packagePreparing ? 'Waiting for Salesforce…' : 'Start deployment'} tone="primary" disabled={packagePreparing} onClick={onConfirm}></box-button></footer>
    </section>
  </div>
}
