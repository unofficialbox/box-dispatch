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
    description: 'Compare the current Salesforce org with the validated package before deployment.',
    beforeLabel: 'Current org',
    afterLabel: 'Validated package',
    direction: 'Current → Validated',
    emptyTitle: 'No Salesforce file changes',
    emptyDescription: 'The selected org already matches the validated Salesforce package.',
  },
  deployment: {
    heading: 'Review deployed changes',
    description: 'Compare the recorded state before and after this deployment.',
    beforeLabel: 'Before deployment',
    afterLabel: 'After deployment',
    direction: 'Before → After',
    emptyTitle: 'No file changes',
    emptyDescription: 'This deployment did not record any Salesforce file additions or updates.',
  },
} as const

const changeColumns: TableColumn[] = [
  { key: 'change', label: 'Change' },
  { key: 'file', label: 'File' },
  { key: 'component', label: 'Component' },
]

const fileID = (file: ValidationFileChange) => `${file.component}:${file.path}`

export function ValidationChangesDrawer({ files, loading, error, stage = 'validation', onClose }: { files: ValidationFileChange[]; loading: boolean; error: string; stage?: ChangeReviewStage; onClose: () => void }) {
  const drawerRef = useRef<HTMLElementTagNameMap['box-drawer']>(null)
  const [selectedFileID, setSelectedFileID] = useState('')
  const selected = useMemo(() => files.find((file) => fileID(file) === selectedFileID) ?? files[0], [files, selectedFileID])
  const rows = useMemo<TableRow[]>(() => files.map((file) => ({
    id: fileID(file),
    cells: {
      change: { kind: 'badge', text: file.kind === 'add' ? 'Add' : 'Update', tone: file.kind === 'add' ? 'success' : 'brand' },
      file: file.path.split('/').at(-1) ?? file.path,
      component: file.component,
    },
    detail: file.path,
  })), [files])
  const copy = reviewCopy[stage]

  useEffect(() => {
    void import('@unofficialbox/box-open-elements/diff-viewer')
  }, [])

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
      {!loading && !error && files.length === 0 && <div className="validation-changes-empty"><h3>{copy.emptyTitle}</h3><p>{copy.emptyDescription}</p></div>}
      {files.length > 0 && <div className="validation-change-browser">
        <section className="validation-change-files" aria-label="Files with changes">
          <header><span>{files.length} {files.length === 1 ? 'file' : 'files'}</span><strong>{copy.direction}</strong></header>
          <BoeTable className="validation-change-table" columns={changeColumns} rows={rows} label="Files with changes" emptyText={copy.emptyDescription} selectionMode="single" selectedIds={selected ? [fileID(selected)] : []} onSelectionChange={(selectedIDs) => setSelectedFileID(selectedIDs[0] ?? '')}/>
        </section>
        <section className="validation-change-preview" aria-live="polite">
          {selected?.previewable
            ? <box-diff-viewer heading={selected.path} before-label={copy.beforeLabel} after-label={copy.afterLabel} before-text={selected.before ?? ''} after-text={selected.after ?? ''} mode="split"></box-diff-viewer>
            : <div className="validation-changes-empty"><h3>Preview unavailable</h3><p>{selected?.path} is binary or too large for an inline text comparison. It will still be included in deployment.</p></div>}
        </section>
      </div>}
    </section>
  </box-drawer>
}
