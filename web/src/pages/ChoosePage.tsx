import { useEffect, useRef } from 'react'
import { TextField } from '@unofficialbox/box-open-elements-react/text-field'
import { ProviderLogo } from '../components/ProviderLogo'
import { SolutionTileGroup } from '../components/SolutionTileGroup'
import type { SolutionTemplate } from '../types'

function ProviderSwitch({ checked, disabled = false, label, description, onToggle }: { checked: boolean; disabled?: boolean; label?: string; description?: string; onToggle?: () => void }) {
  const switchRef = useRef<HTMLElementTagNameMap['box-switch']>(null)
  useEffect(() => {
    const element = switchRef.current
    if (!element || !onToggle) return
    const toggle = () => onToggle()
    element.addEventListener('checked-changed', toggle)
    return () => element.removeEventListener('checked-changed', toggle)
  }, [onToggle])
  return <box-switch ref={switchRef} checked={checked} disabled={disabled} label={label} description={description}></box-switch>
}

function SystemProvider({ provider, name }: { provider: string; name: string }) {
  return <span className="system-provider" data-system-provider={provider}><ProviderLogo provider={provider} size="compact"/><span className="system-provider-copy"><strong>{name}</strong></span></span>
}

export function ChoosePage({ templates, selectedTemplateID, selectedComponents, deploymentName, assembling, notice, onTemplateChange, onToggleSalesforce, onDeploymentNameChange, onAssemble }: { templates: SolutionTemplate[]; selectedTemplateID: string; selectedComponents: string[]; deploymentName: string; assembling: boolean; notice: string; onTemplateChange: (templateID: string) => void; onToggleSalesforce: () => void; onDeploymentNameChange: (name: string) => void; onAssemble: () => void }) {
  const hasName = deploymentName.trim().length > 0
  const nameLength = [...deploymentName].length
  const nameIsValid = hasName && nameLength <= 80
  const solutionOptions = templates.map((template) => ({ id: template.id, label: template.name, description: template.description, meta: template.sector || 'Solution' }))
  return <section className="choose-layout choose-stage" aria-label="Choose deployment">
    <article className="task-surface choose-card">
      <header className="task-heading"><h2>Choose a solution</h2></header>
      <section className="deployment-name-field"><TextField label="Name this deployment" value={deploymentName} placeholder="For example, Northstar CLM rollout" required invalid={deploymentName.length > 0 && !nameIsValid} errorMessage={nameLength > 80 ? 'Use 80 characters or fewer.' : undefined} onValueChanged={(event) => onDeploymentNameChange(event.detail.value)}/></section>
      <SolutionTileGroup className="solution-list" legend="Solutions" name="deployment-solution" options={solutionOptions} value={selectedTemplateID} disabled={assembling} onChange={onTemplateChange}/>
      <section className="system-selection" aria-labelledby="systems-title"><header><h3 id="systems-title">Systems</h3></header><div className="system-grid"><div className="system-option required"><SystemProvider provider="box" name="Box"/><ProviderSwitch checked disabled label="Box" description="Required"/><em>Required</em></div><div className="system-option"><SystemProvider provider="salesforce" name="Salesforce"/><ProviderSwitch checked={selectedComponents.includes('salesforce')} disabled={assembling} label="Salesforce" description="Optional" onToggle={onToggleSalesforce}/><em>Optional</em></div><div className="system-option unavailable" aria-disabled="true"><SystemProvider provider="databricks" name="Databricks"/><ProviderSwitch checked={false} disabled label="Databricks" description="Coming soon"/><em>Coming soon</em></div><div className="system-option unavailable" aria-disabled="true"><SystemProvider provider="amazon bedrock" name="Amazon Bedrock"/><ProviderSwitch checked={false} disabled label="Amazon Bedrock" description="Coming soon"/><em>Coming soon</em></div></div></section>
      <footer className="choose-actions"><p className="notice" role="status">{notice}</p><div className="stage-navigation"><box-button label={assembling ? 'Preparing package…' : 'Prepare package'} tone="primary" disabled={assembling || !nameIsValid} onClick={onAssemble}></box-button></div></footer>
    </article>
  </section>
}
