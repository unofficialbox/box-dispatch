import { useEffect, useRef, useState } from 'react'
import type { ConnectionSummary, RunDiagnostic, SalesforceOAuthJob, ScratchOrgJob, BoxOAuthJob } from '../types'
import { ProviderLogo } from './ProviderLogo'

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

function RemoveConnectionButton({ label, disabled = false, onPress }: { label: string; disabled?: boolean; onPress: () => void }) {
  const ref = useRef<HTMLElement>(null)
  const handlerRef = useRef(onPress)
  useEffect(() => { handlerRef.current = onPress }, [onPress])
  useEffect(() => {
    const button = ref.current
    if (!button) return
    const handleClick = (event: Event) => {
      event.preventDefault()
      if (!disabled) handlerRef.current()
    }
    button.addEventListener('click', handleClick)
    return () => button.removeEventListener('click', handleClick)
  }, [disabled])
  return <box-icon-button ref={ref} className="connection-remove" icon="cart-1" label={label} disabled={disabled}></box-icon-button>
}

function DrawerButton({ label, tone = 'neutral', disabled = false, onPress }: { label: string; tone?: 'primary' | 'neutral' | 'danger'; disabled?: boolean; onPress: () => void }) {
  return <box-button label={label} tone={tone} disabled={disabled} onClick={(event) => {
    event.preventDefault()
    if (!disabled) onPress()
  }}></box-button>
}

function DrawerSwitch({ checked, label, description, disabled = false, onChange }: { checked: boolean; label: string; description: string; disabled?: boolean; onChange: (checked: boolean) => void }) {
  const ref = useRef<HTMLElement>(null)
  useEffect(() => {
    const element = ref.current
    if (!element) return
    const handleChange = (event: Event) => onChange((event as CustomEvent<{ checked: boolean }>).detail.checked)
    element.addEventListener('checked-changed', handleChange)
    return () => element.removeEventListener('checked-changed', handleChange)
  }, [onChange])
  return <box-switch ref={ref} checked={checked} disabled={disabled} label={label} description={description}></box-switch>
}

type DrawerElement = HTMLElement & { close: () => void }

export function DiagnosticsDrawer({ diagnostic, onClose }: { diagnostic: RunDiagnostic | null; onClose: () => void }) {
  const drawerRef = useDrawerClose(onClose)
  return <box-drawer ref={drawerRef} className="diagnostic-drawer" open heading={diagnostic?.title ?? 'Loading diagnostic guidance'} description={diagnostic?.summary ?? 'Dispatch is preparing safe next steps for this failed run.'} position="right" size="medium" busy={!diagnostic}>
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
          <pre>{diagnostic.technicalDetail}</pre>
        </details>}
      </>}
    </section>
  </box-drawer>
}

export function SalesforceConnectionDrawer({ connection, loading, error, oauthJob, scratchJob, onLogin, onSelect, onRemove, onOpen, onCreateScratch, onClose }: { connection?: ConnectionSummary; loading: boolean; error: string; oauthJob: SalesforceOAuthJob | null; scratchJob: ScratchOrgJob | null; onLogin: (loginHost: 'production' | 'sandbox', role: 'org' | 'devhub') => Promise<boolean>; onSelect: (id: string) => Promise<boolean>; onRemove: (id: string) => Promise<boolean>; onOpen: () => void; onCreateScratch: (alias: string, installManagedPackage: boolean) => void; onClose: () => void }) {
  const drawerRef = useDrawerClose(onClose)
  const closeDrawer = () => (drawerRef.current as DrawerElement | null)?.close()
  const [loginHost, setLoginHost] = useState<'production' | 'sandbox'>('production')
  const [alias, setAlias] = useState('')
  const [installManagedPackage, setInstallManagedPackage] = useState(true)
  const [addMode, setAddMode] = useState<'existing' | 'scratch' | null>((connection?.orgs?.length ?? 0) > 0 ? null : 'existing')
  const loginHostRef = useValueChanged((value) => setLoginHost(value === 'sandbox' ? 'sandbox' : 'production'))
  const aliasRef = useValueChanged(setAlias)
  const orgs = connection?.orgs ?? []
  const selected = orgs.find((org) => org.selected) ?? orgs[0]
  const loggingIn = oauthJob?.status === 'pending'
  const creating = scratchJob?.status === 'queued' || scratchJob?.status === 'creating'
  const preparing = scratchJob?.status === 'preparing'
  const canCreateScratch = Boolean(connection?.devHubConfigured)
  return <box-drawer ref={drawerRef} className="connection-drawer" open heading="Salesforce connections" position="right" size="large" busy={loading}>
    <section className="drawer-content salesforce-environments">
      {error && <div className="drawer-inline-error" role="alert"><strong>Salesforce connection needs attention</strong><p>{error}</p></div>}
      {orgs.length > 0 && <section className="saved-connection-summary salesforce-current-org" aria-label="Connected Salesforce orgs">
        <header className="connection-section-heading current-environment-heading"><ProviderLogo provider="salesforce" size="standard"/><div><span className="eyebrow">Selected environment</span><h3>{selected?.alias || selected?.username || 'Salesforce org'}</h3></div><RemoveConnectionButton label="Remove selected Salesforce org" disabled={loading || !selected?.id} onPress={() => { if (selected?.id) void onRemove(selected.id) }}/></header>
        {selected && <div className="connection-identity"><span className={`connection-state-dot ${connection?.verified ? 'ready' : ''}`} aria-hidden="true"></span><span>{connection?.verified ? 'Verified' : 'Not verified'} as {selected.username || selected.alias || 'the selected Salesforce user'}</span></div>}
        {selected && <dl className="selected-environment-details">
          <div><dt>Org ID</dt><dd>{selected.orgId || 'Not reported'}</dd></div>
          <div><dt>Org type</dt><dd>{selected.kind || 'Org'}</dd></div>
        </dl>}
        <div className="saved-connection-actions"><DrawerButton label="Open org" disabled={loading || !connection?.launchUrl} onPress={onOpen}/></div>
      </section>}
      <section className="drawer-section connection-add-section">
        <div><h3>Add a Salesforce environment</h3><p>Use an existing org or create a temporary org for this deployment.</p></div>
        <div className="connection-mode-picker" role="group" aria-label="Salesforce connection type">
          <button type="button" className={`connection-mode-card ${addMode === 'existing' ? 'selected' : ''}`} aria-pressed={addMode === 'existing'} disabled={loading} onClick={() => setAddMode(addMode === 'existing' ? null : 'existing')}><span className="connection-mode-icon">↗</span><span><strong>Existing org</strong><small>Connect production or sandbox.</small></span></button>
          <button type="button" className={`connection-mode-card ${addMode === 'scratch' ? 'selected' : ''}`} aria-pressed={addMode === 'scratch'} disabled={loading} onClick={() => setAddMode(addMode === 'scratch' ? null : 'scratch')}><span className="connection-mode-icon">＋</span><span><strong>Scratch org</strong><small>Create from your Dev Hub.</small></span></button>
        </div>
      </section>
      {addMode === 'existing' && <section className="drawer-section connection-mode-panel">
        <div><h3>Connect an existing org</h3><p>Choose where Salesforce should authenticate, then connect either a working org or a Dev Hub.</p></div>
        <div className="drawer-button-row">
          <DrawerButton label={loggingIn && oauthJob?.role !== 'devhub' ? 'Waiting for Salesforce…' : 'Connect org'} tone="primary" disabled={loading || loggingIn} onPress={() => { void onLogin(loginHost, 'org') }}/>
          <DrawerButton label={loggingIn && oauthJob?.role === 'devhub' ? 'Waiting for Salesforce…' : 'Connect Dev Hub'} disabled={loading || loggingIn} onPress={() => { void onLogin(loginHost, 'devhub') }}/>
        </div>
        <box-select ref={loginHostRef} label="Sign-in endpoint" value={loginHost} options={[{ label: 'Production', value: 'production' }, { label: 'Sandbox', value: 'sandbox' }]} required></box-select>
        {oauthJob?.status === 'pending' && <p className="scratch-job scratch-job-queued" role="status" aria-live="polite">{oauthJob.message}</p>}
        {orgs.length > 0 && <div className="connected-environments">
          <h4>Connected environments</h4>
          <ul className="connection-option-list">
            {orgs.map((org) => <li key={org.id || `${org.alias}-${org.username}`} className={org.selected ? 'selected' : ''}>
              <button type="button" className="connection-option-main" aria-pressed={org.selected} disabled={loading || org.selected || org.devHub || !org.id} onClick={() => { if (org.id) void onSelect(org.id) }}>
                <span className={`connection-state-dot ${org.status === 'Ready' ? 'ready' : ''}`} aria-hidden="true"></span>
                <span className="connection-option-copy"><strong>{org.alias || org.username || 'Salesforce org'}</strong><small>{[org.kind, org.username, org.orgId].filter(Boolean).join(' · ')}</small></span>
                <span className="connection-option-badge">{org.selected ? 'Selected' : org.devHub ? 'Dev Hub' : 'Use org'}</span>
              </button>
              <RemoveConnectionButton label={`Remove ${org.alias || org.username || 'Salesforce org'}`} disabled={loading || !org.id} onPress={() => { if (org.id) void onRemove(org.id) }}/>
            </li>)}
          </ul>
        </div>}
      </section>}
      {addMode === 'scratch' && <section className="drawer-section connection-mode-panel scratch-org-section">
        <div><h3>Create a scratch org</h3><p>{canCreateScratch ? 'Dispatch creates, selects, and verifies a 30-day org from your Dev Hub.' : 'Connect a Dev Hub before creating a scratch org.'}</p></div>
        {canCreateScratch
          ? <>
              <box-text-field className="scratch-org-alias" ref={aliasRef} label="Scratch org alias" value={alias}></box-text-field>
              <div className="scratch-package-option"><DrawerSwitch checked={installManagedPackage} disabled={creating || preparing} onChange={setInstallManagedPackage} label="Install Box for Salesforce automatically" description="Dispatch checks version 5.43 first, then installs it only when needed."/></div>
              <DrawerButton label={creating ? 'Creating scratch org…' : 'Create and use scratch org'} tone="primary" disabled={loading || creating || preparing} onPress={() => onCreateScratch(alias, installManagedPackage)}/>
              {scratchJob && scratchJob.status !== 'failed' && <div className={`scratch-job scratch-job-${scratchJob.status}`} role="status" aria-live="polite"><strong>{scratchJob.message}</strong>{scratchJob.packageMessage && <span>{scratchJob.packageMessage}</span>}{scratchJob.packageRequestId && <small>Salesforce request {scratchJob.packageRequestId}</small>}</div>}
            </>
          : <DrawerButton label="Connect a Dev Hub" tone="primary" disabled={loading || loggingIn} onPress={() => { void onLogin(loginHost, 'devhub') }}/>
        }
      </section>}
    </section>
    <footer slot="footer" className="drawer-actions"><DrawerButton label="Close" onPress={closeDrawer}/></footer>
  </box-drawer>
}

export function BoxConnectionDrawer({ connection, loading, error, oauthJob, onLogin, onSelect, onRemove, onVerify, onOpen, onClose }: { connection?: ConnectionSummary; loading: boolean; error: string; oauthJob: BoxOAuthJob | null; onLogin: () => Promise<boolean>; onSelect: (id: string) => Promise<boolean>; onRemove: (id: string) => Promise<boolean>; onVerify: () => Promise<boolean>; onOpen: () => void; onClose: () => void }) {
  const drawerRef = useDrawerClose(onClose)
  const closeDrawer = () => (drawerRef.current as DrawerElement | null)?.close()
  const apps = connection?.connections ?? []
  const selected = apps.find((app) => app.selected) ?? apps[0]
  const appSelectRef = useValueChanged((value) => { if (value && value !== selected?.id) void onSelect(value) })
  const loggingIn = oauthJob?.status === 'pending'
  const canLogin = Boolean(connection?.oauthConfigured)
  return <box-drawer ref={drawerRef} className="connection-drawer" open heading="Connect Box" position="right" size="large" busy={loading}>
    <section className="drawer-content box-connection-form">
      {error && <div className="drawer-inline-error" role="alert"><strong>Box connection needs attention</strong><p>{error}</p></div>}
      {apps.length > 0 && <section className="saved-connection-summary" aria-label="Connected Box users">
        <h3>Connected users</h3>
        <div className="saved-connection-picker">
          <box-select ref={appSelectRef} label="Box connection" value={selected?.id ?? ''} options={apps.map((app) => ({ label: app.alias || app.identity || 'Box', value: app.id ?? '' }))}></box-select>
          <RemoveConnectionButton label="Remove Box connection" disabled={loading || !selected?.id} onPress={() => { if (selected?.id) void onRemove(selected.id) }}/>
        </div>
        {selected && <p>Connected as {selected.identity || selected.alias || 'the selected Box user'}.</p>}
        <div className="saved-connection-actions"><DrawerButton label="Open" disabled={loading || !connection?.launchUrl} onPress={onOpen}/><DrawerButton label={loading ? 'Checking…' : 'Check availability'} tone="primary" disabled={loading} onPress={() => { void onVerify() }}/></div>
      </section>}
      <section className="drawer-section">
        <h3>Log in with Box</h3>
        {!canLogin && <p>Set BOX_CLIENT_ID and BOX_CLIENT_SECRET in .env, then restart Dispatch.</p>}
        <div className="drawer-button-row">
          <DrawerButton label={loggingIn ? 'Waiting for Box…' : apps.length > 0 ? 'Add another Box user' : 'Log in with Box'} tone="primary" disabled={loading || loggingIn || !canLogin} onPress={() => { void onLogin() }}/>
        </div>
        {oauthJob && oauthJob.status !== 'failed' && <p className={`scratch-job scratch-job-${oauthJob.status === 'pending' ? 'queued' : oauthJob.status}`} role="status" aria-live="polite">{oauthJob.message}</p>}
      </section>
    </section>
    <footer slot="footer" className="drawer-actions"><DrawerButton label="Close" onPress={closeDrawer}/></footer>
  </box-drawer>
}
