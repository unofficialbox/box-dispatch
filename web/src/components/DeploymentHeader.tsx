import { useEffect, useMemo, useRef } from 'react'
import '@unofficialbox/box-open-elements/breadcrumb'
import '@unofficialbox/box-open-elements/progress-steps'
import type { BreadcrumbItem } from '@unofficialbox/box-open-elements/breadcrumb'
import type { ProgressStepItem } from '@unofficialbox/box-open-elements/progress-steps'
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
  const statusTone = run?.status === 'failed' ? 'error' : isRunning ? 'inprogress' : state === 'Not ready' ? 'error' : 'success'
  const breadcrumbItems: BreadcrumbItem[] = [{ label: 'Deployments', href: '#workspace', value: 'deployments' }, { label: title, value: 'current' }]
  return <header className="deployment-header"><div className="header-row"><div className="deployment-title"><box-breadcrumb className="deployment-breadcrumb" label="Deployment location" items={breadcrumbItems}></box-breadcrumb><div className="title-row"><h1>{title}</h1><box-badge className="meta-status" label={state} tone={statusTone}></box-badge></div></div><box-select className="environment" label="Environment" hideLabel value="development" options={[{ label: 'Development', value: 'development' }]}></box-select></div><WorkflowIndicator activePhase={activePhase} run={run} onPhaseChange={onPhaseChange} /></header>
}

function WorkflowIndicator({ activePhase, run, onPhaseChange }: { activePhase: Phase; run: DispatchRun | null; onPhaseChange: (phase: Phase) => void }) {
  const progressRef = useRef<(HTMLElement & { items: ProgressStepItem[]; value: string }) | null>(null)
  const phases = useMemo<{ label: string; phase: Phase; available: boolean }[]>(() => [
    { label: 'Choose', phase: 'Choose', available: true },
    { label: 'Connect', phase: 'Connect', available: true },
    { label: 'Configure', phase: 'Configure', available: true },
    { label: 'Validate', phase: 'Review', available: true },
    { label: 'Deploy', phase: 'Deploy', available: run?.action === 'deploy' || (run?.action === 'validate' && run.status === 'completed') },
    { label: 'Summary', phase: 'Summary', available: run?.action === 'deploy' && run.status === 'completed' },
  ], [run?.action, run?.status])
  const activeIndex = activePhase === 'Choose' ? 0 : activePhase === 'Connect' ? 1 : activePhase === 'Configure' ? 2 : activePhase === 'Review' ? 3 : activePhase === 'Summary' ? 5 : activePhase === 'Deploy' ? run?.action === 'deploy' ? 4 : 3 : 2
  const currentPhase = phases[activeIndex]?.phase ?? 'Choose'
  const items = useMemo<ProgressStepItem[]>(() => phases.map((step, index) => {
    const failed = index === activeIndex && run?.status === 'failed' && (activePhase === 'Review' || activePhase === 'Deploy')
    const complete = index < activeIndex || (run?.status === 'completed' && index === activeIndex)
    return {
      label: step.label,
      value: step.phase,
      status: failed ? 'failed' : complete ? 'complete' : step.available ? undefined : 'disabled',
    }
  }), [activeIndex, activePhase, phases, run?.status])

  useEffect(() => {
    const progress = progressRef.current
    if (!progress) return
    progress.items = items
    progress.value = currentPhase
  }, [currentPhase, items])

  useEffect(() => {
    const progress = progressRef.current
    if (!progress) return
    const handleChange = (event: Event) => onPhaseChange((event as CustomEvent<{ value: Phase }>).detail.value)
    progress.addEventListener('value-changed', handleChange)
    return () => progress.removeEventListener('value-changed', handleChange)
  }, [onPhaseChange])

  return <box-progress-steps ref={progressRef} className="workflow-indicator" label="Deployment workflow"></box-progress-steps>
}
