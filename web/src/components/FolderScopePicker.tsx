import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Alert } from '@unofficialbox/box-open-elements-react/alert'
import { TextField } from '@unofficialbox/box-open-elements-react/text-field'
import '@unofficialbox/box-open-elements/patterns/folder-tree'
import type { FolderTreeNode, FolderTreeNodeDetail, FolderTreeSelectedDetail } from '@unofficialbox/box-open-elements/patterns/folder-tree'
import type { BoxFolderChildren, BoxFolderSearchResults } from '../types'

export type FolderScope = { id: string; name: string }

const rootNode = (): FolderTreeNode => ({ id: '0', name: 'All Files', type: 'folder', hasChildren: true, loadState: 'unloaded' })

async function readJSON<T>(response: Response): Promise<T> {
  if (response.ok) return response.json() as Promise<T>
  const payload = await response.json().catch(() => null) as { error?: string } | null
  throw new Error(payload?.error?.trim() || 'Live folders could not be loaded.')
}

export function FolderScopePicker({ connectionId, label, selected, disabled, onSelected }: { connectionId: string; label: string; selected: FolderScope; disabled: boolean; onSelected: (scope: FolderScope) => void }) {
  const treeRef = useRef<HTMLElementTagNameMap['box-folder-tree']>(null)
  const requestsRef = useRef(new Map<string, AbortController>())
  const searchRequestRef = useRef<AbortController | null>(null)
  const [browseNodes, setBrowseNodes] = useState<FolderTreeNode[]>([rootNode()])
  const [searchNodes, setSearchNodes] = useState<FolderTreeNode[] | null>(null)
  const [query, setQuery] = useState('')
  const [browsePartial, setBrowsePartial] = useState(false)
  const [searchPartial, setSearchPartial] = useState(false)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  const stopRequests = useCallback(() => {
    for (const controller of requestsRef.current.values()) controller.abort()
    requestsRef.current.clear()
    searchRequestRef.current?.abort()
    searchRequestRef.current = null
  }, [])

  const loadChildren = useCallback(async (parentId: string) => {
    if (!connectionId || requestsRef.current.has(parentId)) return
    const controller = new AbortController()
    requestsRef.current.set(parentId, controller)
    setBrowseNodes((current) => current.map((node) => node.id === parentId ? { ...node, loadState: 'loading' } : node))
    setLoading(true)
    setError('')
    try {
      const response = await fetch('/api/box/folders/children', {
        method: 'POST', headers: { 'Content-Type': 'application/json' }, signal: controller.signal,
        body: JSON.stringify({ connectionId, parentId }),
      })
      const children = await readJSON<BoxFolderChildren>(response)
      setBrowseNodes((current) => {
        const withoutExistingChildren = current.filter((node) => node.parentId !== parentId)
        const nextChildren: FolderTreeNode[] = children.folders.map((folder) => ({ id: folder.id, parentId, name: folder.name, type: 'folder', hasChildren: true, loadState: 'unloaded' }))
        const parentLoadState: FolderTreeNode['loadState'] = children.partial ? 'partial' : 'loaded'
        return withoutExistingChildren.map((node) => node.id === parentId ? { ...node, hasChildren: nextChildren.length > 0, childCount: nextChildren.length, loadState: parentLoadState } : node).concat(nextChildren)
      })
      setBrowsePartial((current) => current || children.partial)
    } catch (caught) {
      if (caught instanceof DOMException && caught.name === 'AbortError') return
      setBrowseNodes((current) => current.map((node) => node.id === parentId ? { ...node, loadState: 'error' } : node))
      setError(caught instanceof Error ? caught.message : 'Live folders could not be loaded.')
    } finally {
      requestsRef.current.delete(parentId)
      setLoading(requestsRef.current.size > 0 || searchRequestRef.current !== null)
    }
  }, [connectionId])

  useEffect(() => {
    stopRequests()
    // oxlint-disable-next-line react/set-state-in-effect
    setBrowseNodes([rootNode()])
    setSearchNodes(null)
    setQuery('')
    setBrowsePartial(false)
    setSearchPartial(false)
    setError('')
    if (connectionId) void loadChildren('0')
    return stopRequests
  }, [connectionId, loadChildren, stopRequests])

  useEffect(() => {
    const search = query.trim()
    searchRequestRef.current?.abort()
    searchRequestRef.current = null
    if (search.length < 2) {
      // oxlint-disable-next-line react/set-state-in-effect
      setSearchNodes(null)
      setSearchPartial(false)
      return
    }
    const controller = new AbortController()
    searchRequestRef.current = controller
    const timer = window.setTimeout(async () => {
      setLoading(true)
      setError('')
      try {
        const response = await fetch('/api/box/folders/search', {
          method: 'POST', headers: { 'Content-Type': 'application/json' }, signal: controller.signal,
          body: JSON.stringify({ connectionId, query: search }),
        })
        const results = await readJSON<BoxFolderSearchResults>(response)
        setSearchNodes(results.folders.map((folder) => ({ id: folder.id, name: folder.name, type: 'folder', hasChildren: false, loadState: 'loaded' })))
        setSearchPartial(results.partial)
      } catch (caught) {
        if (caught instanceof DOMException && caught.name === 'AbortError') return
        setSearchNodes([])
        setError(caught instanceof Error ? caught.message : 'Live folder search could not be completed.')
      } finally {
        if (searchRequestRef.current === controller) {
          searchRequestRef.current = null
          setLoading(requestsRef.current.size > 0)
        }
      }
    }, 250)
    return () => {
      window.clearTimeout(timer)
      controller.abort()
    }
  }, [connectionId, query])

  const visibleNodes = useMemo(() => searchNodes ?? browseNodes, [browseNodes, searchNodes])
  const partial = searchNodes === null ? browsePartial : searchPartial

  useEffect(() => {
    const tree = treeRef.current
    if (!tree) return
    tree.nodes = visibleNodes
    tree.label = `${label} folder root`
    tree.selectedIds = [selected.id]
    tree.selectionDisabled = disabled
    tree.loading = loading
    tree.partial = partial
  }, [disabled, label, loading, partial, selected.id, visibleNodes])

  useEffect(() => {
    const tree = treeRef.current
    if (!tree) return
    const onLoad = (event: Event) => {
      if (searchNodes) return
      const detail = (event as CustomEvent<FolderTreeNodeDetail>).detail
      void loadChildren(detail.id)
    }
    const onSelect = (event: Event) => {
      const detail = (event as CustomEvent<FolderTreeSelectedDetail>).detail
      onSelected({ id: detail.node.id, name: detail.node.name })
    }
    tree.addEventListener('load-children', onLoad)
    tree.addEventListener('selected-changed', onSelect)
    return () => {
      tree.removeEventListener('load-children', onLoad)
      tree.removeEventListener('selected-changed', onSelect)
    }
  }, [loadChildren, onSelected, searchNodes])

  return <section className="folder-scope-picker" aria-label={label}>
    <header><span>{label}</span><strong>{selected.name}</strong></header>
    <TextField label="Find a folder" value={query} placeholder="Search live folders" disabled={disabled} onValueChanged={(event) => setQuery(event.detail.value)}/>
    <div className="folder-scope-tree"><box-folder-tree ref={treeRef}></box-folder-tree></div>
    {partial ? <p className="folder-scope-partial">More folders are available. Narrow the search or expand a branch.</p> : null}
    {error ? <Alert heading="Folders unavailable" message={error} tone="error" open/> : null}
  </section>
}
