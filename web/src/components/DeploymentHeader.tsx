import type { DeploymentPlan, DispatchRun, Phase } from '../types'

const formatDeploymentTitle = (value: string) => {
  if (/contract lifecycle management|^clm\b/i.test(value)) return 'CLM deployment'
  return value || 'Deployment plan'
}

export function DeploymentHeader({ plan, draftName, activePhase, run, onPhaseChange }: { plan: DeploymentPlan; draftName?: string; activePhase: Phase; run: DispatchRun | null; onPhaseChange: (phase: Phase) => void }) {
  const readiness = plan.components.every((component) => component.ready) ? 'Ready' : 'Not ready'
  const isRunning = activePhase === 'Deploy' && (run?.status === 'queued' || run?.status === 'running')
  const state = run?.status === 'completed' ? run.action === 'validate' ? 'Validation complete' : 'Deployment complete' : run?.status === 'failed' ? 'Not ready' : isRunning ? 'In progress' : readiness
  const title = draftName !== undefined ? draftName.trim() || 'New deployment' : plan.name.trim() || formatDeploymentTitle(plan.template)
  return <header className="deployment-header"><div className="header-row"><div className="deployment-title"><div className="breadcrumbs"><a href="#workspace">Deployments</a><span aria-hidden="true">/</span><span>{title}</span></div><div className="title-row"><h1>{title}</h1><span className={`meta-status ${isRunning ? 'running' : run?.status === 'failed' ? 'failed' : 'ready'}`}>{state}</span></div></div><box-select className="environment" label="Environment" hideLabel value="development" options={[{ label: 'Development', value: 'development' }]}></box-select></div><WorkflowIndicator activePhase={activePhase} run={run} onPhaseChange={onPhaseChange} /></header>
}

function WorkflowIndicator({ activePhase, run, onPhaseChange }: { activePhase: Phase; run: DispatchRun | null; onPhaseChange: (phase: Phase) => void }) {
  const steps: { label: string; phase: Phase; available: boolean }[] = [
    { label: 'Choose', phase: 'Choose', available: true },
    { label: 'Connect', phase: 'Connect', available: true },
    { label: 'Configure', phase: 'Configure', available: true },
    { label: 'Validate', phase: 'Review', available: true },
    { label: 'Deploy', phase: 'Deploy', available: run?.action === 'deploy' || (run?.action === 'validate' && run.status === 'completed') },
    { label: 'Summary', phase: 'Summary', available: run?.action === 'deploy' && run.status === 'completed' },
  ]
  const activeIndex = activePhase === 'Choose' ? 0 : activePhase === 'Connect' ? 1 : activePhase === 'Configure' ? 2 : activePhase === 'Review' ? 3 : activePhase === 'Summary' ? 5 : activePhase === 'Deploy' ? run?.action === 'deploy' ? 4 : 3 : 2
  return <ol className="workflow-indicator" aria-label="Deployment workflow">{steps.map((step, index) => {
    const complete = index < activeIndex || (run?.status === 'completed' && index === activeIndex)
    const active = index === activeIndex && !complete
    const failed = active && run?.status === 'failed' && (activePhase === 'Review' || activePhase === 'Deploy')
    return <li className={`${complete ? 'complete' : ''} ${active ? 'active' : ''} ${failed ? 'failed' : ''}`} key={step.label}><button className="workflow-step" type="button" aria-current={active ? 'step' : undefined} aria-label={`${step.label}${complete ? ', complete' : active ? failed ? ', failed' : ', current step' : ''}`} disabled={!step.available} onClick={() => onPhaseChange(step.phase)}><span className="workflow-node"><span className="workflow-node-symbol" aria-hidden="true">{complete ? '✓' : failed ? '×' : ''}</span></span><span className="workflow-label">{step.label}</span></button></li>
  })}</ol>
}
