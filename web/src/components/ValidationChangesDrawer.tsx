import { useEffect, useMemo, useRef, useState } from 'react'
import type { ValidationFileChange } from '../types'

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

export function ValidationChangesDrawer({ files, loading, error, stage = 'validation', onClose }: { files: ValidationFileChange[]; loading: boolean; error: string; stage?: ChangeReviewStage; onClose: () => void }) {
  const drawerRef = useRef<HTMLElement>(null)
  const [selectedPath, setSelectedPath] = useState('')
  const selected = useMemo(() => files.find((file) => file.path === selectedPath) ?? files[0], [files, selectedPath])
  const copy = reviewCopy[stage]

  useEffect(() => {
    void import('@unofficialbox/box-open-elements/diff-viewer')
  }, [])

  useEffect(() => {
    const drawer = drawerRef.current
    if (!drawer) return
    const handleOpenChanged = (event: Event) => {
      if (!(event as CustomEvent<{ open: boolean }>).detail.open) onClose()
    }
    drawer.addEventListener('open-changed', handleOpenChanged)
    return () => drawer.removeEventListener('open-changed', handleOpenChanged)
  }, [onClose])

  return <box-drawer ref={drawerRef} className="validation-changes-drawer" open heading={copy.heading} description={copy.description} position="right" size="full" busy={loading}>
    <section className="validation-changes-content">
      {error && <div className="drawer-inline-error" role="alert"><strong>Changes could not be loaded</strong><p>{error}</p></div>}
      {!loading && !error && files.length === 0 && <div className="validation-changes-empty"><h3>{copy.emptyTitle}</h3><p>{copy.emptyDescription}</p></div>}
      {files.length > 0 && <div className="validation-change-browser">
        <nav className="validation-change-files" aria-label="Files with changes">
          <header><span>{files.length} {files.length === 1 ? 'file' : 'files'}</span><strong>{copy.direction}</strong></header>
          {files.map((file) => <button type="button" key={`${file.component}:${file.path}`} className={file.path === selected?.path ? 'selected' : ''} aria-current={file.path === selected?.path ? 'true' : undefined} onClick={() => setSelectedPath(file.path)}>
            <span className={`change-kind change-kind-${file.kind}`}>{file.kind === 'add' ? 'Add' : 'Update'}</span>
            <span className="change-file-copy"><strong>{file.path.split('/').at(-1)}</strong><small>{file.component}</small><code>{file.path}</code></span>
          </button>)}
        </nav>
        <section className="validation-change-preview" aria-live="polite">
          {selected?.previewable
            ? <box-diff-viewer heading={selected.path} before-label={copy.beforeLabel} after-label={copy.afterLabel} before-text={selected.before ?? ''} after-text={selected.after ?? ''} mode="split"></box-diff-viewer>
            : <div className="validation-changes-empty"><h3>Preview unavailable</h3><p>{selected?.path} is binary or too large for an inline text comparison. It will still be included in deployment.</p></div>}
        </section>
      </div>}
    </section>
  </box-drawer>
}
