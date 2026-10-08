import { useEffect, useRef } from 'react'
import '@unofficialbox/box-open-elements/search-field'

type SearchFieldElement = HTMLElement & { value: string }

export function SearchFieldControl({ value, label, placeholder, onChange }: { value: string; label: string; placeholder?: string; onChange: (value: string) => void }) {
  const ref = useRef<SearchFieldElement>(null)

  useEffect(() => {
    const element = ref.current
    if (!element) return
    const handleChange = (event: Event) => onChange((event as CustomEvent<{ value: string }>).detail.value)
    element.addEventListener('value-changed', handleChange)
    return () => element.removeEventListener('value-changed', handleChange)
  }, [onChange])

  return <box-search-field ref={ref} label={label} value={value} placeholder={placeholder}></box-search-field>
}
