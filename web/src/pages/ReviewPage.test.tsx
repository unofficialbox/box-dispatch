// @vitest-environment jsdom
import { render } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { ReviewPage } from './ReviewPage'

describe('ReviewPage', () => {
  it('keeps deployment summary fields in the details rail only', () => {
    const { container } = render(<ReviewPage
      plan={{
        exists: true,
        name: 'Box CLM',
        templateId: 'clm',
        template: 'CLM deployment',
        repository: 'https://github.com/unofficialbox/box-bedrock-for-clm',
        strategy: 'reuse',
        components: [
          { id: 'box', name: 'Box', configured: true, verified: true, ready: true },
          { id: 'salesforce', name: 'Salesforce', configured: true, verified: true, ready: true },
        ],
      }}
      notice="Authentication is current."
      checkingConnections={false}
      onDeploy={vi.fn()}
      onEditConnections={vi.fn()}
      onBack={vi.fn()}
    />)

    expect(container.querySelector('.review-summary')).toBeNull()
    const facts = container.querySelector('box-fact-list') as HTMLElement & { rows: Array<{ label: string; value: string }> }
    expect(facts.rows).toContainEqual({ label: 'Deployment', value: 'Box CLM' })
    expect(facts.rows).toContainEqual({ label: 'Solution', value: 'CLM deployment' })
  })
})
