import { useEffect, useRef } from 'react'
import '@unofficialbox/box-open-elements/table'
import type { BoxElementEventMap } from '@unofficialbox/box-open-elements/native-types'
import type { TableColumn, TableRow, TableRowActionDetail, TableSelectionMode, TableSortDetail } from '@unofficialbox/box-open-elements/table'

type BoeTableProps = {
  className?: string
  columns: TableColumn[]
  emptyText?: string
  errorText?: string
  label: string
  loading?: boolean
  onSelectionChange?: (selectedIds: string[]) => void
  onRowAction?: (detail: TableRowActionDetail) => void
  onSort?: (detail: TableSortDetail) => void
  rows: TableRow[]
  selectedIds?: string[]
  selectionMode?: TableSelectionMode
  sortDirection?: TableSortDetail['direction']
  sortKey?: string
}

const EMPTY_SELECTED_IDS: string[] = []

export function BoeTable({ className, columns, emptyText = 'No rows', errorText = '', label, loading = false, onSelectionChange, onRowAction, onSort, rows, selectedIds = EMPTY_SELECTED_IDS, selectionMode = 'none', sortDirection, sortKey }: BoeTableProps) {
  const tableRef = useRef<HTMLElementTagNameMap['box-table'] | null>(null)
  const onSelectionChangeRef = useRef(onSelectionChange)
  const onRowActionRef = useRef(onRowAction)
  const onSortRef = useRef(onSort)

  useEffect(() => { onSelectionChangeRef.current = onSelectionChange }, [onSelectionChange])
  useEffect(() => { onRowActionRef.current = onRowAction }, [onRowAction])
  useEffect(() => { onSortRef.current = onSort }, [onSort])

  useEffect(() => {
    const table = tableRef.current
    if (!table) return
    table.columns = columns
    table.rows = rows
    table.selectionMode = selectionMode
    table.selectedIds = selectedIds
  }, [columns, rows, selectedIds, selectionMode])

  useEffect(() => {
    const table = tableRef.current
    if (!table) return
    if (sortKey && sortDirection) {
      table.setAttribute('sort-key', sortKey)
      table.setAttribute('sort-direction', sortDirection)
    } else {
      table.removeAttribute('sort-key')
      table.removeAttribute('sort-direction')
    }
  }, [sortDirection, sortKey])

  useEffect(() => {
    const table = tableRef.current
    if (!table) return
    const handleSort = (event: BoxElementEventMap['box-table']['sort']) => onSortRef.current?.(event.detail)
    const handleSelection = (event: BoxElementEventMap['box-table']['selection-changed']) => onSelectionChangeRef.current?.(event.detail.selectedIds)
    const handleRowAction = (event: BoxElementEventMap['box-table']['row-action']) => onRowActionRef.current?.(event.detail)
    table.addEventListener('sort', handleSort)
    table.addEventListener('selection-changed', handleSelection)
    table.addEventListener('row-action', handleRowAction)
    return () => {
      table.removeEventListener('sort', handleSort)
      table.removeEventListener('selection-changed', handleSelection)
      table.removeEventListener('row-action', handleRowAction)
    }
  }, [])

  return <box-table ref={tableRef} className={className} label={label} loading={loading} emptyText={emptyText} errorText={errorText}></box-table>
}
