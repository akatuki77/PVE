import type { VM } from '../api/types'
import { gib, pct } from '../format'

interface Props {
  vms: VM[]
  selected: number | null
  onSelect: (vmid: number) => void
}

export default function VmTable({ vms, selected, onSelect }: Props) {
  if (vms.length === 0) {
    return <p>VMがありません</p>
  }
  return (
    <table>
      <thead>
        <tr>
          <th>VMID</th>
          <th>名前</th>
          <th>ノード</th>
          <th>状態</th>
          <th>CPU</th>
          <th>メモリ</th>
          <th>タグ</th>
        </tr>
      </thead>
      <tbody>
        {vms.map((vm) => (
          <tr
            key={vm.vmid}
            className={vm.vmid === selected ? 'selected' : ''}
            onClick={() => onSelect(vm.vmid)}
          >
            <td>{vm.vmid}</td>
            <td>{vm.name}</td>
            <td>{vm.node}</td>
            <td>{vm.status}</td>
            <td>
              {pct(vm.cpu * 100)} / {vm.maxcpu} コア
            </td>
            <td>
              {gib(vm.mem)} / {gib(vm.maxmem)}（{pct(vm.maxmem > 0 ? (vm.mem / vm.maxmem) * 100 : 0)}）
            </td>
            <td>{(vm.tags ?? []).join(', ')}</td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}
