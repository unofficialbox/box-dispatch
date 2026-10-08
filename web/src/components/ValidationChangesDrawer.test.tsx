// @vitest-environment jsdom
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { TableRow } from '@unofficialbox/box-open-elements/table'
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
    const table = container.querySelector('box-table.validation-change-table') as HTMLElement & { rows: TableRow[]; selectedIds: string[] }
    expect(table.rows).toHaveLength(2)
    expect(table.rows[1]).toMatchObject({ detail: 'uiBundles/clmreactapp/archive.zip' })
    fireEvent(table, new CustomEvent('selection-changed', { detail: { selectedIds: ['UIBundle:clmreactapp:uiBundles/clmreactapp/archive.zip'] } }))
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

  it('keeps selection valid across repeated changes and a replacement file set', () => {
    const files = [
      { component: 'Settings:Communities', path: 'settings/Communities.settings-meta.xml', kind: 'update' as const, before: 'false', after: 'true', previewable: true },
      { component: 'UIBundle:clmreactapp', path: 'uiBundles/clmreactapp/archive.zip', kind: 'add' as const, previewable: false },
    ]
    const { container, rerender } = render(<ValidationChangesDrawer loading={false} error="" onClose={vi.fn()} files={files}/>)
    const table = container.querySelector('box-table.validation-change-table') as HTMLElement & { selectedIds: string[] }

    fireEvent(table, new CustomEvent('selection-changed', { detail: { selectedIds: ['UIBundle:clmreactapp:uiBundles/clmreactapp/archive.zip'] } }))
    expect(table.selectedIds).toEqual(['UIBundle:clmreactapp:uiBundles/clmreactapp/archive.zip'])
    fireEvent(table, new CustomEvent('selection-changed', { detail: { selectedIds: ['Settings:Communities:settings/Communities.settings-meta.xml'] } }))
    expect(table.selectedIds).toEqual(['Settings:Communities:settings/Communities.settings-meta.xml'])

    rerender(<ValidationChangesDrawer loading={false} error="" onClose={vi.fn()} files={[{ component: 'ApexClass:Renewal', path: 'classes/Renewal.cls', kind: 'add', before: '', after: 'public class Renewal {}', previewable: true }]}/>)
    expect(table.selectedIds).toEqual(['ApexClass:Renewal:classes/Renewal.cls'])
    expect(container.querySelector('box-diff-viewer')?.getAttribute('heading')).toBe('classes/Renewal.cls')
  })
})
