// @vitest-environment jsdom
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ComparePage } from './ComparePage'
import type { BoxComparison, ConnectionSummary } from '../types'

const connections: ConnectionSummary[] = [{
  name: 'Box', configured: true, verified: true,
  connections: [
    { id: 'box-prod', alias: 'Production', status: 'Ready', selected: true, enterpriseId: 'EID-prod', identity: 'owner@example.test', domain: 'prod.app.box.com' },
    { id: 'box-test', alias: 'Sandbox', status: 'Ready', selected: false, enterpriseId: 'EID-test', identity: 'tester@example.test', domain: 'test.app.box.com' },
  ],
}]

const comparison: BoxComparison = {
  startedAt: '2026-10-26T12:00:00Z', completedAt: '2026-10-26T12:00:01Z',
  left: { connectionId: 'box-prod', alias: 'Production', identity: 'owner@example.test', enterpriseId: 'EID-prod', domain: 'prod.app.box.com', capturedAt: '2026-10-26T12:00:00Z' },
  right: { connectionId: 'box-test', alias: 'Sandbox', identity: 'tester@example.test', enterpriseId: 'EID-test', domain: 'test.app.box.com', capturedAt: '2026-10-26T12:00:00Z' },
  summary: { equal: 2, changed: 1, leftOnly: 0, rightOnly: 0 },
  templates: [{
    key: 'contracts', displayName: 'Contracts', status: 'changed',
    changes: [{ path: 'fields.status.options', before: ['Draft'], after: ['Draft', 'Approved'] }],
    left: { key: 'contracts', displayName: 'Contracts', hidden: false, copyInstanceOnItemCopy: false, fields: [{ key: 'status', type: 'enum', displayName: 'Status', hidden: false, options: ['Draft'] }] },
    right: { key: 'contracts', displayName: 'Contracts', hidden: false, copyInstanceOnItemCopy: false, fields: [{ key: 'status', type: 'enum', displayName: 'Status', hidden: false, options: ['Draft', 'Approved'] }] },
  }],
}

afterEach(() => vi.restoreAllMocks())

describe('ComparePage', () => {
  it('requires two saved Box connections', () => {
    render(<ComparePage connections={[{ ...connections[0], connections: connections[0].connections?.slice(0, 1) }]}/>)
    expect(document.querySelector('box-alert[heading="Two Box connections required"]')).toBeTruthy()
    expect(document.querySelector('box-button[label="Run live comparison"]')).toBeNull()
  })

  it('runs an exact live comparison and renders a Box Open Elements diff', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify(comparison), { status: 200, headers: { 'Content-Type': 'application/json' } }))
    const { container } = render(<ComparePage connections={connections}/>)
    await waitFor(() => expect(container.querySelector('box-select[label="Source Box connection"]')?.getAttribute('value')).toBe('box-prod'))
    await waitFor(() => expect(container.querySelector('box-select[label="Destination Box connection"]')?.getAttribute('value')).toBe('box-test'))

    fireEvent.click(container.querySelector('box-button[label="Run live comparison"]')!)

    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith('/api/box/comparisons', expect.objectContaining({ method: 'POST' })))
    expect(JSON.parse(String((fetchMock.mock.calls[0][1] as RequestInit).body))).toEqual({ leftConnectionId: 'box-prod', rightConnectionId: 'box-test' })
    await waitFor(() => expect(container.querySelector('box-diff-viewer[heading="contracts.json"]')).toBeTruthy())
    const viewer = container.querySelector('box-diff-viewer')!
    expect(viewer.getAttribute('before-label')).toBe('Production')
    expect(viewer.getAttribute('after-label')).toBe('Sandbox')
    expect(viewer.getAttribute('before-text')).toContain('"Draft"')
    expect(viewer.getAttribute('after-text')).toContain('"Approved"')
    expect(screen.getByText('fields.status.options')).toBeTruthy()
  })
})
