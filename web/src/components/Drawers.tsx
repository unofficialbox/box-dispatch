import { useEffect, useRef, useState } from 'react'
import type { BoxConnectionInput, RunDiagnostic, SalesforceConnectionOption, SalesforceRESTInput, ScratchOrgJob } from '../types'

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
  return <box-drawer ref={drawerRef} open heading={diagnostic?.title ?? 'Loading diagnostic guidance'} description={diagnostic?.summary ?? 'Dispatch is preparing safe next steps for this failed run.'} position="right" size="large" busy={!diagnostic}>
    <section className="drawer-content">
      {diagnostic && <>
        {(diagnostic.provider || diagnostic.code) && <dl className="diagnostic-summary">
          {diagnostic.provider && <div><dt>Provider</dt><dd>{diagnostic.provider}</dd></div>}
          {diagnostic.code && <div><dt>Error code</dt><dd><code>{diagnostic.code}</code></dd></div>}
        </dl>}
        <h3>Recommended next steps</h3>
        <ol>{diagnostic.nextSteps.map((step) => <li key={step}>{step}</li>)}</ol>
        {diagnostic.technicalDetail && <details className="diagnostic-detail">
          <summary>Technical details</summary>
          <p>This provider response has been sanitized so it is safe to inspect in the browser.</p>
          <pre>{diagnostic.technicalDetail}</pre>
        </details>}
      </>}
    </section>
  </box-drawer>
}

export function SalesforceConnectionDrawer({ options, selectedAlias, loading, scratchJob, onChange, onSave, onSaveREST, onCheck, onCreateScratch, onClose }: { options: SalesforceConnectionOption[]; selectedAlias: string; loading: boolean; scratchJob: ScratchOrgJob | null; onChange: (alias: string) => void; onSave: () => void; onSaveREST: (input: SalesforceRESTInput) => void; onCheck: () => void; onCreateScratch: (alias: string) => void; onClose: () => void }) {
  const drawerRef = useDrawerClose(onClose)
  const selectRef = useValueChanged(onChange)
  const selectOptions = options.map((option) => ({ label: `${option.alias} · ${option.kind}${option.expiresAt ? ` · expires ${option.expiresAt}` : ''}`, value: option.alias }))
  const [input, setInput] = useState<SalesforceRESTInput>({ instanceUrl: '', accessToken: '', devHubUrl: '', devHubAccessToken: '', clientId: '', clientSecret: '' })
  const [alias, setAlias] = useState('')
  const set = (field: keyof SalesforceRESTInput) => (value: string) => setInput((current) => ({ ...current, [field]: value }))
  const instanceURLRef = useValueChanged(set('instanceUrl'))
  const accessTokenRef = useValueChanged(set('accessToken'))
  const devHubURLRef = useValueChanged(set('devHubUrl'))
  const devHubTokenRef = useValueChanged(set('devHubAccessToken'))
  const clientIDRef = useValueChanged(set('clientId'))
  const clientSecretRef = useValueChanged(set('clientSecret'))
  const aliasRef = useValueChanged(setAlias)
  const hasTarget = input.instanceUrl.trim() !== '' && input.accessToken.trim() !== ''
  const hasDevHub = input.devHubUrl.trim() !== '' && input.devHubAccessToken.trim() !== '' && input.clientId.trim() !== ''
  const canSaveREST = hasTarget || hasDevHub
  const creating = scratchJob?.status === 'queued' || scratchJob?.status === 'creating'
  const selectedOption = options.find((option) => option.alias === selectedAlias)
  return <box-drawer ref={drawerRef} open heading="Salesforce environments" description="Check the selected org or create a replacement scratch org. Salesforce credentials stay in the local Go service." position="right" size="large" busy={loading}>
    <section className="drawer-content salesforce-environments">
      <section className="drawer-section"><div className="drawer-section-heading"><div><h3>Selected org</h3><p>{selectedAlias || 'No Salesforce org selected'}</p></div><box-button label={loading ? 'Checking…' : 'Check availability'} tone="secondary" disabled={loading || !selectedAlias} onClick={onCheck}></box-button></div>
        <box-select ref={selectRef} label="Available connection" value={selectedAlias} options={selectOptions} disabled={loading} loading={loading} emptyText="No saved Salesforce environments"></box-select>{selectedOption && !selectedOption.selected && <box-button label="Use selected org" tone="secondary" disabled={loading || !selectedAlias} onClick={onSave}></box-button>}
      </section>
      <section className="drawer-section"><h3>Create a scratch org</h3><p>Dispatch asks the saved Dev Hub to create a 30-day Developer scratch org, then selects it automatically.</p><box-text-field ref={aliasRef} label="Scratch org alias" description="Optional; Dispatch generates one when blank." value={alias}></box-text-field><box-button label={creating ? 'Creating scratch org…' : 'Create and use scratch org'} tone="primary" disabled={loading || creating} onClick={() => onCreateScratch(alias)}></box-button>{scratchJob && <p className={`scratch-job scratch-job-${scratchJob.status}`} role="status">{scratchJob.message}{scratchJob.expirationDate ? ` Expires ${scratchJob.expirationDate}.` : ''}</p>}</section>
      <details className="drawer-section connection-setup"><summary>Dev Hub and REST connection</summary><p>Use a Dev Hub access token and Connected App client ID. A current-org token is optional until a scratch org is created.</p><box-text-field ref={instanceURLRef} label="Current org URL" value={input.instanceUrl} autocomplete="url"></box-text-field><box-text-field ref={accessTokenRef} label="Current org access token" type="password" value={input.accessToken} autocomplete="off" reveal></box-text-field><box-text-field ref={devHubURLRef} label="Dev Hub URL" required value={input.devHubUrl} autocomplete="url"></box-text-field><box-text-field ref={devHubTokenRef} label="Dev Hub access token" type="password" required value={input.devHubAccessToken} autocomplete="off" reveal></box-text-field><box-text-field ref={clientIDRef} label="Connected App client ID" required value={input.clientId} autocomplete="off"></box-text-field><box-text-field ref={clientSecretRef} label="Connected App client secret" type="password" value={input.clientSecret} autocomplete="off" reveal></box-text-field><box-button label={loading ? 'Saving…' : 'Save REST connection'} tone="primary" disabled={loading || !canSaveREST} onClick={() => onSaveREST(input)}></box-button></details>
    </section>
  </box-drawer>
}

export function BoxConnectionDrawer({ loading, error, onSave, onClose }: { loading: boolean; error: string; onSave: (input: BoxConnectionInput) => void; onClose: () => void }) {
  const drawerRef = useDrawerClose(onClose)
  const [input, setInput] = useState<BoxConnectionInput>({ clientId: '', clientSecret: '', subjectType: 'user', subjectId: '' })
  const set = (field: keyof BoxConnectionInput) => (value: string) => setInput((current) => ({ ...current, [field]: value }))
  const clientIDRef = useValueChanged(set('clientId'))
  const clientSecretRef = useValueChanged(set('clientSecret'))
  const subjectTypeRef = useValueChanged(set('subjectType'))
  const subjectIDRef = useValueChanged(set('subjectId'))
  const canSave = input.clientId.trim() !== '' && input.clientSecret.trim() !== '' && input.subjectId.trim() !== ''
  return <box-drawer ref={drawerRef} open heading="Connect Box" description="Set up the Client Credentials Grant connection used by Dispatch. Secrets go only to the local Dispatch service and are never returned to the browser." position="right" size="large" busy={loading}>
    <section className="drawer-content box-connection-form">
      {error && <div className="drawer-inline-error" role="alert"><strong>Connection not saved</strong><p>{error}</p></div>}
      <box-text-field ref={clientIDRef} label="Client ID" value={input.clientId} required autocomplete="off"></box-text-field>
      <box-text-field ref={clientSecretRef} label="Client secret" value={input.clientSecret} type="password" required autocomplete="off" reveal></box-text-field>
      <box-select ref={subjectTypeRef} label="Subject type" value={input.subjectType} options={[{ label: 'User', value: 'user' }, { label: 'Enterprise', value: 'enterprise' }]} required></box-select>
      <box-text-field ref={subjectIDRef} label="Subject ID" value={input.subjectId} required></box-text-field>
    </section>
    <footer slot="footer" className="drawer-actions"><box-button label="Cancel" tone="secondary" onClick={onClose}></box-button><box-button label={loading ? 'Saving…' : 'Save Box connection'} tone="primary" disabled={!canSave || loading} onClick={() => { if (canSave) onSave(input) }}></box-button></footer>
  </box-drawer>
}
