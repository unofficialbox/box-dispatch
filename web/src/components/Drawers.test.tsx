// @vitest-environment jsdom
import { fireEvent, render } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { BoxConnectionDrawer, SalesforceConnectionDrawer } from './Drawers'

const resolveTrue = async () => true

describe('connection drawers', () => {
  it('opens the selected Salesforce environment with a concise label', () => {
    const onOpen = vi.fn()
    const { container } = render(<SalesforceConnectionDrawer
      connection={{
        name: 'Salesforce', configured: true, verified: true, restConfigured: true,
        launchUrl: 'https://example.my.salesforce.com/',
        orgs: [{ id: 'org-1', alias: 'Test1', kind: 'production', status: 'Ready', selected: true, username: 'user@example.com' }],
      }}
      loading={false} error="" oauthJob={null} scratchJob={null}
      onLogin={resolveTrue} onSelect={resolveTrue} onRemove={resolveTrue}
      onOpen={onOpen} onCreateScratch={vi.fn()} onClose={vi.fn()}
    />)

    const button = container.querySelector<HTMLElement>('.saved-connection-actions box-button[label="Open org"]')
    expect(button).toBeTruthy()
    fireEvent.click(button!)
    expect(onOpen).toHaveBeenCalledOnce()
  })

  it('separates current-org management from the two add-connection paths', () => {
    const { container } = render(<SalesforceConnectionDrawer
      connection={{
        name: 'Salesforce', configured: true, verified: true, restConfigured: true, devHubConfigured: true,
        launchUrl: 'https://example.my.salesforce.com/',
        orgs: [
          { id: 'org-1', alias: 'Test1', kind: 'Scratch org', status: 'Ready', selected: true, username: 'user@example.com', orgId: '00D123' },
          { id: 'org-2', alias: 'Production', kind: 'Org', status: 'Ready', selected: false, username: 'admin@example.com', orgId: '00D456' },
        ],
      }}
      loading={false} error="" oauthJob={null} scratchJob={null}
      onLogin={resolveTrue} onSelect={resolveTrue} onRemove={resolveTrue}
      onOpen={vi.fn()} onCreateScratch={vi.fn()} onClose={vi.fn()}
    />)

    expect(container.querySelector('box-select[label="Salesforce org"]')).toBeNull()
    expect(container.querySelector('box-button[label="Verify"]')).toBeNull()
    expect(container.querySelector('.selected-environment-details')?.textContent).toContain('00D123')
    expect(container.querySelector('.selected-environment-details')?.textContent).toContain('Scratch org')
    expect(container.querySelector('box-select[label="Salesforce environment"]')).toBeNull()
    expect(container.querySelector('box-text-field[label="Scratch org alias"]')).toBeNull()

    fireEvent.click(container.querySelector('button.connection-mode-card:nth-child(2)')!)

    expect(container.querySelector('box-text-field[label="Scratch org alias"]')).toBeTruthy()
    expect(container.querySelector('box-switch[label="Install Box for Salesforce automatically"][checked]')).toBeTruthy()
    expect(container.querySelector('box-select[label="Salesforce environment"]')).toBeNull()
  })

  it('lists connected Salesforce environments below the connection actions', () => {
    const onSelect = vi.fn(resolveTrue)
    const { container } = render(<SalesforceConnectionDrawer
      connection={{
        name: 'Salesforce', configured: true, verified: true, restConfigured: true,
        orgs: [
          { id: 'org-1', alias: 'Current', kind: 'Scratch org', status: 'Ready', selected: true, username: 'current@example.com', orgId: '00D1' },
          { id: 'org-2', alias: 'Production', kind: 'Org', status: 'Ready', selected: false, username: 'admin@example.com', orgId: '00D2' },
        ],
      }}
      loading={false} error="" oauthJob={null} scratchJob={null}
      onLogin={resolveTrue} onSelect={onSelect} onRemove={resolveTrue}
      onOpen={vi.fn()} onCreateScratch={vi.fn()} onClose={vi.fn()}
    />)

    fireEvent.click(container.querySelector('button.connection-mode-card:first-child')!)
    const actions = container.querySelector('.connection-mode-panel .drawer-button-row')!
    const list = container.querySelector('.connected-environments')!
    expect(actions.compareDocumentPosition(list) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    const production = Array.from(container.querySelectorAll<HTMLButtonElement>('.connection-option-main')).find((button) => button.textContent?.includes('Production'))
    fireEvent.click(production!)
    expect(onSelect).toHaveBeenCalledWith('org-2')
  })

  it('opens the selected Box environment with a concise label', () => {
    const onOpen = vi.fn()
    const { container } = render(<BoxConnectionDrawer
      connection={{
        name: 'Box', configured: true, verified: true, oauthConfigured: true,
        launchUrl: 'https://app.box.com/',
        connections: [{ id: 'box-1', alias: 'Production', status: 'Ready', selected: true, identity: 'user@example.com' }],
      }}
      loading={false} error="" oauthJob={null}
      onLogin={resolveTrue} onSelect={resolveTrue} onRemove={resolveTrue} onVerify={resolveTrue}
      onOpen={onOpen} onClose={vi.fn()}
    />)

    const button = container.querySelector<HTMLElement>('.saved-connection-actions box-button[label="Open"]')
    expect(button).toBeTruthy()
    fireEvent.click(button!)
    expect(onOpen).toHaveBeenCalledOnce()
  })
})
