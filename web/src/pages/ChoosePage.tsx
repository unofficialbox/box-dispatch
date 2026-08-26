import { useEffect, useRef } from 'react'
import { ProviderLogo } from '../components/ProviderLogo'
import type { SolutionTemplate } from '../types'

type SwitchElement = HTMLElement & { checked: boolean }
type TextFieldElement = HTMLElement & { value: string }

function ProviderSwitch({ checked, disabled = false, label, description, onToggle }: { checked: boolean; disabled?: boolean; label?: string; description?: string; onToggle?: () => void }) {
  const switchRef = useRef<SwitchElement>(null)
  useEffect(() => {
    const element = switchRef.current
    if (!element || !onToggle) return
    const toggle = () => onToggle()
    element.addEventListener('checked-changed', toggle)
    return () => element.removeEventListener('checked-changed', toggle)
  }, [onToggle])
  return <box-switch ref={switchRef} checked={checked} disabled={disabled} label={label} description={description}></box-switch>
}

function SystemProvider({ provider, name, description }: { provider: string; name: string; description: string }) {
  return <span className="system-provider" data-system-provider={provider}><ProviderLogo provider={provider} size="compact"/><span className="system-provider-copy"><strong>{name}</strong><small>{description}</small></span></span>
}

export function ChoosePage({ templates, selectedTemplateID, selectedComponents, deploymentName, assembling, notice, onTemplateChange, onToggleSalesforce, onDeploymentNameChange, onAssemble }: { templates: SolutionTemplate[]; selectedTemplateID: string; selectedComponents: string[]; deploymentName: string; assembling: boolean; notice: string; onTemplateChange: (templateID: string) => void; onToggleSalesforce: () => void; onDeploymentNameChange: (name: string) => void; onAssemble: () => void }) {
  const nameRef = useRef<TextFieldElement>(null)
  useEffect(() => {
    const field = nameRef.current
    if (!field) return
    const handleValueChanged = (event: Event) => onDeploymentNameChange((event as CustomEvent<{ value: string }>).detail.value)
    field.addEventListener('value-changed', handleValueChanged)
    return () => field.removeEventListener('value-changed', handleValueChanged)
  }, [onDeploymentNameChange])
  const hasName = deploymentName.trim().length > 0
  const nameLength = [...deploymentName].length
  const nameIsValid = hasName && nameLength <= 80
  return <section className="choose-layout choose-stage" aria-label="Choose deployment">
    <article className="task-surface choose-card">
      <header className="task-heading"><div><h2>Choose a solution</h2><p>Select one solution and the systems Dispatch should configure.</p></div></header>
      <section className="deployment-name-field" aria-labelledby="deployment-name-heading"><div><h3 id="deployment-name-heading">Name this deployment</h3><p>Use a name that will make this deployment easy to find later.</p></div><box-text-field ref={nameRef} label="Name this deployment" value={deploymentName} placeholder="For example, Northstar CLM rollout" required invalid={deploymentName.length > 0 && !nameIsValid} errorMessage={nameLength > 80 ? 'Use 80 characters or fewer.' : undefined}></box-text-field></section>
      <div className="solution-list" role="radiogroup" aria-label="Solutions">{templates.map((template) => <button className={`solution-option ${template.id === selectedTemplateID ? 'selected' : ''}`} type="button" role="radio" aria-checked={template.id === selectedTemplateID} key={template.id} onClick={() => onTemplateChange(template.id)} disabled={assembling}><span className="choice-marker" aria-hidden="true">{template.id === selectedTemplateID ? '✓' : ''}</span><span><strong>{template.name}</strong><small>{template.description}</small></span><span className="template-sector">{template.sector || 'Solution'}</span></button>)}</div>
      <section className="system-selection" aria-labelledby="systems-title"><header><h3 id="systems-title">Systems</h3><p>Box is required. Add Salesforce when the solution includes CRM metadata.</p></header><div className="system-grid"><div className="system-option required"><SystemProvider provider="box" name="Box" description="Content platform"/><ProviderSwitch checked disabled label="Box" description="Required"/><em>Required</em></div><div className="system-option"><SystemProvider provider="salesforce" name="Salesforce" description="CRM records and workflows"/><ProviderSwitch checked={selectedComponents.includes('salesforce')} disabled={assembling} label="Salesforce" description="Optional" onToggle={onToggleSalesforce}/><em>Optional</em></div><div className="system-option unavailable" aria-disabled="true"><SystemProvider provider="databricks" name="Databricks" description="Data intelligence"/><ProviderSwitch checked={false} disabled label="Databricks" description="Coming soon"/><em>Coming soon</em></div><div className="system-option unavailable" aria-disabled="true"><SystemProvider provider="amazon bedrock" name="Amazon Bedrock" description="AgentCore integration"/><ProviderSwitch checked={false} disabled label="Amazon Bedrock" description="Coming soon"/><em>Coming soon</em></div></div></section>
      <footer className="choose-actions"><p className="notice" role="status">{notice}</p><div className="stage-navigation"><box-button label={assembling ? 'Preparing package…' : 'Prepare package'} tone="primary" disabled={assembling || !nameIsValid} onClick={onAssemble}></box-button></div></footer>
    </article>
  </section>
}
