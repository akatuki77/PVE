import type { HealthzResponse, MetricsResponse, Range, VMsResponse } from './types'

async function getJSON<T>(path: string): Promise<T> {
  const res = await fetch(path)
  if (!res.ok) {
    throw new Error(`${path}: ${res.status} ${res.statusText}`)
  }
  return (await res.json()) as T
}

export const api = {
  healthz: () => getJSON<HealthzResponse>('/api/healthz'),
  vms: () => getJSON<VMsResponse>('/api/vms'),
  metrics: (vmid: number, range: Range) =>
    getJSON<MetricsResponse>(`/api/vms/${vmid}/metrics?range=${range}`),
}
