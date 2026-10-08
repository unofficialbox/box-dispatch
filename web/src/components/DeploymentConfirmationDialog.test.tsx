// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { DeploymentConfirmationDialog } from './DeploymentConfirmationDialog'
import type { DeploymentPlan } from '../types'

const plan: DeploymentPlan = { exists: true, name: 'Northstar CLM', templateId: 'clm', template: 'CLM', repository: 'example', strategy: 'reuse', components: [{ id: 'box', name: 'Box', configured: true, verified: true, ready: true }, { id: 'salesforce', name: 'Salesforce', configured: true, verified: true, ready: true }] }

describe('DeploymentConfirmationDialog', () => {
  afterEach(cleanup)
  it('routes native Escape cancellation through the controlled close action', () => {
    const onCancel = vi.fn()
    const { container } = render(<DeploymentConfirmationDialog plan={plan} packagePreparing={false} onCancel={onCancel} onConfirm={vi.fn()}/>)
    const event = new Event('cancel', { cancelable: true })
    fireEvent(container.querySelector('dialog')!, event)
    expect(onCancel).toHaveBeenCalledOnce()
    expect(event.defaultPrevented).toBe(true)
  })

  it('summarizes the validated target before deployment', () => {
    const onConfirm = vi.fn()
    const { container } = render(<DeploymentConfirmationDialog plan={plan} packagePreparing={false} onCancel={vi.fn()} onConfirm={onConfirm}/>)
    expect(screen.getByText('Northstar CLM')).toBeTruthy()
    expect(screen.getByText('Box, Salesforce')).toBeTruthy()
    const start = container.querySelector<HTMLElement>('box-button[label="Start deployment"]')
    fireEvent.click(start!)
    expect(onConfirm).toHaveBeenCalledOnce()
  })

  it('waits for background managed-package setup', () => {
    const { container } = render(<DeploymentConfirmationDialog plan={plan} packagePreparing packageMessage="Salesforce reports in progress" onCancel={vi.fn()} onConfirm={vi.fn()}/>)
    expect(screen.getByText('Salesforce setup is still running')).toBeTruthy()
    expect(container.querySelector('box-button[label="Waiting for Salesforce…"][disabled]')).toBeTruthy()
  })
})
