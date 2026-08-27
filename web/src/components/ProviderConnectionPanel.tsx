import { useEffect, useRef, type ReactNode } from 'react'
import { ProviderLogo } from './ProviderLogo'

type ProviderConnectionPanelProps = {
  provider: string
  title: string
  count: number
  onManage?: () => void
  actions?: ReactNode
  compact?: boolean
  children: ReactNode
}

export function ProviderConnectionPanel({ provider, title, count, onManage, actions, compact = false, children }: ProviderConnectionPanelProps) {
  return <section className={`settings-provider provider-connection-panel${compact ? ' provider-connection-panel--compact' : ''}`} aria-label={title}>
    <header>
      <div className="settings-provider-title"><ProviderLogo provider={provider} size="standard"/><div><h3>{title}</h3><p>{count} saved {count === 1 ? 'connection' : 'connections'}</p></div></div>
      {actions ?? (onManage ? <box-button label="Manage" tone="neutral" onClick={onManage}></box-button> : null)}
    </header>
    <div className="settings-connection-list">{children}</div>
  </section>
}

function RemoveConnectionButton({ label, disabled, onRemove }: { label: string; disabled: boolean; onRemove: () => void }) {
  const ref = useRef<HTMLElement>(null)
  const onRemoveRef = useRef(onRemove)
  useEffect(() => { onRemoveRef.current = onRemove }, [onRemove])
  useEffect(() => {
    const button = ref.current
    if (!button) return
    const handleClick = (event: Event) => {
      event.preventDefault()
      if (!disabled) onRemoveRef.current()
    }
    button.addEventListener('click', handleClick)
    return () => button.removeEventListener('click', handleClick)
  }, [disabled])
  return <box-icon-button ref={ref} className="settings-connection-remove" icon="cart-1" label={label} disabled={disabled}></box-icon-button>
}

export function ProviderConnectionRow({ primary, details, selected = false, ready = false, removeLabel, removeDisabled = false, onRemove }: { primary: string; details: string[]; selected?: boolean; ready?: boolean; removeLabel?: string; removeDisabled?: boolean; onRemove?: () => void }) {
  return <article className="settings-connection-row"><div className="settings-connection-copy"><strong>{primary}</strong>{details.filter(Boolean).map((detail, index) => <small key={`${detail}-${index}`}>{detail}</small>)}</div><div className="settings-connection-actions"><div className="settings-connection-status">{selected ? <box-badge label="Selected" tone="info"></box-badge> : null}<box-badge label={ready ? 'Ready' : 'Not ready'} tone={ready ? 'success' : 'error'}></box-badge></div>{onRemove && removeLabel ? <RemoveConnectionButton label={removeLabel} disabled={removeDisabled} onRemove={onRemove}/> : null}</div></article>
}

export function EmptyProviderConnection({ provider, compact = false }: { provider: string; compact?: boolean }) {
  return <p className="settings-empty">{compact ? `No ${provider} environment selected.` : `No saved ${provider} connections.`}</p>
}
