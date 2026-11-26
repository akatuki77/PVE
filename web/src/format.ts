import type { VM } from './api/types'

export const pct = (n: number) => `${n.toFixed(1)}%`

export const gib = (bytes: number) => `${(bytes / 1024 ** 3).toFixed(2)} GiB`

export const time = (value: string | number) => new Date(value).toLocaleTimeString()

// 今使える CPU の量（コア数）。
export const cpuLimitOf = (vm: VM) => (vm.cpulimit > 0 && vm.cpulimit < vm.maxcpu ? vm.cpulimit : vm.maxcpu)

// 今使えるメモリの量（バイト）。
export const memLimitOf = (vm: VM) => (vm.balloon > 0 && vm.balloon < vm.maxmem ? vm.balloon : vm.maxmem)
