// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest'
import { DISPATCH_DARK_THEME, DISPATCH_LIGHT_THEME, startDispatchTheme } from './theme'

const mediaPreference = (dark: boolean) => {
  const listeners = new Set<(event: MediaQueryListEvent) => void>()
  const media = {
    matches: dark,
    media: '(prefers-color-scheme: dark)',
    onchange: null,
    addEventListener: (_type: string, listener: (event: MediaQueryListEvent) => void) => listeners.add(listener),
    removeEventListener: (_type: string, listener: (event: MediaQueryListEvent) => void) => listeners.delete(listener),
    addListener: vi.fn(),
    removeListener: vi.fn(),
    dispatchEvent: vi.fn(),
  } as unknown as MediaQueryList

  return {
    media,
    setDark(next: boolean) {
      Object.defineProperty(media, 'matches', { configurable: true, value: next })
      const event = { matches: next, media: media.media } as MediaQueryListEvent
      listeners.forEach((listener) => listener(event))
    },
  }
}

describe('Dispatch theme', () => {
  it('follows the system preference and updates Box design tokens', () => {
    const preference = mediaPreference(true)
    vi.stubGlobal('matchMedia', () => preference.media)
    window.localStorage.clear()

    const controller = startDispatchTheme()
    expect(controller.getPreference()).toBe('system')
    expect(controller.getResolvedTheme()).toBe('dark')
    expect(document.documentElement.getAttribute('data-theme')).toBe('dark')
    expect(document.documentElement.style.colorScheme).toBe('dark')
    expect(document.documentElement.style.getPropertyValue('--boe-token-surface-surface')).toBe('#121513')
    expect(document.documentElement.style.getPropertyValue('--boe-token-surface-surface-brand')).toBe('#00e581')
    expect(document.documentElement.style.getPropertyValue('--boe-token-surface-illustration-surface-box-neutral')).toBe('#00e581')
    expect(document.documentElement.style.getPropertyValue('--boe-token-surface-status-surface-inprogress')).toBe('#ffa300')
    expect(document.documentElement.style.getPropertyValue('--boe-token-surface-status-surface-accent')).toBe('#8b49cf')
    expect(document.documentElement.style.getPropertyValue('--boe-token-surface-badge-foldershared-surface')).toBe('#0098ff')

    preference.setDark(false)
    expect(controller.getResolvedTheme()).toBe('light')
    expect(document.documentElement.getAttribute('data-theme')).toBe('light')
    expect(document.documentElement.style.colorScheme).toBe('light')
    expect(document.documentElement.style.getPropertyValue('--boe-token-surface-surface')).toBe('#ffffff')
    expect(document.documentElement.style.getPropertyValue('--boe-token-surface-surface-brand')).toBe('#007a4c')
    expect(document.documentElement.style.getPropertyValue('--boe-token-surface-illustration-surface-box-neutral')).toBe('#007a4c')
    expect(document.documentElement.style.getPropertyValue('--boe-token-surface-status-surface-inprogress')).toBe('#ffa300')
    expect(document.documentElement.style.getPropertyValue('--boe-token-surface-status-surface-accent')).toBe('#8b49cf')
    expect(document.documentElement.style.getPropertyValue('--boe-token-surface-badge-foldershared-surface')).toBe('#0098ff')

    controller.stop()
    vi.unstubAllGlobals()
  })

  it('uses named Dispatch design systems when the preference changes', () => {
    const preference = mediaPreference(false)
    vi.stubGlobal('matchMedia', () => preference.media)
    window.localStorage.clear()
    const events: string[] = []
    const listener = (event: Event) => events.push((event as CustomEvent<{ designSystemName: string }>).detail.designSystemName)
    document.documentElement.addEventListener('boe:theme-change', listener)

    const controller = startDispatchTheme()
    expect(events.at(-1)).toBe(DISPATCH_LIGHT_THEME)

    preference.setDark(true)
    expect(events.at(-1)).toBe(DISPATCH_DARK_THEME)

    controller.stop()
    document.documentElement.removeEventListener('boe:theme-change', listener)
    vi.unstubAllGlobals()
  })
})
