import { useEffect, useRef, type ReactNode } from 'react'
import '@unofficialbox/box-open-elements/resource-row'

type ResourceRowElement = HTMLElement & {
  selected: boolean
  disabled: boolean
}

export function SelectableResourceRow({ className, label, meta, value, status, selected = false, disabled = false, icon, actions, onSelect }: { className?: string; label: string; meta?: string; value: string; status?: ReactNode; selected?: boolean; disabled?: boolean; icon?: ReactNode; actions?: ReactNode; onSelect: (value: string) => void }) {
  const ref = useRef<ResourceRowElement>(null)
  const onSelectRef = useRef(onSelect)
  useEffect(() => { onSelectRef.current = onSelect }, [onSelect])
  useEffect(() => {
    const row = ref.current
    if (!row) return
    const handleSelect = (event: Event) => onSelectRef.current((event as CustomEvent<{ value: string }>).detail.value)
    row.addEventListener('select', handleSelect)
    return () => row.removeEventListener('select', handleSelect)
  }, [])
  return <box-resource-row ref={ref} className={className} label={label} meta={meta} value={value} selected={selected} disabled={disabled}>
    {icon ? <span className="resource-row-icon" slot="icon">{icon}</span> : null}
    {status ? <span className="resource-row-status" slot="status">{status}</span> : null}
    {actions ? <span className="resource-row-actions" slot="actions">{actions}</span> : null}
  </box-resource-row>
}
