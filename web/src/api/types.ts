export interface HealthzResponse {
  status: 'ok' | 'error'
  db: 'ok' | 'error'
}
