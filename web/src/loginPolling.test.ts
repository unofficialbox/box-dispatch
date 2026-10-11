// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { watchLoginJob } from './loginPolling'

afterEach(() => { vi.useRealTimers(); vi.unstubAllGlobals() })
const response = (status: string) => new Response(JSON.stringify({ status, message: status }))

describe('login polling lifecycle', () => {
  it('cancels a pending closed popup and permits a separate new watcher', async () => {
    vi.useFakeTimers()
    vi.stubGlobal('fetch', vi.fn().mockImplementation(async () => response('pending')))
    const popup = { closed: false }
    const updates = vi.fn()
    const result = watchLoginJob('/local/login', popup, new AbortController(), updates).catch((error: Error) => error.message)
    await vi.advanceTimersByTimeAsync(1)
    popup.closed = true
    await vi.advanceTimersByTimeAsync(300)
    expect(await result).toContain('Login window closed')
    expect(vi.getTimerCount()).toBe(0)
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response('active')))
    await watchLoginJob('/local/new-login', { closed: false }, new AbortController(), updates)
    expect(updates).toHaveBeenLastCalledWith({ status: 'active', message: 'active' })
  })

  it('recognizes a successful callback that closes during a pending poll', async () => {
    vi.useFakeTimers()
    const fetchJob = vi.fn().mockResolvedValueOnce(response('pending')).mockResolvedValue(response('active'))
    vi.stubGlobal('fetch', fetchJob)
    const popup = { closed: false }
    const updates = vi.fn()
    const result = watchLoginJob('/local/login', popup, new AbortController(), updates)
    await vi.advanceTimersByTimeAsync(1)
    popup.closed = true
    await vi.advanceTimersByTimeAsync(300)
    await result
    expect(updates).toHaveBeenLastCalledWith({ status: 'active', message: 'active' })
    expect(vi.getTimerCount()).toBe(0)
  })

  it('bounds a pending job and clears its timers on timeout', async () => {
    vi.useFakeTimers()
    vi.stubGlobal('fetch', vi.fn().mockImplementation(async () => response('pending')))
    const result = watchLoginJob('/local/login', { closed: false }, new AbortController(), vi.fn()).catch((error: Error) => error.message)
    await vi.advanceTimersByTimeAsync(120_000)
    expect(await result).toContain('Login timed out')
    expect(vi.getTimerCount()).toBe(0)
  })

  it('discards an old response after drawer cancellation', async () => {
    let resolve!: (response: Response) => void
    vi.stubGlobal('fetch', vi.fn(() => new Promise<Response>((done) => { resolve = done })))
    const controller = new AbortController()
    const updates = vi.fn()
    const result = watchLoginJob('/local/login', { closed: false }, controller, updates).catch(() => undefined)
    controller.abort()
    resolve(response('active'))
    await result
    expect(updates).not.toHaveBeenCalled()
  })
})
