// @vitest-environment jsdom
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { FolderScopePicker } from './FolderScopePicker'

afterEach(() => vi.restoreAllMocks())

describe('FolderScopePicker', () => {
  it('loads branches lazily and searches live beyond visible nodes', async () => {
    const onSelected = vi.fn()
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockImplementation(async (input, init) => {
      const url = String(input)
      const body = JSON.parse(String(init?.body ?? '{}')) as { parentId?: string; query?: string }
      if (url.endsWith('/api/box/folders/search')) {
        return new Response(JSON.stringify({ folders: [{ id: 'archive', name: 'Archive 2024' }], partial: true }), { status: 200, headers: { 'Content-Type': 'application/json' } })
      }
      const folders = body.parentId === 'contracts' ? [{ id: 'executed', name: 'Executed' }] : [{ id: 'contracts', name: 'Contracts' }]
      return new Response(JSON.stringify({ parentId: body.parentId, folders, partial: false }), { status: 200, headers: { 'Content-Type': 'application/json' } })
    })
    const { container } = render(<FolderScopePicker connectionId="box-prod" label="Source root" selected={{ id: '0', name: 'All Files' }} disabled={false} onSelected={onSelected}/>)
    const tree = container.querySelector('box-folder-tree') as HTMLElementTagNameMap['box-folder-tree']
    await waitFor(() => expect(tree.nodes.map((node) => node.name)).toEqual(['All Files', 'Contracts']))

    fireEvent(tree, new CustomEvent('load-children', { detail: { id: 'contracts', node: tree.nodes[1] } }))
    await waitFor(() => expect(tree.nodes.map((node) => node.name)).toContain('Executed'))
    expect(fetchMock.mock.calls.some(([input, init]) => String(input).endsWith('/api/box/folders/children') && JSON.parse(String((init as RequestInit).body)).parentId === 'contracts')).toBe(true)

    fireEvent(container.querySelector('box-text-field[label="Find a folder"]')!, new CustomEvent('value-changed', { detail: { value: 'archive' } }))
    await waitFor(() => expect(fetchMock.mock.calls.some(([input]) => String(input).endsWith('/api/box/folders/search'))).toBe(true))
    await waitFor(() => expect(tree.nodes.map((node) => node.name)).toEqual(['Archive 2024']))
    expect(screen.getByText('More folders are available. Narrow the search or expand a branch.')).toBeTruthy()

    fireEvent(tree, new CustomEvent('selected-changed', { detail: { id: 'archive', node: tree.nodes[0], selectedIds: ['archive'] } }))
    expect(onSelected).toHaveBeenCalledWith({ id: 'archive', name: 'Archive 2024' })
  })

  it('aborts pending provider work when removed', async () => {
    const request = { signal: null as AbortSignal | null }
    vi.spyOn(globalThis, 'fetch').mockImplementation(async (_input, init) => {
      request.signal = init?.signal ?? null
      return new Promise<Response>((_, reject) => request.signal?.addEventListener('abort', () => reject(new DOMException('Aborted', 'AbortError')), { once: true }))
    })
    const { unmount } = render(<FolderScopePicker connectionId="box-prod" label="Source root" selected={{ id: '0', name: 'All Files' }} disabled={false} onSelected={vi.fn()}/>)
    await waitFor(() => expect(request.signal).not.toBeNull())
    unmount()
    expect(request.signal?.aborted).toBe(true)
  })
})
