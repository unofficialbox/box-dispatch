import { describe, expect, it, vi } from 'vitest'
import { markConnectionsChecking, markPlanConnectionsChecking, refreshPlanReadiness, revalidateConnections } from './connectionRefresh'
import type { ConnectionSummary, DeploymentPlan } from './types'

const connections: ConnectionSummary[] = [
  { name: 'Box', configured: true, verified: true, status: 'Ready', connections: [{ id: 'box-1', alias: 'Legal', status: 'Ready', selected: true }] },
  { name: 'Salesforce', configured: true, verified: true, status: 'Ready', orgs: [{ id: 'org-1', alias: 'Legal', kind: 'Org', status: 'Ready', selected: true }] },
  { name: 'Databricks', configured: true, verified: true, status: 'Ready' },
]

const plan: DeploymentPlan = {
  exists: true,
  name: 'Legal rollout',
  templateId: 'clm',
  template: 'CLM',
  repository: 'example/template',
  strategy: 'reuse',
  components: [
    { id: 'box', name: 'Box', configured: true, verified: true, ready: true },
    { id: 'salesforce', name: 'Salesforce', configured: true, verified: true, ready: true },
  ],
}

describe('connection refresh', () => {
  it('clears selected live readiness while a page refresh is in flight', () => {
    const checkingConnections = markConnectionsChecking(connections)
    expect(checkingConnections[0]).toMatchObject({ verified: false, status: 'Checking' })
    expect(checkingConnections[0].connections?.[0].status).toBe('Checking')
    expect(checkingConnections[1].orgs?.[0].status).toBe('Checking')
    expect(checkingConnections[2]).toEqual(connections[2])
    expect(markPlanConnectionsChecking(plan).components.every((component) => !component.ready && !component.verified)).toBe(true)
  })

  it('checks providers in parallel before reading the current summaries', async () => {
    let releaseChecks: () => void = () => undefined
    const checksFinished = new Promise<void>((resolve) => { releaseChecks = resolve })
    const calls: string[] = []
    const fetchConnectionStatus = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      calls.push(url)
      if (url.endsWith('/check')) {
        await checksFinished
        return new Response('{}', { status: 200 })
      }
      return new Response(JSON.stringify(connections), { status: 200 })
    })

    const refresh = revalidateConnections(fetchConnectionStatus)
    await vi.waitFor(() => expect(fetchConnectionStatus).toHaveBeenCalledTimes(2))
    expect(calls).toEqual(['/api/connections/box/check', '/api/connections/salesforce/check'])
    releaseChecks()
    await expect(refresh).resolves.toEqual(connections)
    expect(calls.at(-1)).toBe('/api/connections')
  })

  it('uses the refreshed summaries as the source of plan readiness', () => {
    const refreshed = refreshPlanReadiness(plan, [{ ...connections[0], verified: true }, { ...connections[1], verified: false }])
    expect(refreshed.components.map(({ id, ready }) => ({ id, ready }))).toEqual([
      { id: 'box', ready: true },
      { id: 'salesforce', ready: false },
    ])
  })

  it('does not reuse a cached ready snapshot when a live check fails', async () => {
    const fetchConnectionStatus = vi.fn(async (input: RequestInfo | URL) => String(input).includes('/salesforce/check')
      ? new Response('{"error":"expired"}', { status: 503 })
      : String(input).endsWith('/check')
        ? new Response('{}', { status: 200 })
        : new Response(JSON.stringify(connections), { status: 200 }))

    const refreshed = await revalidateConnections(fetchConnectionStatus)

    expect(refreshed.find((connection) => connection.name === 'Box')?.verified).toBe(true)
    expect(refreshed.find((connection) => connection.name === 'Salesforce')).toMatchObject({ verified: false, status: 'Not ready' })
    expect(refreshed.find((connection) => connection.name === 'Salesforce')?.orgs?.[0].status).toBe('Not ready')
  })
})
