// @vitest-environment jsdom
import { fireEvent, render, within } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { Sidebar } from './Sidebar'

describe('Sidebar', () => {
  beforeEach(() => window.localStorage.clear())

  it('exposes only the current route and updates it after navigation', () => {
    const callbacks = { onOverview: vi.fn(), onNewDeployment: vi.fn(), onHistory: vi.fn(), onSettings: vi.fn() }
    const { container, rerender } = render(<Sidebar activeView="overview" {...callbacks}/>)
    for (const [activeView, name] of [['overview', 'Overview'], ['workflow', 'Deployments'], ['history', 'Deployment history'], ['settings', 'Settings']] as const) {
      rerender(<Sidebar activeView={activeView} {...callbacks}/>)
      expect(container.querySelectorAll('[aria-current]')).toHaveLength(1)
      expect(within(container).getByRole('link', { name }).getAttribute('aria-current')).toBe('page')
    }
  })

  it('uses the clock-2 icon for deployment history', () => {
    const { container } = render(<Sidebar activeView="overview" onOverview={vi.fn()} onNewDeployment={vi.fn()} onHistory={vi.fn()} onSettings={vi.fn()}/>)

    const history = within(container).getByRole('link', { name: 'Deployment history' })
    expect(history.querySelector('.boe-rail-icon')?.innerHTML).toContain('M75 21V72.99L97.22 94.72')
    expect(container.querySelector('box-nav-sidebar')?.hasAttribute('collapsed')).toBe(false)
  })

  it('keeps explicit route names when collapsed styles hide the visual labels', () => {
    const { container } = render(<Sidebar activeView="overview" onOverview={vi.fn()} onNewDeployment={vi.fn()} onHistory={vi.fn()} onSettings={vi.fn()}/>)

    for (const name of ['Overview', 'Deployments', 'Deployment history', 'Settings']) {
      expect(within(container).getByRole('link', { name }).getAttribute('aria-label')).toBe(name)
    }
  })

  it('persists the compact preference while keeping the library toggle reachable', () => {
    const { container } = render(<Sidebar activeView="overview" onOverview={vi.fn()} onNewDeployment={vi.fn()} onHistory={vi.fn()} onSettings={vi.fn()}/>)
    const sidebar = container.querySelector('box-nav-sidebar')
    const toggle = container.querySelector('box-sidebar-toggle-button')

    expect(toggle?.getAttribute('label')).toBe('Collapse navigation')
    expect(toggle?.getAttribute('slot')).toBe('header')
    fireEvent(toggle!, new CustomEvent('toggle', { detail: { expanded: false } }))
    expect(sidebar?.hasAttribute('collapsed')).toBe(true)
    expect(toggle?.getAttribute('label')).toBe('Expand navigation')
    expect(window.localStorage.getItem('dispatch-sidebar-collapsed')).toBe('true')
  })
})
