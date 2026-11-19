export interface HealthzResponse {
  status: 'ok' | 'error'
  db: 'ok' | 'error'
}

export interface VM {
  vmid: number
  name: string
  node: string
  status: string
  cpu: number // 0〜1
  maxcpu: number
  mem: number // バイト
  maxmem: number // バイト
  tags: string[] | null
}

export interface VMsResponse {
  vms: VM[]
  updated_at: string | null
  error: string
}

export type Range = '1h' | '6h' | '24h'

export interface MetricPoint {
  t: number // UNIX時刻（ミリ秒）
  cpu: number // %
  mem: number // %
  cpus: number
  mem_used: number
  mem_max: number
}

export interface MetricsResponse {
  vmid: number
  range: Range
  points: MetricPoint[]
}
