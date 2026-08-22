import { useEffect, useRef, useState } from 'react'
import type { BoxConnectionInput, RunDiagnostic, SalesforceConnectionOption } from '../types'

type ValueElement = HTMLElement & { value: string }

function useDrawerClose(onClose: () => void) {
  const ref = useRef<HTMLElement>(null)
  useEffect(() => {
    const drawer = ref.current
    if (!drawer) return
    const handleOpenChanged = (event: Event) => {
      if (!(event as CustomEvent<{ open: boolean }>).detail.open) onClose()
    }
    drawer.addEventListener('open-changed', handleOpenChanged)
    return () => drawer.removeEventListener('open-changed', handleOpenChanged)
  }, [onClose])
  return ref
}

function useValueChanged(onChange: (value: string) => void) {
  const ref = useRef<ValueElement>(null)
  useEffect(() => {
    const field = ref.current
    if (!field) return
    const handleValueChanged = (event: Event) => onChange((event as CustomEvent<{ value: string }>).detail.value)
    field.addEventListener('value-changed', handleValueChanged)
    return () => field.removeEventListener('value-changed', handleValueChanged)
  }, [onChange])
  return ref
}

export function DiagnosticsDrawer({ diagnostic, onClose }: { diagnostic: RunDiagnostic | null; onClose: () => void }) {
  const drawerRef = useDrawerClose(onClose)
  return <box-drawer ref={drawerRef} open heading={diagnostic?.title ?? 'Loading diagnostic guidance'} description={diagnostic?.summary ?? 'Dispatch is preparing safe next steps for this failed run.'} position="right">
    <section className="drawer-content">
      {diagnostic && <><h3>Recommended next steps</h3><ol>{diagnostic.nextSteps.map((step) => <li key={step}>{step}</li>)}</ol><div className="cli-hint"><strong>Full provider detail</strong><p>{diagnostic.cliHint}</p></div></>}
    </section>
  </box-drawer>
}

export function SalesforceConnectionDrawer({ options, selectedAlias, loading, onChange, onSave, onClose }: { options: SalesforceConnectionOption[]; selectedAlias: string; loading: boolean; onChange: (alias: string) => void; onSave: () => void; onClose: () => void }) {
  const drawerRef = useDrawerClose(onClose)
  const selectRef = useValueChanged(onChange)
  const selectOptions = options.map((option) => ({ label: `${option.alias} · ${option.kind}${option.expiresAt ? ` · expires ${option.expiresAt}` : ''}`, value: option.alias }))
  return <box-drawer ref={drawerRef} open heading="Salesforce connection" description="Choose an org already authenticated in Salesforce CLI. Dispatch will recheck it before deployment." position="right">
    <section className="drawer-content">
      {loading && options.length === 0 ? <p>Loading authenticated orgs…</p> : options.length === 0 ? <div className="cli-hint"><strong>No authenticated aliases found</strong><p>Authenticate an org and give it an alias with Salesforce CLI, then reopen this panel.</p></div> : <>
        <box-select ref={selectRef} label="Authenticated org" description="Only aliases, status, and scratch-org expiration are shown. Credentials stay in Salesforce CLI." value={selectedAlias} options={selectOptions} disabled={loading}></box-select>
        <footer className="drawer-actions"><box-button label={loading ? 'Saving…' : 'Use this Salesforce org'} tone="primary" disabled={loading || !selectedAlias} onClick={onSave}></box-button></footer>
      </>}
    </section>
  </box-drawer>
}

export function BoxConnectionDrawer({ loading, onSave, onClose }: { loading: boolean; onSave: (input: BoxConnectionInput) => void; onClose: () => void }) {
  const drawerRef = useDrawerClose(onClose)
  const [input, setInput] = useState<BoxConnectionInput>({ clientId: '', clientSecret: '', subjectType: 'user', subjectId: '' })
  const set = (field: keyof BoxConnectionInput) => (value: string) => setInput((current) => ({ ...current, [field]: value }))
  const clientIDRef = useValueChanged(set('clientId'))
  const clientSecretRef = useValueChanged(set('clientSecret'))
  const subjectTypeRef = useValueChanged(set('subjectType'))
  const subjectIDRef = useValueChanged(set('subjectId'))
  const canSave = input.clientId.trim() !== '' && input.clientSecret.trim() !== '' && input.subjectId.trim() !== ''
  return <box-drawer ref={drawerRef} open heading="Connect Box" description="Set up the Client Credentials Grant connection used by Dispatch. Secrets go only to the local Dispatch API and are never returned to the browser." position="right">
    <section className="drawer-content box-connection-form">
      <box-text-field ref={clientIDRef} label="Client ID" value={input.clientId} required></box-text-field>
      <box-text-field ref={clientSecretRef} label="Client secret" value={input.clientSecret} type="password" required></box-text-field>
      <box-select ref={subjectTypeRef} label="Subject type" value={input.subjectType} options={[{ label: 'User', value: 'user' }, { label: 'Enterprise', value: 'enterprise' }]} required></box-select>
      <box-text-field ref={subjectIDRef} label="Subject ID" value={input.subjectId} required></box-text-field>
      <footer className="drawer-actions"><box-button label="Cancel" tone="secondary" onClick={onClose}></box-button><box-button label={loading ? 'Saving…' : 'Save Box connection'} tone="primary" disabled={!canSave || loading} onClick={() => { if (canSave) onSave(input) }}></box-button></footer>
    </section>
  </box-drawer>
}
