import { useEffect, useMemo, useRef, useState } from 'react'
import type { ValidationFileChange } from '../types'

export function ValidationChangesDrawer({ files, loading, error, onClose }: { files: ValidationFileChange[]; loading: boolean; error: string; onClose: () => void }) {
  const drawerRef = useRef<HTMLElement>(null)
  const [selectedPath, setSelectedPath] = useState('')
  const selected = useMemo(() => files.find((file) => file.path === selectedPath) ?? files[0], [files, selectedPath])

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

  return <box-drawer ref={drawerRef} className="validation-changes-drawer" open heading="Salesforce changes" description="Review the exact packaged files Dispatch will add or update." position="right" size="full" busy={loading}>
    <section className="validation-changes-content">
      {error && <div className="drawer-inline-error" role="alert"><strong>Changes could not be loaded</strong><p>{error}</p></div>}
      {!loading && !error && files.length === 0 && <div className="validation-changes-empty"><h3>No Salesforce file changes</h3><p>The selected org already matches the packaged Salesforce configuration.</p></div>}
      {files.length > 0 && <div className="validation-change-browser">
        <nav className="validation-change-files" aria-label="Salesforce files with changes">
          <header><span>{files.length} {files.length === 1 ? 'file' : 'files'}</span><strong>Current → Packaged</strong></header>
          {files.map((file) => <button type="button" key={`${file.component}:${file.path}`} className={file.path === selected?.path ? 'selected' : ''} aria-current={file.path === selected?.path ? 'true' : undefined} onClick={() => setSelectedPath(file.path)}>
            <span className={`change-kind change-kind-${file.kind}`}>{file.kind === 'add' ? 'Add' : 'Update'}</span>
            <span className="change-file-copy"><strong>{file.path.split('/').at(-1)}</strong><small>{file.component}</small><code>{file.path}</code></span>
          </button>)}
        </nav>
        <section className="validation-change-preview" aria-live="polite">
          {selected?.previewable
            ? <box-diff-viewer heading={selected.path} before-label="Current org" after-label="Packaged change" before-text={selected.before ?? ''} after-text={selected.after ?? ''} mode="split"></box-diff-viewer>
            : <div className="validation-changes-empty"><h3>Preview unavailable</h3><p>{selected?.path} is binary or too large for an inline text comparison. It will still be included in deployment.</p></div>}
        </section>
      </div>}
    </section>
  </box-drawer>
}
