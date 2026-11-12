import { describe, expect, it } from 'vitest'
import type { BoxFolderDiff } from './types'
import { buildFolderTreeDiffNodes } from './folderTreeDiff'

const folder = (path: string, status: BoxFolderDiff['status'], files = 0): BoxFolderDiff => ({
  path,
  name: path.split('/').at(-1) ?? path,
  status,
  changes: status === 'changed' ? [{ path: 'items.files', before: files, after: files + 1 }] : [],
  left: status === 'right_only' ? undefined : { path, name: path.split('/').at(-1) ?? path, depth: path.split('/').length - 1, items: { folders: 0, files, webLinks: 0, other: 0 } },
  right: status === 'left_only' ? undefined : { path, name: path.split('/').at(-1) ?? path, depth: path.split('/').length - 1, items: { folders: 0, files: status === 'changed' ? files + 1 : files, webLinks: 0, other: 0 } },
})

describe('buildFolderTreeDiffNodes', () => {
  it('maps live paths, hierarchy, statuses, direct counts, and changed descendants', () => {
    const nodes = buildFolderTreeDiffNodes([
      folder('/Contracts/Archive', 'left_only', 2),
      folder('/Contracts', 'equal'),
      folder('/Legal', 'right_only'),
      folder('/Contracts/Active', 'changed', 3),
    ])

    expect(nodes.map((node) => node.id)).toEqual(['/Contracts', '/Contracts/Active', '/Contracts/Archive', '/Legal'])
    expect(nodes[0]).toMatchObject({ parentId: undefined, status: 'equal', childCount: 2, hasChildren: true, hasChangedDescendants: true })
    expect(nodes[1]).toMatchObject({ parentId: '/Contracts', status: 'changed', metadata: 'files 3 → 4' })
    expect(nodes[1].before?.metadata).toEqual({ folders: 0, files: 3, webLinks: 0, other: 0 })
    expect(nodes[2]).toMatchObject({ parentId: '/Contracts', status: 'removed', metadata: '2 files' })
    expect(nodes[3]).toMatchObject({ parentId: undefined, status: 'added', metadata: 'Empty folder' })
  })
})
