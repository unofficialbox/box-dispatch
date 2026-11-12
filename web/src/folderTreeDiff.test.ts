import { describe, expect, it } from 'vitest'
import type { BoxFolderDiff, BoxFolderEntryDiff } from './types'
import { buildFolderTreeDiffNodes } from './folderTreeDiff'

const folder = (path: string, status: BoxFolderDiff['status'], files = 0): BoxFolderDiff => ({
  path,
  name: path.split('/').at(-1) ?? path,
  status,
  changes: status === 'changed' ? [{ path: 'items.files', before: files, after: files + 1 }] : [],
  left: status === 'right_only' ? undefined : { path, name: path.split('/').at(-1) ?? path, depth: path.split('/').length - 1, items: { folders: 0, files, webLinks: 0, other: 0 } },
  right: status === 'left_only' ? undefined : { path, name: path.split('/').at(-1) ?? path, depth: path.split('/').length - 1, items: { folders: 0, files: status === 'changed' ? files + 1 : files, webLinks: 0, other: 0 } },
})

const entry = (path: string, status: BoxFolderEntryDiff['status'], leftType: BoxFolderEntryDiff['type'], rightType = leftType): BoxFolderEntryDiff => ({
  path,
  name: path.split('/').at(-1) ?? path,
  type: rightType,
  status,
  changes: status === 'changed' ? [{ path: 'type', before: leftType, after: rightType }] : [],
  left: status === 'right_only' ? undefined : { path, name: path.split('/').at(-1) ?? path, type: leftType },
  right: status === 'left_only' ? undefined : { path, name: path.split('/').at(-1) ?? path, type: rightType },
})

describe('buildFolderTreeDiffNodes', () => {
  it('maps live paths, hierarchy, statuses, direct counts, and changed descendants', () => {
    const nodes = buildFolderTreeDiffNodes([
      folder('/Contracts/Archive', 'left_only', 2),
      folder('/Contracts', 'equal'),
      folder('/Legal', 'right_only'),
      folder('/Contracts/Active', 'changed', 3),
    ], [
      entry('/Contracts/Active/agreement.pdf', 'equal', 'file'),
      entry('/Contracts/Archive/legacy.txt', 'left_only', 'file'),
      entry('/Legal/Reference', 'right_only', 'web_link'),
      entry('/Contracts/Active/resource', 'changed', 'file', 'web_link'),
    ])

    expect(nodes.map((node) => node.id)).toEqual([
      'folder:/Contracts',
      'folder:/Contracts/Active',
      'entry:/Contracts/Active/agreement.pdf',
      'entry:/Contracts/Active/resource',
      'folder:/Contracts/Archive',
      'entry:/Contracts/Archive/legacy.txt',
      'folder:/Legal',
      'entry:/Legal/Reference',
    ])
    expect(nodes[0]).toMatchObject({ parentId: undefined, status: 'equal', childCount: 2, hasChildren: true, hasChangedDescendants: true })
    expect(nodes[1]).toMatchObject({ parentId: 'folder:/Contracts', status: 'changed', metadata: 'files 3 → 4', childCount: 2 })
    expect(nodes[1].before?.metadata).toEqual({ folders: 0, files: 3, webLinks: 0, other: 0 })
    expect(nodes[3]).toMatchObject({ parentId: 'folder:/Contracts/Active', type: 'web_link', status: 'changed', metadata: 'type file → web link' })
    expect(nodes[4]).toMatchObject({ parentId: 'folder:/Contracts', status: 'removed', metadata: '2 files' })
    expect(nodes[5]).toMatchObject({ parentId: 'folder:/Contracts/Archive', type: 'file', status: 'removed' })
    expect(nodes[6]).toMatchObject({ parentId: undefined, status: 'added', metadata: 'Empty folder', childCount: 1 })
    expect(nodes[7]).toMatchObject({ parentId: 'folder:/Legal', type: 'web_link', status: 'added' })
  })
})
