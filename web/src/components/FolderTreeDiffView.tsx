import { useEffect, useMemo, useRef, useState } from 'react'
import '@unofficialbox/box-open-elements/patterns/folder-tree'
import type { BoxFolderComparison } from '../types'
import { buildFolderTreeDiffNodes } from '../folderTreeDiff'

export function FolderTreeDiffView({ comparison, mode }: { comparison: BoxFolderComparison; mode: 'unified' | 'side-by-side' }) {
  const ref = useRef<HTMLElementTagNameMap['box-folder-tree-diff']>(null)
  const nodes = useMemo(() => buildFolderTreeDiffNodes(comparison.folders), [comparison.folders])
  const [changesOnly, setChangesOnly] = useState(true)
  const partial = comparison.scope.leftTruncated || comparison.scope.rightTruncated

  useEffect(() => {
    const tree = ref.current
    if (!tree) return
    tree.nodes = nodes
    tree.activeId = nodes.find((node) => node.status !== 'equal')?.id ?? nodes[0]?.id ?? null
  }, [nodes])

  useEffect(() => {
    const tree = ref.current
    if (!tree) return
    tree.label = 'Folder structure changes'
    tree.leftLabel = comparison.left.alias
    tree.rightLabel = comparison.right.alias
    tree.mode = mode
    tree.partial = partial
  }, [comparison.left.alias, comparison.right.alias, mode, partial])

  useEffect(() => {
    const tree = ref.current
    if (!tree) return
    tree.changesOnly = changesOnly
  }, [changesOnly])

  useEffect(() => {
    const tree = ref.current
    if (!tree) return
    const handleChangesOnly = (event: CustomEvent<{ changesOnly: boolean }>) => setChangesOnly(event.detail.changesOnly)
    tree.addEventListener('changes-only-changed', handleChangesOnly)
    return () => tree.removeEventListener('changes-only-changed', handleChangesOnly)
  }, [])

  return <box-folder-tree-diff ref={ref}></box-folder-tree-diff>
}
