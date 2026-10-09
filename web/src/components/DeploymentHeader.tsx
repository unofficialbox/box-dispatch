import { useEffect, useRef } from 'react'
import '@unofficialbox/box-open-elements/breadcrumb'
import '@unofficialbox/box-open-elements/path'
import type { BreadcrumbItem } from '@unofficialbox/box-open-elements/breadcrumb'
import type { PathStage } from '@unofficialbox/box-open-elements/path'
import type { DeploymentPlan, DispatchRun, Phase } from '../types'
import { deploymentOutcome } from '../deploymentPresentation'

const formatDeploymentTitle = (value: string) => {
  if (/contract lifecycle management|^clm\b/i.test(value)) return 'CLM deployment'
  return value || 'Deployment plan'
}

export function DeploymentHeader({ plan, draftName, activePhase, run }: { plan: DeploymentPlan; draftName?: string; activePhase: Phase; run: DispatchRun | null }) {
  const readiness = plan.components.every((component) => component.ready) ? 'Ready' : 'Not ready'
  const isRunning = activePhase === 'Deploy' && (run?.status === 'queued' || run?.status === 'running')
  const completedDeploymentOutcome = run?.action === 'deploy' && run.status === 'completed' ? deploymentOutcome(run) : null
  const state = run?.status === 'completed' ? run.action === 'validate' ? 'Validation complete' : completedDeploymentOutcome?.label === 'Complete' ? 'Deployment complete' : completedDeploymentOutcome?.label ?? 'Recorded' : run?.status === 'failed' ? 'Not ready' : isRunning ? 'In progress' : readiness
  const title = draftName !== undefined ? draftName.trim() || 'New deployment' : plan.name.trim() || formatDeploymentTitle(plan.template)
  const statusTone = run?.status === 'failed' || state === 'Needs attention' ? 'error' : isRunning ? 'inprogress' : state === 'Not ready' ? 'error' : state === 'Recorded' ? 'info' : 'success'
  const breadcrumbItems: BreadcrumbItem[] = [{ label: 'Deployments', href: '#workspace', value: 'deployments' }, { label: title, value: 'current' }]
  return <header className="deployment-header"><div className="header-row"><div className="deployment-title"><box-breadcrumb className="deployment-breadcrumb" label="Deployment location" items={breadcrumbItems}></box-breadcrumb><div className="title-row"><h1>{title}</h1><box-badge className="meta-status" label={state} tone={statusTone}></box-badge></div></div><box-select className="environment" label="Environment" hideLabel value="development" options={[{ label: 'Development', value: 'development' }]}></box-select></div><WorkflowPath activePhase={activePhase} run={run} /></header>
}

const stages: PathStage[] = [
  { id: 'Choose', label: 'Choose' },
  { id: 'Connect', label: 'Connect' },
  { id: 'Configure', label: 'Configure' },
  { id: 'Review', label: 'Validate' },
  { id: 'Deploy', label: 'Deploy' },
  { id: 'Summary', label: 'Summary' },
]

function WorkflowPath({ activePhase, run }: { activePhase: Phase; run: DispatchRun | null }) {
  const pathRef = useRef<(HTMLElement & { stages: PathStage[] }) | null>(null)
  const currentPhase = activePhase === 'Deploy' && run?.action !== 'deploy' ? 'Review' : activePhase
  const hasIncompleteDeployment = activePhase === 'Summary' && run?.action === 'deploy' && run.status === 'completed' && deploymentOutcome(run).label !== 'Complete'
  const hasError = run?.status === 'failed' && (activePhase === 'Review' || activePhase === 'Deploy') || hasIncompleteDeployment
  useEffect(() => {
    if (pathRef.current) pathRef.current.stages = stages
  }, [])

  return <box-path ref={pathRef} className="workflow-path" label="Deployment lifecycle" current={currentPhase} hasError={hasError}></box-path>
}
