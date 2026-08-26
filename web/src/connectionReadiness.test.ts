import { describe, expect, it } from 'vitest'
import { refreshProviderReadiness } from './connectionReadiness'

describe('refreshProviderReadiness', () => {
  it('updates the Box workflow state immediately after an OAuth connection refresh', () => {
    const plan = {
      exists: true, name: 'Northstar CLM rollout', templateId: 'clm', template: 'CLM deployment', repository: 'https://example.test/clm', strategy: 'reuse' as const,
      components: [
        { id: 'box', name: 'Box', configured: false, verified: false, ready: false },
        { id: 'salesforce', name: 'Salesforce', configured: true, verified: true, ready: true },
      ],
    }

    const updated = refreshProviderReadiness(plan, [{ name: 'Box', configured: true, verified: true, status: 'Ready' }], 'box')

    expect(updated.components).toEqual([
      { id: 'box', name: 'Box', configured: true, verified: true, ready: true },
      { id: 'salesforce', name: 'Salesforce', configured: true, verified: true, ready: true },
    ])
  })
})
