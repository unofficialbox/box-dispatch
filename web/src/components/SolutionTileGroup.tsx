import { useEffect, useRef } from 'react'
import '@unofficialbox/box-open-elements/tile-group'
import type { TileOption } from '@unofficialbox/box-open-elements/tile-group'

type TileGroupElement = HTMLElement & {
  options: TileOption[]
  value: string
}

export function SolutionTileGroup({ options, value, name, className, legend, disabled = false, onChange }: { options: TileOption[]; value: string; name: string; className?: string; legend: string; disabled?: boolean; onChange?: (value: string) => void }) {
  const ref = useRef<TileGroupElement>(null)
  const renderedOptions = disabled ? options.map((option) => ({ ...option, disabled: true })) : options

  useEffect(() => {
    const element = ref.current
    if (!element || !onChange) return
    const handleChange = (event: Event) => {
      const selected = (event as CustomEvent<{ selected: string[] }>).detail.selected[0]
      if (selected) onChange(selected)
    }
    element.addEventListener('tile-change', handleChange)
    return () => element.removeEventListener('tile-change', handleChange)
  }, [onChange])

  return <box-tile-group ref={ref} className={className} legend={legend} name={name} options={renderedOptions} value={value}></box-tile-group>
}
