import { useEffect, useState } from 'react'
import { api } from './api/client'
import type { HealthzResponse } from './api/types'

export default function App() {
  const [health, setHealth] = useState<HealthzResponse | null>(null)
  const [error, setError] = useState('')

  useEffect(() => {
    api
      .healthz()
      .then(setHealth)
      .catch((err: unknown) => setError(err instanceof Error ? err.message : String(err)))
  }, [])

  return (
    <main className="container">
      <h1>ProxConductor</h1>
      <p className="muted">Proxmox VE 向けの運用支援ツール</p>
      <section className="card">
        <h2>サーバーの状態</h2>
        {error && <p className="error">接続できません：{error}</p>}
        {!error && !health && <p className="muted">確認中…</p>}
        {health && (
          <dl>
            <dt>API</dt>
            <dd className={health.status === 'ok' ? 'ok' : 'error'}>{health.status}</dd>
            <dt>DB</dt>
            <dd className={health.db === 'ok' ? 'ok' : 'error'}>{health.db}</dd>
          </dl>
        )}
      </section>
    </main>
  )
}
