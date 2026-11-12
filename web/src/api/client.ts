import type { HealthzResponse } from './types'

async function getJSON<T>(path: string): Promise<T> {
  const res = await fetch(path)
  if (!res.ok) {
    throw new Error(`${path}: ${res.status} ${res.statusText}`)
  }
  return (await res.json()) as T
}

export const api = {
  healthz: () => getJSON<HealthzResponse>('/api/healthz'),
}
