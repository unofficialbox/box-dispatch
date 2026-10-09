// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { DeploymentConfirmationDialog } from './DeploymentConfirmationDialog'
import type { DeploymentPlan } from '../types'

const plan: DeploymentPlan = { exists: true, name: 'Northstar CLM', templateId: 'clm', template: 'CLM', repository: 'example', strategy: 'reuse', components: [{ id: 'box', name: 'Box', configured: true, verified: true, ready: true }, { id: 'salesforce', name: 'Salesforce', configured: true, verified: true, ready: true }] }

describe('DeploymentConfirmationDialog', () => {
  afterEach(cleanup)
  it('routes Box Open Elements cancellation through the controlled close action', () => {
    const onCancel = vi.fn()
    const { container } = render(<DeploymentConfirmationDialog plan={plan} packagePreparing={false} onCancel={onCancel} onConfirm={vi.fn()}/>)
    fireEvent(container.querySelector('box-dialog')!, new CustomEvent('cancel'))
    expect(onCancel).toHaveBeenCalledOnce()
  })

  it('summarizes the validated target before deployment', () => {
    const onConfirm = vi.fn()
    const { container } = render(<DeploymentConfirmationDialog plan={plan} packagePreparing={false} onCancel={vi.fn()} onConfirm={onConfirm}/>)
    expect(screen.getByText('Northstar CLM')).toBeTruthy()
    expect(screen.getByText('Box, Salesforce')).toBeTruthy()
    fireEvent(container.querySelector('box-dialog')!, new CustomEvent('confirm'))
    expect(onConfirm).toHaveBeenCalledOnce()
  })

  it('waits for background managed-package setup', () => {
    const { container } = render(<DeploymentConfirmationDialog plan={plan} packagePreparing packageMessage="Salesforce reports in progress" onCancel={vi.fn()} onConfirm={vi.fn()}/>)
    expect(screen.getByText('Salesforce setup is still running')).toBeTruthy()
    const dialog = container.querySelector('box-dialog')
    expect(dialog?.hasAttribute('confirm-disabled')).toBe(true)
    expect(dialog?.hasAttribute('confirm-busy')).toBe(true)
    expect(dialog?.getAttribute('confirm-busy-label')).toBe('Waiting for Salesforce…')
  })
})
