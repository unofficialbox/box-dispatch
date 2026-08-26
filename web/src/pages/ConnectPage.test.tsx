// @vitest-environment jsdom
import { render } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { ConnectPage } from './ConnectPage'
import type { ConnectionSummary, DeploymentPlan } from '../types'

const plan: DeploymentPlan = {
  exists: true,
  name: 'Northstar CLM rollout',
  templateId: 'clm',
  template: 'CLM deployment',
  repository: 'example/dispatch-template',
  strategy: 'reuse',
  components: [
    { id: 'box', name: 'Box', configured: true, verified: true, ready: true },
    { id: 'salesforce', name: 'Salesforce', configured: true, verified: true, ready: true },
  ],
}

const connections: ConnectionSummary[] = [
  { name: 'Box', configured: true, verified: true, launchUrl: 'https://app.box.com/' },
  { name: 'Salesforce', configured: true, verified: true, launchUrl: 'https://example.my.salesforce.com/' },
]

describe('ConnectPage', () => {
  it('offers direct access to each verified provider', () => {
    const { container } = render(<ConnectPage plan={plan} connections={connections} notice="" onBoxConnection={vi.fn()} onSalesforceConnection={vi.fn()} onOpenProvider={vi.fn()} onBack={vi.fn()} onNext={vi.fn()} />)

    expect(container.querySelector('[data-provider-logo="box"]')).toBeTruthy()
    expect(container.querySelector('[data-provider-logo="salesforce"]')).toBeTruthy()
    expect(container.querySelector('box-button[label="Open"][aria-label="Open Box"]')).toBeTruthy()
    expect(container.querySelector('box-button[label="Open"][aria-label="Open Salesforce"]')).toBeTruthy()
  })
})
