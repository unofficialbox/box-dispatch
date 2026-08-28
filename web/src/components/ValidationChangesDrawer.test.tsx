// @vitest-environment jsdom
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { ValidationChangesDrawer } from './ValidationChangesDrawer'

describe('ValidationChangesDrawer', () => {
  it('shows file-level changes and switches the diff preview', () => {
    const { container } = render(<ValidationChangesDrawer loading={false} error="" onClose={vi.fn()} files={[
      { component: 'Settings:Communities', path: 'settings/Communities.settings-meta.xml', kind: 'update', before: '<enabled>false</enabled>', after: '<enabled>true</enabled>', previewable: true },
      { component: 'UIBundle:clmreactapp', path: 'uiBundles/clmreactapp/archive.zip', kind: 'add', previewable: false },
    ]} />)

    expect(screen.getByText('2 files')).toBeTruthy()
    const viewer = container.querySelector('box-diff-viewer')
    expect(viewer?.getAttribute('heading')).toBe('settings/Communities.settings-meta.xml')
    expect(viewer?.getAttribute('before-label')).toBe('Current org')
    expect(viewer?.getAttribute('after-label')).toBe('Validated package')
    expect(viewer?.getAttribute('before-text')).toContain('<enabled>false</enabled>')
    expect(viewer?.getAttribute('after-text')).toContain('<enabled>true</enabled>')
    fireEvent.click(screen.getByText('archive.zip'))
    expect(screen.getByText('Preview unavailable')).toBeTruthy()
  })

  it('uses recorded before and after labels for a completed deployment', () => {
    const { container } = render(<ValidationChangesDrawer stage="deployment" loading={false} error="" onClose={vi.fn()} files={[
      { component: 'Settings:Communities', path: 'settings/Communities.settings-meta.xml', kind: 'update', before: 'false', after: 'true', previewable: true },
    ]} />)

    const drawer = container.querySelector('box-drawer')
    const viewer = container.querySelector('box-diff-viewer')
    expect(drawer?.getAttribute('heading')).toBe('Review deployed changes')
    expect(viewer?.getAttribute('before-label')).toBe('Before deployment')
    expect(viewer?.getAttribute('after-label')).toBe('After deployment')
  })
})
