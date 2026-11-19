import { useEffect, useState } from 'react'
import { api } from './api/client'
import type { VMsResponse } from './api/types'
import MetricsTable from './components/MetricsTable'
import VmTable from './components/VmTable'
import { time } from './format'

const REFRESH_MS = 15_000

export default function App() {
  const [data, setData] = useState<VMsResponse | null>(null)
  const [fetchError, setFetchError] = useState('')
  const [selected, setSelected] = useState<number | null>(null)

  useEffect(() => {
    const load = () =>
      api
        .vms()
        .then((r) => {
          setData(r)
          setFetchError('')
        })
        .catch((err: unknown) => setFetchError(String(err)))
    load()
    const timer = setInterval(load, REFRESH_MS)
    return () => clearInterval(timer)
  }, [])

  return (
    <main>
      <h1>ProxConductor</h1>
      <p>最終取得：{data?.updated_at ? time(data.updated_at) : 'まだ取得していません'}</p>
      {fetchError && <p className="error">APIに接続できません：{fetchError}</p>}
      {data?.error && <p className="error">Proxmoxからの取得に失敗：{data.error}</p>}

      <h2>VM一覧</h2>
      {data && <VmTable vms={data.vms} selected={selected} onSelect={setSelected} />}

      {selected !== null && <MetricsTable vmid={selected} />}
    </main>
  )
}
