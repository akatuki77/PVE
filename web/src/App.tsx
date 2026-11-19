import { useState } from 'react'
import MetricsTable from './components/MetricsTable'
import VmTable from './components/VmTable'
import { time } from './format'
import { useLiveVMs } from './hooks/useLiveVMs'

export default function App() {
  const { data, connected } = useLiveVMs()
  const [selected, setSelected] = useState<number | null>(null)

  return (
    <main>
      <h1>ProxConductor</h1>
      <p>
        {connected ? '● リアルタイム更新中' : '○ 再接続中…'}　最終取得：
        {data?.updated_at ? time(data.updated_at) : 'まだ取得していません'}
      </p>
      {data?.error && <p className="error">Proxmoxからの取得に失敗：{data.error}</p>}

      <h2>VM一覧</h2>
      {data && <VmTable vms={data.vms} selected={selected} onSelect={setSelected} />}

      {selected !== null && <MetricsTable vmid={selected} updatedAt={data?.updated_at ?? null} />}
    </main>
  )
}
