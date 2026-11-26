import { useState } from 'react'
import { api } from './api/client'
import EventTable from './components/EventTable'
import MetricsTable from './components/MetricsTable'
import VmTable from './components/VmTable'
import { time } from './format'
import { useLiveData } from './hooks/useLiveData'

export default function App() {
  const { data, events, connected } = useLiveData()
  const [selected, setSelected] = useState<number | null>(null)
  const [toggleError, setToggleError] = useState('')

  // 切り替えの結果は SSE で届くので、ここでは送るだけ
  const toggleAutoscale = (vmid: number, enabled: boolean) => {
    setToggleError('')
    api.setAutoscale(vmid, enabled).catch((err: unknown) => setToggleError(String(err)))
  }

  return (
    <main>
      <h1>ProxConductor</h1>
      <p>
        {connected ? '● リアルタイム更新中' : '○ 再接続中…'}　最終取得：
        {data?.updated_at ? time(data.updated_at) : 'まだ取得していません'}
      </p>
      {data?.error && <p className="error">Proxmoxからの取得に失敗：{data.error}</p>}
      {toggleError && <p className="error">オートスケールの切り替えに失敗：{toggleError}</p>}

      <h2>VM一覧</h2>
      {data && (
        <VmTable
          vms={data.vms}
          selected={selected}
          onSelect={setSelected}
          onToggleAutoscale={toggleAutoscale}
        />
      )}

      {selected !== null && <MetricsTable vmid={selected} updatedAt={data?.updated_at ?? null} />}

      <h2>オートスケールの記録</h2>
      <EventTable events={events} />
    </main>
  )
}
