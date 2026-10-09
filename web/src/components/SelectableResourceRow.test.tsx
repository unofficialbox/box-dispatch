// @vitest-environment jsdom
import { fireEvent, render } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SelectableResourceRow } from './SelectableResourceRow'

describe('SelectableResourceRow', () => {
  it('uses the published selection and independent action slots', () => {
    const onSelect = vi.fn()
    const onAction = vi.fn()
    const { container } = render(<SelectableResourceRow label="Salesforce" meta="example.my.salesforce.com" value="salesforce" selected status={<box-badge label="Ready" tone="success"></box-badge>} actions={<box-button label="Manage" onClick={onAction}></box-button>} onSelect={onSelect}/>)
    const row = container.querySelector('box-resource-row')!

    expect(row.hasAttribute('selected')).toBe(true)
    expect(row.querySelector('[slot="status"] box-badge')?.getAttribute('label')).toBe('Ready')
    expect(row.querySelector('[slot="actions"] box-button')?.getAttribute('label')).toBe('Manage')
    fireEvent(row, new CustomEvent('select', { detail: { value: 'salesforce' } }))
    expect(onSelect).toHaveBeenCalledWith('salesforce')
  })
})
