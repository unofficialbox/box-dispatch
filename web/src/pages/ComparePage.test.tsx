// @vitest-environment jsdom
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ComparePage } from './ComparePage'
import type { BoxAIAgentComparison, BoxComparison, BoxFolderComparison, ConnectionSummary, DeploymentPlan } from '../types'

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

const folderComparison: BoxFolderComparison = {
  startedAt: '2026-10-26T12:00:00Z', completedAt: '2026-10-26T12:00:01Z',
  left: comparison.left, right: comparison.right,
  scope: { rootLabel: 'All Files', maxDepth: 4, maxFolders: 500, leftTruncated: false, rightTruncated: false },
  summary: { equal: 0, changed: 1, leftOnly: 0, rightOnly: 0 },
  folders: [{
    path: '/Contracts', name: 'Contracts', status: 'changed',
    changes: [{ path: 'items.files', before: 2, after: 3 }],
    left: { path: '/Contracts', name: 'Contracts', depth: 1, items: { folders: 1, files: 2, webLinks: 0, other: 0 } },
    right: { path: '/Contracts', name: 'Contracts', depth: 1, items: { folders: 1, files: 3, webLinks: 0, other: 0 } },
  }],
}

const aiAgentComparison: BoxAIAgentComparison = {
  startedAt: '2026-10-26T12:00:00Z', completedAt: '2026-10-26T12:00:01Z',
  left: comparison.left, right: comparison.right,
  summary: { equal: 0, changed: 1, leftOnly: 0, rightOnly: 0 },
  agents: [{
    key: 'contract extractor', name: 'Contract Extractor', product: 'AI Studio custom agent', capabilities: ['Extract'], status: 'changed',
    changes: [{ path: 'configuration.extract.long_text.model', before: 'model-a', after: 'model-b' }],
    left: { key: 'contract extractor', name: 'Contract Extractor', product: 'AI Studio custom agent', capabilities: ['Extract'], accessState: 'enabled', configuration: { extract: { description: 'Extract terms', long_text: { model: 'model-a' } } } },
    right: { key: 'contract extractor', name: 'Contract Extractor', product: 'AI Studio custom agent', capabilities: ['Extract'], accessState: 'enabled', configuration: { extract: { description: 'Extract terms', long_text: { model: 'model-b' } } } },
  }],
}

const promotableAIAgentComparison: BoxAIAgentComparison = {
  ...aiAgentComparison,
  summary: { equal: 0, changed: 0, leftOnly: 2, rightOnly: 0 },
  agents: [
    {
      key: 'legal assistant', name: 'Legal Assistant', product: 'AI Studio custom agent', capabilities: ['Ask'], status: 'left_only', changes: [],
      left: { key: 'legal assistant', name: 'Legal Assistant', product: 'AI Studio custom agent', capabilities: ['Ask'], accessState: 'enabled', configuration: { ask: { description: 'Answer legal questions' } } },
    },
    {
      key: 'summary writer', name: 'Summary Writer', product: 'AI Studio custom agent', capabilities: ['Text generation'], status: 'left_only', changes: [],
      left: { key: 'summary writer', name: 'Summary Writer', product: 'AI Studio custom agent', capabilities: ['Text generation'], accessState: 'enabled', configuration: { textGen: { description: 'Write summaries' } } },
    },
  ],
}

const promotableMetadataComparison: BoxComparison = {
  ...comparison,
  summary: { equal: 1, changed: 1, leftOnly: 1, rightOnly: 1 },
  templates: [
    {
      key: 'contracts', displayName: 'Contracts', status: 'changed',
      changes: [{ path: 'fields.status.options', before: ['Draft', 'Approved'], after: ['Draft'] }],
      left: { key: 'contracts', displayName: 'Contracts', hidden: false, copyInstanceOnItemCopy: false, fields: [{ key: 'status', type: 'enum', displayName: 'Status', hidden: false, options: ['Draft', 'Approved'] }] },
      right: { key: 'contracts', displayName: 'Contract records', hidden: false, copyInstanceOnItemCopy: false, fields: [{ key: 'status', type: 'enum', displayName: 'State', hidden: false, options: ['Draft'] }] },
    },
    {
      key: 'retention', displayName: 'Retention', status: 'left_only', changes: [],
      left: { key: 'retention', displayName: 'Retention', hidden: false, copyInstanceOnItemCopy: true, fields: [{ key: 'years', type: 'float', displayName: 'Years', description: 'Retention term', hidden: false, options: [] }] },
    },
    {
      key: 'vendor', displayName: 'Vendor', status: 'right_only', changes: [],
      right: { key: 'vendor', displayName: 'Vendor', hidden: false, copyInstanceOnItemCopy: false, fields: [] },
    },
    {
      key: 'equal', displayName: 'Equal', status: 'equal', changes: [],
      left: { key: 'equal', displayName: 'Equal', hidden: false, copyInstanceOnItemCopy: false, fields: [] },
      right: { key: 'equal', displayName: 'Equal', hidden: false, copyInstanceOnItemCopy: false, fields: [] },
    },
  ],
}

const preparedPlan: DeploymentPlan = {
  exists: true, name: 'AI agents to Sandbox', templateId: 'ai-agent-promotion', template: 'AI agent promotion',
  repository: 'box://enterprise/EID-prod/ai-agents', strategy: 'reuse',
  components: [{ id: 'box', name: 'Box', configured: true, verified: true, ready: true }],
}

const preparedMetadataPlan: DeploymentPlan = {
  ...preparedPlan,
  name: 'Metadata templates to Sandbox', templateId: 'metadata-template-promotion', template: 'Metadata template promotion',
  repository: 'box://enterprise/EID-prod/metadata-templates',
}

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

describe('ComparePage', () => {
  it('requires two saved Box connections', () => {
    render(<ComparePage connections={[{ ...connections[0], connections: connections[0].connections?.slice(0, 1) }]}/>)
    expect(document.querySelector('box-alert[heading="Two Box connections required"]')).toBeTruthy()
    expect(document.querySelector('box-button[label="Run live comparison"]')).toBeNull()
  })

  it('runs an exact live comparison and renders a Box Open Elements diff', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify(comparison), { status: 200, headers: { 'Content-Type': 'application/json' } }))
    const { container } = render(<ComparePage connections={connections}/>)
    expect(screen.queryByText('Live environment inspection')).toBeNull()
    expect(screen.queryByText('Comparison direction')).toBeNull()
    expect(screen.queryByText(/Query two Box enterprises now/)).toBeNull()
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
    expect(screen.queryByText('fields.status.options')).toBeNull()
    expect(container.querySelector('box-icon-button[label="Add Contracts to deployment plan"]')).toBeNull()
  })

  it('uses the unified diff mode on narrow viewports', async () => {
    vi.stubGlobal('matchMedia', vi.fn(() => ({
      matches: true,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    })))
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify(comparison), { status: 200, headers: { 'Content-Type': 'application/json' } }))
    const { container } = render(<ComparePage connections={connections}/>)

    fireEvent.click(container.querySelector('box-button[label="Run live comparison"]')!)

    await waitFor(() => expect(container.querySelector('box-diff-viewer[heading="contracts.json"]')).toBeTruthy())
    expect(container.querySelector('box-diff-viewer')?.getAttribute('mode')).toBe('inline')
  })

  it('switches to a bounded live folder comparison', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify(folderComparison), { status: 200, headers: { 'Content-Type': 'application/json' } }))
    const { container } = render(<ComparePage connections={connections}/>)
    const typeSelect = container.querySelector('box-select[label="Box object type"]')!
    fireEvent(typeSelect, new CustomEvent('value-changed', { bubbles: true, detail: { value: 'folders' } }))

    fireEvent.click(container.querySelector('box-button[label="Run live comparison"]')!)

    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith('/api/box/comparisons/folders', expect.objectContaining({ method: 'POST' })))
    await waitFor(() => expect(container.querySelector('box-diff-viewer[heading="folder-structure.json"]')).toBeTruthy())
    expect(screen.getByRole('heading', { name: 'Folders' })).toBeTruthy()
    expect(screen.queryByText('items.files')).toBeNull()
    expect(container.querySelector('box-diff-viewer')?.getAttribute('before-text')).toContain('/Contracts')
  })

  it('compares live AI Studio custom agents without calling them Box Extract Agents', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify(aiAgentComparison), { status: 200, headers: { 'Content-Type': 'application/json' } }))
    const { container } = render(<ComparePage connections={connections}/>)
    const typeSelect = container.querySelector('box-select[label="Box object type"]')!
    fireEvent(typeSelect, new CustomEvent('value-changed', { bubbles: true, detail: { value: 'ai_agents' } }))

    fireEvent.click(container.querySelector('box-button[label="Run live comparison"]')!)

    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith('/api/box/comparisons/ai-agents', expect.objectContaining({ method: 'POST' })))
    await waitFor(() => expect(container.querySelector('box-diff-viewer[heading="contract-extractor.json"]')).toBeTruthy())
    expect(screen.getByRole('heading', { name: 'AI Studio custom agents' })).toBeTruthy()
    expect(screen.queryByText('configuration.extract.long_text.model')).toBeNull()
    expect(container.querySelector('box-diff-viewer')?.getAttribute('before-text')).toContain('Extract')
  })

  it('prepares a deployment from supported source-only AI agents', async () => {
    const onPromotionPrepared = vi.fn()
    const fetchMock = vi.spyOn(globalThis, 'fetch')
      .mockResolvedValueOnce(new Response(JSON.stringify(promotableAIAgentComparison), { status: 200, headers: { 'Content-Type': 'application/json' } }))
      .mockResolvedValueOnce(new Response(JSON.stringify(preparedPlan), { status: 201, headers: { 'Content-Type': 'application/json' } }))
    const { container } = render(<ComparePage connections={connections} onPromotionPrepared={onPromotionPrepared}/>)
    fireEvent(container.querySelector('box-select[label="Box object type"]')!, new CustomEvent('value-changed', { bubbles: true, detail: { value: 'ai_agents' } }))
    fireEvent.click(container.querySelector('box-button[label="Run live comparison"]')!)

    await waitFor(() => expect(container.querySelector('box-button[label="Prepare deployment"]')).toBeTruthy())
    expect(screen.getByText('0 of 1 selected')).toBeTruthy()
    expect(container.querySelector('box-button[label="Prepare deployment"]')?.hasAttribute('disabled')).toBe(true)
    fireEvent.click(container.querySelector('box-icon-button[icon="plus"][label="Add Legal Assistant to deployment plan"]')!)
    expect(screen.getByText('1 of 1 selected')).toBeTruthy()
    const removeButton = container.querySelector('box-icon-button[icon="cart-1"][label="Remove Legal Assistant from deployment plan"]')
    expect(removeButton).toBeTruthy()
    expect(container.querySelector('box-button[label="Remove from plan"]')).toBeNull()
    fireEvent.click(container.querySelector('box-button[label="Prepare deployment"]')!)

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2))
    expect(fetchMock.mock.calls[1][0]).toBe('/api/box/promotions/ai-agents')
    expect(JSON.parse(String((fetchMock.mock.calls[1][1] as RequestInit).body))).toEqual({
      name: 'AI agents to Sandbox', leftConnectionId: 'box-prod', rightConnectionId: 'box-test', agentKeys: ['legal assistant'],
    })
    await waitFor(() => expect(onPromotionPrepared).toHaveBeenCalledWith(preparedPlan))
  })

  it('focuses the list on changed and source-only templates and prepares selected templates for deployment', async () => {
    const onPromotionPrepared = vi.fn()
    const fetchMock = vi.spyOn(globalThis, 'fetch')
      .mockResolvedValueOnce(new Response(JSON.stringify(promotableMetadataComparison), { status: 200, headers: { 'Content-Type': 'application/json' } }))
      .mockResolvedValueOnce(new Response(JSON.stringify(preparedMetadataPlan), { status: 201, headers: { 'Content-Type': 'application/json' } }))
    const { container } = render(<ComparePage connections={connections} onPromotionPrepared={onPromotionPrepared}/>)
    fireEvent.click(container.querySelector('box-button[label="Run live comparison"]')!)

    await waitFor(() => expect(container.querySelector('box-icon-button[icon="plus"][label="Add Contracts to deployment plan"]')).toBeTruthy())
    expect(container.querySelector('box-icon-button[icon="plus"][label="Add Retention to deployment plan"]')).toBeTruthy()
    const table = container.querySelector('box-table[label="Metadata template comparison results"]') as HTMLElement & { rows: Array<{ id: string }> }
    expect(table.rows.map((row) => row.id)).toEqual(['contracts', 'retention'])
    expect(screen.getByText('Showing 2 of 4')).toBeTruthy()
    expect(screen.getByText('0 of 2 selected')).toBeTruthy()

    fireEvent.click(container.querySelector('box-icon-button[icon="plus"][label="Add Contracts to deployment plan"]')!)
    expect(screen.getByText('1 of 2 selected')).toBeTruthy()

    fireEvent(container.querySelector('box-text-field[label="Filter results"]')!, new CustomEvent('value-changed', { bubbles: true, detail: { value: 'retention' } }))
    await waitFor(() => expect(table.rows.map((row) => row.id)).toEqual(['retention']))
    fireEvent.click(container.querySelector('box-icon-button[icon="plus"][label="Add Retention to deployment plan"]')!)
    expect(screen.getByText('2 of 2 selected')).toBeTruthy()
    fireEvent.click(container.querySelector('box-button[label="Prepare deployment"]')!)

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2))
    expect(fetchMock.mock.calls[1][0]).toBe('/api/box/promotions/metadata-templates')
    expect(JSON.parse(String((fetchMock.mock.calls[1][1] as RequestInit).body))).toEqual({
      name: 'Metadata templates to Sandbox', leftConnectionId: 'box-prod', rightConnectionId: 'box-test', templateKeys: ['contracts', 'retention'],
    })
    await waitFor(() => expect(onPromotionPrepared).toHaveBeenCalledWith(preparedMetadataPlan))
  })
})
