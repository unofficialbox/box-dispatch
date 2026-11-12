import type { FolderTreeDiffNode } from '@unofficialbox/box-open-elements/patterns/folder-tree'
import type { BoxFolder, BoxFolderDiff, BoxFolderEntryDiff } from './types'

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

const folderNodeID = (path: string) => `folder:${path}`
const entryNodeID = (path: string) => `entry:${path}`

export function buildFolderTreeDiffNodes(folders: BoxFolderDiff[], entries: BoxFolderEntryDiff[] = []): FolderTreeDiffNode[] {
  const orderedFolders = [...folders].sort((left, right) => left.path.localeCompare(right.path))
  const orderedEntries = [...entries].sort((left, right) => left.path.localeCompare(right.path))
  const knownPaths = new Set(orderedFolders.map((folder) => folder.path))
  const parentByPath = new Map(orderedFolders.map((folder) => [folder.path, parentPath(folder.path, knownPaths)]))
  const childCountByPath = new Map<string, number>()

  for (const parent of parentByPath.values()) {
    if (parent) childCountByPath.set(parent, (childCountByPath.get(parent) ?? 0) + 1)
  }
  for (const entry of orderedEntries) {
    const parent = parentPath(entry.path, knownPaths)
    if (parent) childCountByPath.set(parent, (childCountByPath.get(parent) ?? 0) + 1)
  }

  const changedAncestors = new Set<string>()
  const markChangedAncestors = (path: string) => {
    let parent = parentPath(path, knownPaths)
    while (parent) {
      changedAncestors.add(parent)
      parent = parentByPath.get(parent)
    }
  }
  for (const folder of orderedFolders) {
    if (folder.status === 'equal') continue
    markChangedAncestors(folder.path)
  }
  for (const entry of orderedEntries) if (entry.status !== 'equal') markChangedAncestors(entry.path)

  const nodes: FolderTreeDiffNode[] = orderedFolders.map((folder) => {
    const childCount = childCountByPath.get(folder.path) ?? 0
    return {
      id: folderNodeID(folder.path),
      parentId: parentByPath.get(folder.path) ? folderNodeID(parentByPath.get(folder.path)!) : undefined,
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

  for (const entry of orderedEntries) {
    const parent = parentPath(entry.path, knownPaths)
    const type = entry.type === 'web_link' ? 'web_link' : 'file'
    const typeChange = entry.status === 'changed' ? entry.changes.find((change) => change.path === 'type') : undefined
    nodes.push({
      id: entryNodeID(entry.path),
      parentId: parent ? folderNodeID(parent) : undefined,
      name: entry.name,
      type,
      path: entry.path,
      status: statusMap[entry.status],
      before: entry.left ? { name: entry.left.name, path: entry.left.path, metadata: { type: entry.left.type } } : undefined,
      after: entry.right ? { name: entry.right.name, path: entry.right.path, metadata: { type: entry.right.type } } : undefined,
      hasChildren: false,
      childCount: 0,
      metadata: typeChange ? `type ${String(typeChange.before).replace('_', ' ')} → ${String(typeChange.after).replace('_', ' ')}` : '',
      loadState: 'loaded',
      hasChangedDescendants: false,
    })
  }

  return nodes.sort((left, right) => left.path.localeCompare(right.path) || (left.type === 'folder' ? -1 : 1))
}
