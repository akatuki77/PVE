import { useEffect, useState } from 'react'
import type { VMsResponse } from '../api/types'

// サーバーから届く最新の VM 一覧と、接続の状態を返す。
export function useLiveVMs() {
  const [data, setData] = useState<VMsResponse | null>(null)
  const [connected, setConnected] = useState(false)

  useEffect(() => {
    const es = new EventSource('/api/stream')
    es.onopen = () => setConnected(true)
    es.onerror = () => setConnected(false)
    es.addEventListener('vms', (e) => {
      setData(JSON.parse(e.data) as VMsResponse)
      setConnected(true)
    })
    return () => es.close()
  }, [])

  return { data, connected }
}
