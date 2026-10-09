import { useRef } from 'react'
import { Dialog } from '@unofficialbox/box-open-elements-react/dialog'
import type { DeploymentPlan } from '../types'

export function DeploymentConfirmationDialog({ plan, packagePreparing, packageMessage, onCancel, onConfirm }: { plan: DeploymentPlan; packagePreparing: boolean; packageMessage?: string; onCancel: () => void; onConfirm: () => void }) {
  const closeIntent = useRef<'cancel' | 'confirm' | null>(null)
  const systems = plan.components.map((component) => component.name).join(', ')
  return <Dialog
    className="deployment-confirmation-dialog"
    open
    heading="Start deployment?"
    description="Dispatch will apply the validated changes to the selected environments. Salesforce can take several minutes to finish."
    confirmLabel="Start deployment"
    confirmDisabled={packagePreparing}
    confirmBusy={packagePreparing}
    confirmBusyLabel="Waiting for Salesforce…"
    size="medium"
    onCancel={() => {
      closeIntent.current = 'cancel'
      onCancel()
    }}
    onConfirm={() => {
      closeIntent.current = 'confirm'
      onConfirm()
    }}
    onOpenChanged={(event) => {
      if (event.detail.open) return
      if (closeIntent.current === null) onCancel()
      closeIntent.current = null
    }}
  >
    <section className="deployment-confirmation">
      <p className="eyebrow">Ready to apply</p>
      <dl><div><dt>Deployment</dt><dd>{plan.name}</dd></div><div><dt>Systems</dt><dd>{systems}</dd></div><div><dt>Strategy</dt><dd>{plan.strategy === 'reuse' ? 'Reuse existing' : 'Create new'}</dd></div></dl>
      {packagePreparing && <div className="confirmation-wait" role="status"><span className="confirmation-pulse" aria-hidden="true"></span><div><strong>Salesforce setup is still running</strong><p>{packageMessage || 'Box for Salesforce is installing in the background. Deployment will be available when it finishes.'}</p></div></div>}
    </section>
  </Dialog>
}
