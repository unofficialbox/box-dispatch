export async function fetchJSON<T>(path: string, signal: AbortSignal): Promise<T> {
  const response = await fetch(path, { signal })
  if (!response.ok) throw new Error(`${path} is unavailable.`)
  return (await response.json()) as T
}

export async function fetchOptionalJSON<T>(path: string, signal: AbortSignal): Promise<T | null> {
  const response = await fetch(path, { signal })
  if (response.status === 204) return null
  if (!response.ok) throw new Error(`${path} is unavailable.`)
  return (await response.json()) as T
}
