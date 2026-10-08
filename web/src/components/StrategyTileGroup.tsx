import { useEffect, useRef } from 'react'
import '@unofficialbox/box-open-elements/tile-group'
import type { TileOption } from '@unofficialbox/box-open-elements/tile-group'

type Strategy = 'reuse' | 'create_new'
type TileGroupElement = HTMLElement & {
  options: TileOption[]
  value: string
}

const strategyOptions: TileOption[] = [
  { id: 'reuse', label: 'Reuse existing', description: 'Keep matching configuration and apply only what is missing.' },
  { id: 'create_new', label: 'Create new', description: 'Create a new named configuration set for this deployment.' },
]

export function StrategyTileGroup({ value, disabled = false, legend = 'Deployment strategy', onChange }: { value: Strategy; disabled?: boolean; legend?: string; onChange: (strategy: Strategy) => void }) {
  const ref = useRef<TileGroupElement>(null)
  const options = strategyOptions.map((option) => ({ ...option, disabled }))

  useEffect(() => {
    const element = ref.current
    if (!element) return
    const handleChange = (event: Event) => {
      const selected = (event as CustomEvent<{ selected: string[] }>).detail.selected[0]
      if (selected === 'reuse' || selected === 'create_new') onChange(selected)
    }
    element.addEventListener('tile-change', handleChange)
    return () => element.removeEventListener('tile-change', handleChange)
  }, [onChange])

  return <box-tile-group ref={ref} className="strategy-picker" legend={legend} name="deployment-strategy" options={options} value={value}></box-tile-group>
}
