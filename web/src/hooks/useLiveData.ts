import { useEffect, useState } from 'react'
import { api } from '../api/client'
import type { AppEvent, VMsResponse } from '../api/types'

const MAX_EVENTS = 50

// サーバーから届く最新の VM 一覧・記録と、接続の状態を返す。
export function useLiveData() {
  const [data, setData] = useState<VMsResponse | null>(null)
  const [events, setEvents] = useState<AppEvent[]>([])
  const [connected, setConnected] = useState(false)

  useEffect(() => {
    let cancelled = false
    api
      .events(MAX_EVENTS)
      .then((es) => !cancelled && setEvents(es))
      .catch(() => {})

    const es = new EventSource('/api/stream')
    es.onopen = () => setConnected(true)
    es.onerror = () => setConnected(false)
    es.addEventListener('vms', (e) => {
      setData(JSON.parse(e.data) as VMsResponse)
      setConnected(true)
    })
    es.addEventListener('event', (e) => {
      const ev = JSON.parse(e.data) as AppEvent
      setEvents((prev) => [ev, ...prev].slice(0, MAX_EVENTS))
    })
    return () => {
      cancelled = true
      es.close()
    }
  }, [])

  return { data, events, connected }
}
