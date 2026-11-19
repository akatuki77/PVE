export const pct = (n: number) => `${n.toFixed(1)}%`

export const gib = (bytes: number) => `${(bytes / 1024 ** 3).toFixed(2)} GiB`

export const time = (value: string | number) => new Date(value).toLocaleTimeString()
