import { useEffect, useRef } from 'react'
import type { SolutionTemplate } from '../types'

type SwitchElement = HTMLElement & { checked: boolean }

function ProviderSwitch({ checked, disabled = false, label, description, onToggle }: { checked: boolean; disabled?: boolean; label: string; description: string; onToggle?: () => void }) {
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

export function ChoosePage({ templates, selectedTemplateID, selectedComponents, assembling, notice, onTemplateChange, onToggleSalesforce, onAssemble }: { templates: SolutionTemplate[]; selectedTemplateID: string; selectedComponents: string[]; assembling: boolean; notice: string; onTemplateChange: (templateID: string) => void; onToggleSalesforce: () => void; onAssemble: () => void }) {
  return <section className="choose-layout choose-stage" aria-label="Choose deployment"><article className="task-surface choose-card"><header className="task-heading"><div><h2>Choose a solution</h2><p>Select the package Dispatch should prepare locally. You will review every change before anything is deployed.</p></div></header><div className="solution-list" role="radiogroup" aria-label="Solutions">{templates.map((template) => <button className={`solution-option ${template.id === selectedTemplateID ? 'selected' : ''}`} type="button" role="radio" aria-checked={template.id === selectedTemplateID} key={template.id} onClick={() => onTemplateChange(template.id)} disabled={assembling}><span className="choice-marker" aria-hidden="true">{template.id === selectedTemplateID ? '✓' : ''}</span><span><strong>{template.name}</strong><small>{template.description}</small></span><span className="template-sector">{template.sector || 'Solution'}</span></button>)}</div><section className="system-selection" aria-labelledby="systems-title"><header><h3 id="systems-title">Choose systems</h3><p>Box is required. Add only the systems this solution needs.</p></header><div className="system-grid"><div className="system-option required"><ProviderSwitch checked disabled label="Box" description="Required content platform" /><em>Required</em></div><div className="system-option"><ProviderSwitch checked={selectedComponents.includes('salesforce')} disabled={assembling} label="Salesforce" description="CRM records and workflows" onToggle={onToggleSalesforce} /><em>Optional</em></div><div className="system-option unavailable" aria-disabled="true"><span className="system-placeholder" aria-hidden="true"></span><span><strong>Databricks</strong><small>Data intelligence integration</small></span><em>Coming soon</em></div><div className="system-option unavailable" aria-disabled="true"><span className="system-placeholder" aria-hidden="true"></span><span><strong>Amazon Bedrock</strong><small>AgentCore integration</small></span><em>Coming soon</em></div></div></section><footer className="choose-actions"><p className="notice" role="status">{notice}</p><div className="stage-navigation"><box-button label="Back" tone="secondary" disabled></box-button><box-button label={assembling ? 'Preparing package…' : 'Next'} tone="primary" disabled={assembling} onClick={onAssemble}></box-button></div></footer></article></section>
}
