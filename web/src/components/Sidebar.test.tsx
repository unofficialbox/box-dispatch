// @vitest-environment jsdom
import { fireEvent, render, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { Sidebar } from './Sidebar'

describe('Sidebar', () => {
  it('uses the clock-2 icon for deployment history', () => {
    const { container } = render(<Sidebar activeView="overview" onOverview={vi.fn()} onNewDeployment={vi.fn()} onHistory={vi.fn()} onSettings={vi.fn()}/>)

    const history = within(container).getByRole('button', { name: 'Deployment history' })
    expect(history.querySelector('.boe-rail-icon')?.innerHTML).toContain('M75 21V72.99L97.22 94.72')
    expect(container.querySelector('box-nav-sidebar')?.hasAttribute('collapsed')).toBe(true)
  })

  it('keeps the sidebar toggle reachable while collapsed and expands from its event', () => {
    const { container } = render(<Sidebar activeView="overview" onOverview={vi.fn()} onNewDeployment={vi.fn()} onHistory={vi.fn()} onSettings={vi.fn()}/>)
    const sidebar = container.querySelector('box-nav-sidebar')
    const toggle = container.querySelector('box-sidebar-toggle-button')

    expect(toggle?.getAttribute('label')).toBe('Expand navigation')
    fireEvent(toggle!, new CustomEvent('toggle', { detail: { expanded: true } }))
    expect(sidebar?.hasAttribute('collapsed')).toBe(false)
    expect(toggle?.getAttribute('label')).toBe('Collapse navigation')
  })
})
