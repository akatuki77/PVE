import { useEffect, useState } from 'react'
import { api } from '../api/client'
import type { MetricPoint, Range } from '../api/types'
import { gib, pct, time } from '../format'

const ranges: Range[] = ['1h', '6h', '24h']
const REFRESH_MS = 15_000

interface Props {
  vmid: number
}

export default function MetricsTable({ vmid }: Props) {
  const [range, setRange] = useState<Range>('1h')
  const [points, setPoints] = useState<MetricPoint[]>([])
  const [error, setError] = useState('')

  useEffect(() => {
    let cancelled = false
    const load = () =>
      api
        .metrics(vmid, range)
        .then((r) => {
          if (cancelled) return
          setPoints(r.points)
          setError('')
        })
        .catch((err: unknown) => !cancelled && setError(String(err)))
    load()
    const timer = setInterval(load, REFRESH_MS)
    return () => {
      cancelled = true
      clearInterval(timer)
    }
  }, [vmid, range])

  return (
    <section>
      <h2>VM {vmid} の使用量の推移</h2>
      <p>
        期間：
        {ranges.map((r) => (
          <button key={r} disabled={r === range} onClick={() => setRange(r)}>
            {r}
          </button>
        ))}
        （{points.length} 件）
      </p>
      {error && <p className="error">{error}</p>}
      <div className="scroll">
        <table>
          <thead>
            <tr>
              <th>時刻</th>
              <th>CPU</th>
              <th>メモリ</th>
              <th>メモリ使用量</th>
              <th>コア数</th>
            </tr>
          </thead>
          <tbody>
            {[...points].reverse().map((p) => (
              <tr key={p.t}>
                <td>{time(p.t)}</td>
                <td>{pct(p.cpu)}</td>
                <td>{pct(p.mem)}</td>
                <td>
                  {gib(p.mem_used)} / {gib(p.mem_max)}
                </td>
                <td>{p.cpus}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  )
}
