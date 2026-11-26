import type { AppEvent } from '../api/types'

interface Props {
  events: AppEvent[]
}

export default function EventTable({ events }: Props) {
  if (events.length === 0) {
    return <p>まだ記録はありません</p>
  }
  return (
    <table>
      <thead>
        <tr>
          <th>時刻</th>
          <th>VMID</th>
          <th>種類</th>
          <th>内容</th>
        </tr>
      </thead>
      <tbody>
        {events.map((e) => (
          <tr key={e.id}>
            <td>{new Date(e.time).toLocaleString()}</td>
            <td>{e.vmid}</td>
            <td>{e.kind}</td>
            <td>
              {e.dry_run && '［ドライラン］'}
              {e.message}
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}
