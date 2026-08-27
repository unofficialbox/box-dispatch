// @vitest-environment jsdom
import { render, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { Sidebar } from './Sidebar'

describe('Sidebar', () => {
  it('uses the clock-2 icon for deployment history', () => {
    const { container } = render(<Sidebar activeView="overview" onOverview={vi.fn()} onNewDeployment={vi.fn()} onHistory={vi.fn()} onSettings={vi.fn()}/>)

    const history = within(container).getByRole('button', { name: 'Deployment history' })
    expect(history.querySelector('.boe-rail-icon')?.innerHTML).toContain('M75 21V72.99L97.22 94.72')
  })
})
