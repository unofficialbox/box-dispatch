import { useEffect, useRef } from 'react'
import '@unofficialbox/box-open-elements/tile-group'
import type { BoxElementEventMap } from '@unofficialbox/box-open-elements/native-types'
import type { TileOption } from '@unofficialbox/box-open-elements/tile-group'

export function SolutionTileGroup({ options, value, name, className, legend, disabled = false, onChange }: { options: TileOption[]; value: string; name: string; className?: string; legend: string; disabled?: boolean; onChange?: (value: string) => void }) {
  const ref = useRef<HTMLElementTagNameMap['box-tile-group']>(null)
  const renderedOptions = disabled ? options.map((option) => ({ ...option, disabled: true })) : options

  useEffect(() => {
    const element = ref.current
    if (!element || !onChange) return
    const handleChange = (event: BoxElementEventMap['box-tile-group']['tile-change']) => {
      const selected = event.detail.selected[0]
      if (selected) onChange(selected)
    }
    element.addEventListener('tile-change', handleChange)
    return () => element.removeEventListener('tile-change', handleChange)
  }, [onChange])

  return <box-tile-group ref={ref} className={className} legend={legend} name={name} options={renderedOptions} value={value}></box-tile-group>
}
