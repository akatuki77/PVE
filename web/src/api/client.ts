import type { AppEvent, HealthzResponse, MetricsResponse, Range, VMsResponse } from './types'

async function getJSON<T>(path: string): Promise<T> {
  const res = await fetch(path)
  if (!res.ok) {
    throw new Error(`${path}: ${res.status} ${res.statusText}`)
  }
  return (await res.json()) as T
}

async function send(method: 'PUT' | 'DELETE', path: string): Promise<void> {
  const res = await fetch(path, { method })
  if (!res.ok) {
    const body = (await res.json().catch(() => null)) as { error?: string } | null
    throw new Error(body?.error ?? `${res.status} ${res.statusText}`)
  }
}

export const api = {
  healthz: () => getJSON<HealthzResponse>('/api/healthz'),
  vms: () => getJSON<VMsResponse>('/api/vms'),
  metrics: (vmid: number, range: Range) =>
    getJSON<MetricsResponse>(`/api/vms/${vmid}/metrics?range=${range}`),
  events: (limit: number) => getJSON<AppEvent[]>(`/api/events?limit=${limit}`),
  setAutoscale: (vmid: number, enabled: boolean) =>
    send(enabled ? 'PUT' : 'DELETE', `/api/vms/${vmid}/autoscale`),
}
