import type { VM } from '../api/types'
import { cpuLimitOf, gib, memLimitOf, pct } from '../format'

interface Props {
  vms: VM[]
  selected: number | null
  onSelect: (vmid: number) => void
  onToggleAutoscale: (vmid: number, enabled: boolean) => void
}

export default function VmTable({ vms, selected, onSelect, onToggleAutoscale }: Props) {
  if (vms.length === 0) {
    return <p>VMがありません</p>
  }
  return (
    <table>
      <thead>
        <tr>
          <th>オートスケール</th>
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
            <td onClick={(e) => e.stopPropagation()}>
              <label>
                <input
                  type="checkbox"
                  checked={vm.autoscale}
                  onChange={(e) => onToggleAutoscale(vm.vmid, e.target.checked)}
                />
                対象
              </label>
              {vm.autoscale_mismatch && <span className="error">（VM名が変わったため停止中）</span>}
            </td>
            <td>{vm.vmid}</td>
            <td>{vm.name}</td>
            <td>{vm.node}</td>
            <td>{vm.status}</td>
            <td>
              {pct(((vm.cpu * vm.maxcpu) / cpuLimitOf(vm)) * 100)}（上限 {cpuLimitOf(vm)} / 全 {vm.maxcpu} コア）
            </td>
            <td>
              {gib(vm.mem)} / {gib(memLimitOf(vm))}（{pct(memLimitOf(vm) > 0 ? (vm.mem / memLimitOf(vm)) * 100 : 0)}、設定 {gib(vm.maxmem)}）
            </td>
            <td>{(vm.tags ?? []).join(', ')}</td>
          </tr>
        ))}
      </tbody>
    </table>
  )
}
