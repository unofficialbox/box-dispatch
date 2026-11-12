import type { FolderTreeDiffNode } from '@unofficialbox/box-open-elements/patterns/folder-tree'
import type { BoxFolder, BoxFolderDiff } from './types'

const statusMap: Record<BoxFolderDiff['status'], FolderTreeDiffNode['status']> = {
  changed: 'changed',
  equal: 'equal',
  left_only: 'removed',
  right_only: 'added',
}

function folderMetadata(folder?: BoxFolder) {
  if (!folder) return undefined
  return {
    folders: folder.items.folders,
    files: folder.items.files,
    webLinks: folder.items.webLinks,
    other: folder.items.other,
  }
}

function countLabel(value: number, singular: string, plural = `${singular}s`) {
  return `${value} ${value === 1 ? singular : plural}`
}

function folderSummary(folder?: BoxFolder) {
  if (!folder) return ''
  const counts = [
    folder.items.folders ? countLabel(folder.items.folders, 'folder') : '',
    folder.items.files ? countLabel(folder.items.files, 'file') : '',
    folder.items.webLinks ? countLabel(folder.items.webLinks, 'web link') : '',
    folder.items.other ? countLabel(folder.items.other, 'other item') : '',
  ].filter(Boolean)
  return counts.join(' · ') || 'Empty folder'
}

function changeSummary(folder: BoxFolderDiff) {
  if (folder.status !== 'changed') return folderSummary(folder.left ?? folder.right)
  return folder.changes.map((change) => {
    const label = change.path.replace('items.', '').replace('webLinks', 'web links')
    return `${label} ${String(change.before)} → ${String(change.after)}`
  }).join(' · ')
}

function parentPath(path: string, knownPaths: Set<string>) {
  let candidate = path.slice(0, path.lastIndexOf('/'))
  while (candidate) {
    if (knownPaths.has(candidate)) return candidate
    candidate = candidate.slice(0, candidate.lastIndexOf('/'))
  }
  return undefined
}

export function buildFolderTreeDiffNodes(folders: BoxFolderDiff[]): FolderTreeDiffNode[] {
  const ordered = [...folders].sort((left, right) => left.path.localeCompare(right.path))
  const knownPaths = new Set(ordered.map((folder) => folder.path))
  const parentByPath = new Map(ordered.map((folder) => [folder.path, parentPath(folder.path, knownPaths)]))
  const childCountByPath = new Map<string, number>()

  for (const parent of parentByPath.values()) {
    if (parent) childCountByPath.set(parent, (childCountByPath.get(parent) ?? 0) + 1)
  }

  const changedAncestors = new Set<string>()
  for (const folder of ordered) {
    if (folder.status === 'equal') continue
    let parent = parentByPath.get(folder.path)
    while (parent) {
      changedAncestors.add(parent)
      parent = parentByPath.get(parent)
    }
  }

  return ordered.map((folder) => {
    const childCount = childCountByPath.get(folder.path) ?? 0
    return {
      id: folder.path,
      parentId: parentByPath.get(folder.path),
      name: folder.name,
      type: 'folder',
      path: folder.path,
      status: statusMap[folder.status],
      before: folder.left ? { name: folder.left.name, path: folder.left.path, metadata: folderMetadata(folder.left) } : undefined,
      after: folder.right ? { name: folder.right.name, path: folder.right.path, metadata: folderMetadata(folder.right) } : undefined,
      hasChildren: childCount > 0,
      childCount,
      metadata: changeSummary(folder),
      loadState: 'loaded',
      hasChangedDescendants: changedAncestors.has(folder.path),
    }
  })
}
