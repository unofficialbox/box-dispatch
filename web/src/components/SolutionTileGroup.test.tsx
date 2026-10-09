// @vitest-environment jsdom
import { fireEvent, render } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SolutionTileGroup } from './SolutionTileGroup'

const options = [
  { id: 'clm', label: 'Contract Lifecycle Management', description: 'Contract workflows.', meta: 'Legal operations', status: { label: 'Available', tone: 'success' as const } },
  { id: 'future', label: 'Future solution', disabled: true, disabledReason: 'Coming soon', status: { label: 'Coming soon' } },
]

describe('SolutionTileGroup', () => {
  it('uses the published metadata and status contract and forwards selection', () => {
    const onChange = vi.fn()
    const { container } = render(<SolutionTileGroup options={options} value="clm" name="solution" legend="Solution" onChange={onChange}/>)
    const group = container.querySelector<HTMLElement & { options: typeof options }>('box-tile-group')!

    expect(group.options).toEqual(options)
    fireEvent(group, new CustomEvent('tile-change', { detail: { selected: ['future'] } }))
    expect(onChange).toHaveBeenCalledWith('future')
  })

  it('disables every option while the surrounding workflow is busy or read-only', () => {
    const { container } = render(<SolutionTileGroup options={options} value="clm" name="solution" legend="Solution" disabled/>)
    const group = container.querySelector<HTMLElement & { options: Array<{ disabled?: boolean }> }>('box-tile-group')!

    expect(group.options.every((option) => option.disabled)).toBe(true)
  })
})
