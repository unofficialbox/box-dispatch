import { useEffect, useMemo, useRef, useState } from 'react'
import { Alert } from '@unofficialbox/box-open-elements-react/alert'
import type { BoxElementEventMap } from '@unofficialbox/box-open-elements/native-types'
import type { TableColumn, TableRow } from '@unofficialbox/box-open-elements/table'
import type { ValidationFileChange } from '../types'
import { BoeTable } from './BoeTable'

type ChangeReviewStage = 'validation' | 'deployment'

const reviewCopy = {
  validation: {
    heading: 'Review validation changes',
    description: 'Compare the current state with the validated package before deployment.',
    beforeLabel: 'Current state',
    afterLabel: 'Validated package',
    direction: 'Current → Validated',
    emptyTitle: 'No file changes',
    emptyDescription: 'The selected environment already matches the validated package.',
  },
  deployment: {
    heading: 'Review deployed changes',
    description: 'Compare the recorded state before and after this deployment.',
    beforeLabel: 'Before deployment',
    afterLabel: 'After deployment',
    direction: 'Before → After',
    emptyTitle: 'No file changes',
    emptyDescription: 'This deployment did not record any file additions or updates.',
  },
} as const

const changeColumns: TableColumn[] = [
  { key: 'change', label: 'Change' },
  { key: 'item', label: 'File', rowHeader: true, hostContent: true },
]

const EMPTY_FILES: ValidationFileChange[] = []

const fileID = (file: ValidationFileChange) => `${file.component}:${file.path}`
const componentLabel = (component: string) => component
  .replace(/^Box:/, '')
  .replaceAll('_', ' ')
  .replace(':', ' · ')

export function ValidationChangesDrawer({ files, loading, error, stage = 'validation', onClose }: { files: ValidationFileChange[] | null | undefined; loading: boolean; error: string; stage?: ChangeReviewStage; onClose: () => void }) {
  const drawerRef = useRef<HTMLElementTagNameMap['box-drawer']>(null)
  const previewRef = useRef<HTMLElement>(null)
  const [selectedFileID, setSelectedFileID] = useState('')
  const [diffMode, setDiffMode] = useState<'split' | 'inline'>('split')
  const safeFiles = files ?? EMPTY_FILES
  const selected = useMemo(() => safeFiles.find((file) => fileID(file) === selectedFileID) ?? safeFiles[0], [safeFiles, selectedFileID])
  const rows = useMemo<TableRow[]>(() => safeFiles.map((file) => ({
    id: fileID(file),
    cells: {
      change: { kind: 'badge', text: file.kind === 'add' ? 'Add' : 'Update', tone: file.kind === 'add' ? 'success' : 'brand' },
      item: file.path.split('/').at(-1) ?? file.path,
    },
    detail: file.path,
  })), [safeFiles])
  const copy = reviewCopy[stage]

  useEffect(() => {
    void import('@unofficialbox/box-open-elements/diff-viewer')
  }, [])

  useEffect(() => {
    const preview = previewRef.current
    if (!preview || typeof ResizeObserver === 'undefined') return
    const updateMode = () => setDiffMode(preview.clientWidth > 0 && preview.clientWidth < 720 ? 'inline' : 'split')
    updateMode()
    const observer = new ResizeObserver(updateMode)
    observer.observe(preview)
    return () => observer.disconnect()
  }, [safeFiles.length])

  useEffect(() => {
    const drawer = drawerRef.current
    if (!drawer) return
    const handleOpenChanged = (event: BoxElementEventMap['box-drawer']['open-changed']) => {
      if (!event.detail.open) onClose()
    }
    drawer.addEventListener('open-changed', handleOpenChanged)
    return () => drawer.removeEventListener('open-changed', handleOpenChanged)
  }, [onClose])

  return <box-drawer ref={drawerRef} className="validation-changes-drawer" open heading={copy.heading} description={copy.description} position="right" size="full" busy={loading}>
    <section className="validation-changes-content">
      {error && <Alert className="drawer-inline-error" heading="Changes could not be loaded" message={error} tone="error" open/>}
      {!loading && !error && safeFiles.length === 0 && <div className="validation-changes-empty"><h3>{copy.emptyTitle}</h3><p>{copy.emptyDescription}</p></div>}
      {safeFiles.length > 0 && <div className="validation-change-browser">
        <section className="validation-change-files" aria-label="Files with changes">
          <header><span>{safeFiles.length} {safeFiles.length === 1 ? 'file' : 'files'}</span><strong>{copy.direction}</strong></header>
          <BoeTable className="validation-change-table" columns={changeColumns} rows={rows} label="Files with changes" emptyText={copy.emptyDescription} selectionMode="single" selectedIds={selected ? [fileID(selected)] : []} onSelectionChange={(selectedIDs) => setSelectedFileID(selectedIDs[0] ?? '')}>
            {safeFiles.map((file) => <span className="validation-change-item" slot={`cell-${fileID(file)}-item`} key={fileID(file)} onClick={() => setSelectedFileID(fileID(file))}><strong>{file.path.split('/').at(-1) ?? file.path}</strong><small>{componentLabel(file.component)}</small></span>)}
          </BoeTable>
        </section>
        <section ref={previewRef} className="validation-change-preview" aria-live="polite">
          {selected?.previewable
            ? <box-diff-viewer heading={selected.path} before-label={copy.beforeLabel} after-label={copy.afterLabel} before-text={selected.before ?? ''} after-text={selected.after ?? ''} mode={diffMode}></box-diff-viewer>
            : <div className="validation-changes-empty"><h3>Preview unavailable</h3><p>No text snapshot was recorded for {selected?.path}. The change remains part of the deployment record.</p></div>}
        </section>
      </div>}
    </section>
  </box-drawer>
}
