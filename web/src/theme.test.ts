// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest'
import { startDispatchTheme } from './theme'

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
    expect(document.documentElement.style.getPropertyValue('--boe-token-surface-surface')).toBe('#1c1c1c')

    preference.setDark(false)
    expect(controller.getResolvedTheme()).toBe('light')
    expect(document.documentElement.getAttribute('data-theme')).toBe('light')
    expect(document.documentElement.style.colorScheme).toBe('light')
    expect(document.documentElement.style.getPropertyValue('--boe-token-surface-surface')).toBe('#ffffff')

    controller.stop()
    vi.unstubAllGlobals()
  })
})
