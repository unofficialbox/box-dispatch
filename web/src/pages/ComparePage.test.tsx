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
    key: 'contract extractor', name: 'Contract Extractor', kind: 'Extract', status: 'changed',
    changes: [{ path: 'configuration.extract.long_text.model', before: 'model-a', after: 'model-b' }],
    left: { key: 'contract extractor', name: 'Contract Extractor', kind: 'Extract', accessState: 'enabled', configuration: { extract: { description: 'Extract terms', long_text: { model: 'model-a' } } } },
    right: { key: 'contract extractor', name: 'Contract Extractor', kind: 'Extract', accessState: 'enabled', configuration: { extract: { description: 'Extract terms', long_text: { model: 'model-b' } } } },
  }],
}

const promotableAIAgentComparison: BoxAIAgentComparison = {
  ...aiAgentComparison,
  summary: { equal: 0, changed: 0, leftOnly: 2, rightOnly: 0 },
  agents: [
    {
      key: 'legal assistant', name: 'Legal Assistant', kind: 'Ask', status: 'left_only', changes: [],
      left: { key: 'legal assistant', name: 'Legal Assistant', kind: 'Ask', accessState: 'enabled', configuration: { ask: { description: 'Answer legal questions' } } },
    },
    {
      key: 'summary writer', name: 'Summary Writer', kind: 'Text generation', status: 'left_only', changes: [],
      left: { key: 'summary writer', name: 'Summary Writer', kind: 'Text generation', accessState: 'enabled', configuration: { textGen: { description: 'Write summaries' } } },
    },
  ],
}

const preparedPlan: DeploymentPlan = {
  exists: true, name: 'AI agents to Sandbox', templateId: 'ai-agent-promotion', template: 'AI agent promotion',
  repository: 'box://enterprise/EID-prod/ai-agents', strategy: 'reuse',
  components: [{ id: 'box', name: 'Box', configured: true, verified: true, ready: true }],
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

  it('switches to a bounded live folder comparison', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify(folderComparison), { status: 200, headers: { 'Content-Type': 'application/json' } }))
    const { container } = render(<ComparePage connections={connections}/>)
    const typeSelect = container.querySelector('box-select[label="Box object type"]')!
    fireEvent(typeSelect, new CustomEvent('value-changed', { bubbles: true, detail: { value: 'folders' } }))

    expect(screen.getByText(/All Files hierarchy to 4 levels/)).toBeTruthy()
    fireEvent.click(container.querySelector('box-button[label="Run live comparison"]')!)

    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith('/api/box/comparisons/folders', expect.objectContaining({ method: 'POST' })))
    await waitFor(() => expect(container.querySelector('box-diff-viewer[heading="folder-structure.json"]')).toBeTruthy())
    expect(screen.getByRole('heading', { name: 'Folders' })).toBeTruthy()
    expect(screen.getByText('items.files')).toBeTruthy()
    expect(screen.getByText('/Contracts')).toBeTruthy()
  })

  it('compares live AI agents and extract-mode configurations', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify(aiAgentComparison), { status: 200, headers: { 'Content-Type': 'application/json' } }))
    const { container } = render(<ComparePage connections={connections}/>)
    const typeSelect = container.querySelector('box-select[label="Box object type"]')!
    fireEvent(typeSelect, new CustomEvent('value-changed', { bubbles: true, detail: { value: 'ai_agents' } }))

    expect(screen.getByText(/current custom AI Studio agents/)).toBeTruthy()
    fireEvent.click(container.querySelector('box-button[label="Run live comparison"]')!)

    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith('/api/box/comparisons/ai-agents', expect.objectContaining({ method: 'POST' })))
    await waitFor(() => expect(container.querySelector('box-diff-viewer[heading="contract-extractor.json"]')).toBeTruthy())
    expect(screen.getByRole('heading', { name: 'AI agents and extraction configurations' })).toBeTruthy()
    expect(screen.getByText('configuration.extract.long_text.model')).toBeTruthy()
    expect(screen.getByText('Extraction configuration')).toBeTruthy()
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
    expect(screen.getByText('1 of 1 deployable agents selected for Sandbox.')).toBeTruthy()
    const removeButton = container.querySelector('box-icon-button[icon="cart-1"][label="Remove Legal Assistant from deployment plan"]')
    expect(removeButton).toBeTruthy()
    expect(container.querySelector('box-button[label="Remove from plan"]')).toBeNull()
    fireEvent.click(removeButton!)
    expect(screen.getByText('0 of 1 deployable agents selected for Sandbox.')).toBeTruthy()
    expect(container.querySelector('box-button[label="Prepare deployment"]')?.hasAttribute('disabled')).toBe(true)
    fireEvent.click(container.querySelector('box-button[label="Add to plan"]')!)
    expect(screen.getByText('1 of 1 deployable agents selected for Sandbox.')).toBeTruthy()
    fireEvent.click(container.querySelector('box-button[label="Prepare deployment"]')!)

    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2))
    expect(fetchMock.mock.calls[1][0]).toBe('/api/box/promotions/ai-agents')
    expect(JSON.parse(String((fetchMock.mock.calls[1][1] as RequestInit).body))).toEqual({
      name: 'AI agents to Sandbox', leftConnectionId: 'box-prod', rightConnectionId: 'box-test', agentKeys: ['legal assistant'],
    })
    await waitFor(() => expect(onPromotionPrepared).toHaveBeenCalledWith(preparedPlan))
  })
})
