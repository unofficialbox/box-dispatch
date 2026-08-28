// @vitest-environment jsdom
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SettingsPage } from './SettingsPage'
import type { ConnectionSummary, DeploymentDefaults } from '../types'

const defaults: DeploymentDefaults = { templateId: 'clm', template: 'Contract Lifecycle Management', repository: 'https://example.com/clm', strategy: 'reuse', components: ['box', 'salesforce'] }
const connections: ConnectionSummary[] = [
  { name: 'Box', configured: true, verified: true, connections: [{ id: 'box-1', alias: 'Production Box', status: 'Ready', selected: true, identity: 'owner@example.com', domain: 'acme.app.box.com' }, { id: 'box-2', alias: 'Sandbox Box', status: 'Not ready', selected: false }] },
  { name: 'Salesforce', configured: true, verified: true, orgs: [{ id: 'sf-1', alias: 'CLM Scratch', kind: 'Scratch org', status: 'Ready', selected: true, username: 'test@example.com', orgId: '00D000000000001', domain: 'example.scratch.my.salesforce.com' }, { id: 'sf-2', alias: 'Production Dev Hub', kind: 'Dev Hub', status: 'Ready', selected: false, devHub: true }] },
]

const connectionActions = {
  onRemoveBoxConnection: vi.fn(async () => true),
  onRemoveSalesforceConnection: vi.fn(async () => true),
}

describe('SettingsPage', () => {
  it('lists every saved connection and deployment defaults without connection readiness', () => {
    const { container } = render(<SettingsPage defaults={defaults} connections={connections} onSaveDefaults={vi.fn()} onBoxConnection={vi.fn()} onSalesforceConnection={vi.fn()} {...connectionActions}/>)
    const view = within(container)

    expect(screen.getByText('Production Box')).toBeTruthy()
    expect(screen.getByText('Sandbox Box')).toBeTruthy()
    expect(screen.getByText('CLM Scratch')).toBeTruthy()
    expect(screen.getByText('Production Dev Hub')).toBeTruthy()
    expect(screen.getByText('acme.app.box.com')).toBeTruthy()
    expect(screen.getByText('example.scratch.my.salesforce.com')).toBeTruthy()
    expect(screen.getByRole('heading', { name: 'Defaults' })).toBeTruthy()
    expect(screen.getByText('Starting configuration for new deployments. Each deployment can override these choices.')).toBeTruthy()
    expect(view.getByRole('radio', { name: /Contract Lifecycle Management \(CLM\)/ }).getAttribute('aria-checked')).toBe('true')
    expect(view.getByRole('radio', { name: /Contract Lifecycle Management \(CLM\)/ }).hasAttribute('disabled')).toBe(true)
    expect(view.getByRole('radio', { name: /Citizen Services/ }).hasAttribute('disabled')).toBe(true)
    expect(view.getByRole('radio', { name: /Life Sciences eTMF/ }).hasAttribute('disabled')).toBe(true)
    expect(view.getByRole('radio', { name: /Insurance Claims Management/ }).hasAttribute('disabled')).toBe(true)
    expect(container.querySelectorAll('box-badge[label="Coming soon"]')).toHaveLength(3)
    expect(container.querySelector('box-select[label="Default solution"]')).toBeNull()
    expect(screen.queryByText('https://example.com/clm')).toBeNull()
    expect(screen.queryByText('Box, Salesforce')).toBeNull()
    expect(screen.queryByText('Package configuration')).toBeNull()
    expect(screen.queryByText('Readiness')).toBeNull()
    expect(screen.queryByText('Experience Cloud E2E')).toBeNull()
    expect(container.querySelectorAll('.settings-provider-grid box-button[label="Manage"]')).toHaveLength(2)
    expect(container.querySelectorAll('.settings-provider-grid box-badge[label="Selected"]')).toHaveLength(2)
    expect(container.querySelectorAll('box-icon-button[icon="cart-1"]')).toHaveLength(4)
  })

  it('removes a saved Box connection directly from its Settings row', () => {
    const onRemoveBoxConnection = vi.fn(async () => true)
    const { container } = render(<SettingsPage defaults={defaults} connections={connections} onSaveDefaults={vi.fn()} onBoxConnection={vi.fn()} onSalesforceConnection={vi.fn()} onRemoveBoxConnection={onRemoveBoxConnection} onRemoveSalesforceConnection={vi.fn(async () => true)}/>)

    fireEvent.click(container.querySelector('box-icon-button[label="Remove Sandbox Box"]')!)

    expect(onRemoveBoxConnection).toHaveBeenCalledWith('box-2')
  })

  it('edits and saves workspace defaults for future deployments', async () => {
    const onSaveDefaults = vi.fn().mockResolvedValue({ ...defaults, strategy: 'create_new', components: ['box'] })
    const { container } = render(<SettingsPage defaults={defaults} connections={connections} onSaveDefaults={onSaveDefaults} onBoxConnection={vi.fn()} onSalesforceConnection={vi.fn()} {...connectionActions}/>)
    const view = within(container)

    fireEvent.click(container.querySelector('box-button[label="Edit defaults"]')!)
    expect(container.querySelector('box-select[label="Default solution"]')).toBeNull()
    expect(view.getByRole('radio', { name: /Contract Lifecycle Management \(CLM\)/ }).getAttribute('aria-checked')).toBe('true')
    expect(view.getByRole('radio', { name: /Citizen Services/ }).hasAttribute('disabled')).toBe(true)
    expect(view.getByRole('radio', { name: /Life Sciences eTMF/ }).hasAttribute('disabled')).toBe(true)
    expect(view.getByRole('radio', { name: /Insurance Claims Management/ }).hasAttribute('disabled')).toBe(true)
    expect(container.querySelectorAll('box-badge[label="Coming soon"]')).toHaveLength(3)
    expect(view.getByText('https://example.com/clm')).toBeTruthy()
    fireEvent.click(view.getByRole('radio', { name: /Create new/ }))
    fireEvent(container.querySelector('box-switch[label="Salesforce"]')!, new CustomEvent('checked-changed', { detail: { checked: false } }))
    fireEvent.click(container.querySelector('box-button[label="Save defaults"]')!)

    await waitFor(() => expect(onSaveDefaults).toHaveBeenCalledWith({ templateId: 'clm', strategy: 'create_new', components: ['box'] }))
    await waitFor(() => expect(container.querySelector('box-button[label="Edit defaults"]')).toBeTruthy())
    expect(view.queryByText('https://example.com/clm')).toBeNull()
  })
})
