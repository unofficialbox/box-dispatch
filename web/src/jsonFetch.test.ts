import { afterEach, describe, expect, it, vi } from 'vitest'
import { fetchJSON, fetchOptionalJSON } from './jsonFetch'

afterEach(() => vi.unstubAllGlobals())

describe('JSON fetch helpers', () => {
  it('returns required JSON responses', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({ status: 'ready' }), { status: 200 })))

    await expect(fetchJSON<{ status: string }>('/api/required', new AbortController().signal)).resolves.toEqual({ status: 'ready' })
  })

  it('represents an empty optional response as null', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(null, { status: 204 })))

    await expect(fetchOptionalJSON('/api/optional', new AbortController().signal)).resolves.toBeNull()
  })

  it('still rejects failed optional responses', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(null, { status: 500 })))

    await expect(fetchOptionalJSON('/api/optional', new AbortController().signal)).rejects.toThrow('/api/optional is unavailable.')
  })
})
