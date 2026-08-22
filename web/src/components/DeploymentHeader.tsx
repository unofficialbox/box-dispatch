import type { DeploymentPlan, DispatchRun, Phase } from '../types'

const formatDeploymentTitle = (value: string) => value.replace(/^clm\b/i, 'CLM')

export function DeploymentHeader({ plan, activePhase, run, onPhaseChange }: { plan: DeploymentPlan; activePhase: Phase; run: DispatchRun | null; onPhaseChange: (phase: Phase) => void }) {
  const readiness = plan.components.every((component) => component.ready) ? 'Ready to validate' : 'Connection needs attention'
  const isRunning = activePhase === 'Deploy' && (run?.status === 'queued' || run?.status === 'running')
  const state = run?.status === 'completed' ? run.action === 'validate' ? 'Validation complete' : 'Deployment complete' : run?.status === 'failed' ? 'Needs attention' : isRunning ? 'In progress' : activePhase === 'Connect' && readiness === 'Ready to validate' ? 'Connections ready' : readiness
  return <header className="deployment-header"><div className="header-row"><div><div className="breadcrumbs"><a href="#workspace">Deployments</a><span aria-hidden="true">/</span><span>{formatDeploymentTitle(plan.template || 'New deployment')}</span></div><div className="title-row"><h1>{formatDeploymentTitle(plan.template || 'Deployment plan')}</h1><span className={`meta-status ${isRunning ? 'running' : run?.status === 'failed' ? 'failed' : 'ready'}`}>{state}</span></div></div><button className="environment" type="button" aria-label="Selected environment"><strong>Development</strong><b aria-hidden="true">⌄</b></button></div><WorkflowIndicator activePhase={activePhase} run={run} onPhaseChange={onPhaseChange} /></header>
}

function WorkflowIndicator({ activePhase, run, onPhaseChange }: { activePhase: Phase; run: DispatchRun | null; onPhaseChange: (phase: Phase) => void }) {
  const steps: { label: string; phase: Phase; available: boolean }[] = [
    { label: 'Choose', phase: 'Choose', available: true },
    { label: 'Connect', phase: 'Connect', available: true },
    { label: 'Configure', phase: 'Configure', available: true },
    { label: 'Validate', phase: 'Review', available: true },
    { label: 'Deploy', phase: 'Deploy', available: run?.action === 'deploy' || (run?.action === 'validate' && run.status === 'completed') },
  ]
  const running = run?.status === 'queued' || run?.status === 'running'
  const activeIndex = activePhase === 'Choose' ? 0 : activePhase === 'Connect' ? 1 : activePhase === 'Configure' ? 2 : activePhase === 'Review' ? 3 : activePhase === 'Deploy' ? run?.action === 'deploy' ? 4 : 3 : 2
  return <ol className="workflow-indicator" aria-label="Deployment workflow">{steps.map((step, index) => {
    const failed = run?.status === 'failed' && index === activeIndex
    const complete = index < activeIndex || (run?.status === 'completed' && index === activeIndex)
    const active = index === activeIndex && (running || activePhase === 'Choose' || activePhase === 'Connect' || activePhase === 'Configure' || activePhase === 'Review')
    return <li className={`${complete ? 'complete' : ''} ${active ? 'active' : ''} ${failed ? 'failed' : ''}`} key={step.label}><button className="workflow-step" type="button" aria-current={active ? 'step' : undefined} disabled={!step.available} onClick={() => onPhaseChange(step.phase)}><span className="workflow-node">{complete ? '✓' : failed ? '!' : ''}</span><span className="workflow-label">{step.label}</span>{active && <small>In progress</small>}</button></li>
  })}</ol>
}
