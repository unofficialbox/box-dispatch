import { useEffect, useRef } from 'react'
import '@unofficialbox/box-open-elements/search-field'
import type { BoxElementEventMap } from '@unofficialbox/box-open-elements/native-types'

export function SearchFieldControl({ value, label, placeholder, onChange }: { value: string; label: string; placeholder?: string; onChange: (value: string) => void }) {
  const ref = useRef<HTMLElementTagNameMap['box-search-field']>(null)

  useEffect(() => {
    const element = ref.current
    if (!element) return
    const handleChange = (event: BoxElementEventMap['box-search-field']['value-changed']) => onChange(event.detail.value)
    element.addEventListener('value-changed', handleChange)
    return () => element.removeEventListener('value-changed', handleChange)
  }, [onChange])

  return <box-search-field ref={ref} label={label} value={value} placeholder={placeholder}></box-search-field>
}
