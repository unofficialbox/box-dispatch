// @vitest-environment jsdom
import { fireEvent, render } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { TableColumn, TableRow } from '@unofficialbox/box-open-elements/table'
import { BoeTable } from './BoeTable'

const columns: TableColumn[] = [{ key: 'name', label: 'Name', sortable: true }]
const rows: TableRow[] = [{ id: 'one', cells: { name: 'One' } }]

describe('BoeTable', () => {
  it('bridges data, state copy, sorting, and selection through element properties', () => {
    const onSort = vi.fn()
    const onSelectionChange = vi.fn()
    const { container, rerender } = render(<BoeTable label="Records" columns={columns} rows={[]} emptyText="Nothing recorded" errorText="" selectionMode="single" selectedIds={[]} onSort={onSort} onSelectionChange={onSelectionChange}/>)
    const table = container.querySelector('box-table') as HTMLElement & { columns: TableColumn[]; rows: TableRow[]; emptyText: string; errorText: string; selectionMode: string; selectedIds: string[] }

    expect(table.columns).toEqual(columns)
    expect(table.rows).toEqual([])
    expect(table.emptyText).toBe('Nothing recorded')
    expect(table.errorText).toBe('')
    expect(table.selectionMode).toBe('single')

    fireEvent(table, new CustomEvent('sort', { detail: { key: 'name', direction: 'descending' } }))
    fireEvent(table, new CustomEvent('selection-changed', { detail: { selectedIds: ['one'] } }))
    expect(onSort).toHaveBeenCalledWith({ key: 'name', direction: 'descending' })
    expect(onSelectionChange).toHaveBeenCalledWith(['one'])

    rerender(<BoeTable label="Records" columns={columns} rows={rows} emptyText="Nothing recorded" errorText="Records unavailable" selectionMode="single" selectedIds={['one']}/>)
    expect(table.rows).toEqual(rows)
    expect(table.errorText).toBe('Records unavailable')
    expect(table.selectedIds).toEqual(['one'])
  })
})
