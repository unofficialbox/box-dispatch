import { useEffect, useRef, useState } from 'react'
import type { ConnectionSummary, RunDiagnostic, SalesforceOAuthJob, ScratchOrgJob, BoxOAuthJob } from '../types'

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

function DrawerButton({ label, tone = 'secondary', disabled = false, onPress }: { label: string; tone?: 'primary' | 'secondary' | 'danger'; disabled?: boolean; onPress: () => void }) {
  const ref = useRef<HTMLButtonElement>(null)
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
  return <button ref={ref} type="button" className={`drawer-button drawer-button-${tone}`} disabled={disabled}>{label}</button>
}

type DrawerElement = HTMLElement & { close: () => void }

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
          <pre>{diagnostic.technicalDetail}</pre>
        </details>}
      </>}
    </section>
  </box-drawer>
}

export function SalesforceConnectionDrawer({ connection, loading, error, oauthJob, scratchJob, onLogin, onSelect, onRemove, onCheck, onCreateScratch, onClose }: { connection?: ConnectionSummary; loading: boolean; error: string; oauthJob: SalesforceOAuthJob | null; scratchJob: ScratchOrgJob | null; onLogin: (loginHost: 'production' | 'sandbox', role: 'org' | 'devhub') => Promise<boolean>; onSelect: (id: string) => Promise<boolean>; onRemove: (id: string) => Promise<boolean>; onCheck: () => Promise<boolean>; onCreateScratch: (alias: string) => void; onClose: () => void }) {
  const drawerRef = useDrawerClose(onClose)
  const closeDrawer = () => (drawerRef.current as DrawerElement | null)?.close()
  const [loginHost, setLoginHost] = useState<'production' | 'sandbox'>('production')
  const [alias, setAlias] = useState('')
  const loginHostRef = useValueChanged((value) => setLoginHost(value === 'sandbox' ? 'sandbox' : 'production'))
  const aliasRef = useValueChanged(setAlias)
  const orgs = connection?.orgs ?? []
  const selected = orgs.find((org) => org.selected) ?? orgs[0]
  const orgSelectRef = useValueChanged((value) => { if (value && value !== selected?.id) void onSelect(value) })
  const loggingIn = oauthJob?.status === 'pending'
  const creating = scratchJob?.status === 'queued' || scratchJob?.status === 'creating'
  const canCheck = Boolean(connection?.restConfigured)
  const canCreateScratch = Boolean(connection?.devHubConfigured)
  return <box-drawer ref={drawerRef} className="connection-drawer" open heading="Connect Salesforce" position="right" size="large" busy={loading}>
    <section className="drawer-content salesforce-environments">
      {error && <div className="drawer-inline-error" role="alert"><strong>Salesforce connection needs attention</strong><p>{error}</p></div>}
      {orgs.length > 0 && <section className="saved-connection-summary" aria-label="Connected Salesforce orgs">
        <h3>Connected orgs</h3>
        <div className="saved-connection-picker">
          <box-select ref={orgSelectRef} label="Salesforce org" value={selected?.id ?? ''} options={orgs.map((org) => ({ label: org.devHub ? `${org.alias || org.username || 'Salesforce org'} (Dev Hub)` : org.alias || org.username || 'Salesforce org', value: org.id ?? '' }))}></box-select>
          <RemoveConnectionButton label="Remove Salesforce org" disabled={loading || !selected?.id} onPress={() => { if (selected?.id) void onRemove(selected.id) }}/>
        </div>
        {selected && <p>Connected as {selected.username || selected.alias || 'the selected Salesforce user'}.</p>}
        <div className="saved-connection-actions"><DrawerButton label={loading ? 'Checking…' : 'Check availability'} tone="primary" disabled={loading || !canCheck} onPress={() => { void onCheck() }}/></div>
      </section>}
      <section className="drawer-section">
        <h3>Log in with Salesforce</h3>
        <box-select ref={loginHostRef} label="Salesforce environment" value={loginHost} options={[{ label: 'Production', value: 'production' }, { label: 'Sandbox', value: 'sandbox' }]} required></box-select>
        <div className="drawer-button-row">
          <DrawerButton label={loggingIn && oauthJob?.role !== 'devhub' ? 'Waiting for Salesforce…' : orgs.length > 0 ? 'Add another Salesforce org' : 'Log in with Salesforce'} tone="primary" disabled={loading || loggingIn} onPress={() => { void onLogin(loginHost, 'org') }}/>
          <DrawerButton label={loggingIn && oauthJob?.role === 'devhub' ? 'Waiting for Salesforce…' : 'Log in as Dev Hub'} disabled={loading || loggingIn} onPress={() => { void onLogin(loginHost, 'devhub') }}/>
        </div>
        {oauthJob && oauthJob.status !== 'failed' && <p className={`scratch-job scratch-job-${oauthJob.status === 'pending' ? 'queued' : oauthJob.status}`} role="status" aria-live="polite">{oauthJob.message}</p>}
      </section>
      <section className="drawer-section scratch-org-section">
        <h3>Create a scratch org</h3>
        <box-text-field className="scratch-org-alias" ref={aliasRef} label="Scratch org alias" value={alias}></box-text-field>
        <DrawerButton label={creating ? 'Creating scratch org…' : 'Create and use scratch org'} tone="primary" disabled={loading || creating || !canCreateScratch} onPress={() => onCreateScratch(alias)}/>
        {scratchJob && scratchJob.status !== 'failed' && <p className={`scratch-job scratch-job-${scratchJob.status}`} role="status" aria-live="polite">{scratchJob.message}{scratchJob.expirationDate ? ` Expires ${scratchJob.expirationDate}.` : ''}</p>}
      </section>
    </section>
    <footer slot="footer" className="drawer-actions"><DrawerButton label="Close" onPress={closeDrawer}/></footer>
  </box-drawer>
}

export function BoxConnectionDrawer({ connection, loading, error, oauthJob, onLogin, onSelect, onRemove, onVerify, onClose }: { connection?: ConnectionSummary; loading: boolean; error: string; oauthJob: BoxOAuthJob | null; onLogin: () => Promise<boolean>; onSelect: (id: string) => Promise<boolean>; onRemove: (id: string) => Promise<boolean>; onVerify: () => Promise<boolean>; onClose: () => void }) {
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
        <div className="saved-connection-actions"><DrawerButton label={loading ? 'Checking…' : 'Check availability'} tone="primary" disabled={loading} onPress={() => { void onVerify() }}/></div>
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
