// Small display helpers shared across pages.

export function fmtNum(n: number | string | undefined | null): string {
  const v = Number(n || 0)
  if (v >= 1e9) return (v / 1e9).toFixed(2) + 'B'
  if (v >= 1e6) return (v / 1e6).toFixed(2) + 'M'
  if (v >= 1e3) return (v / 1e3).toFixed(2) + 'K'
  return String(v)
}

export function fmtCost(n: number | undefined | null): string {
  return Number(n || 0).toFixed(4)
}

export function fmtTime(iso: string | undefined | null): string {
  if (!iso) return '-'
  return iso.length >= 19 ? iso.slice(11, 19) : iso
}

export function okBadge(ok: boolean | undefined): { label: string; tone: string } {
  return ok
    ? { label: '正常', tone: 'success' }
    : { label: '异常', tone: 'danger' }
}

export function statusTone(status: number): string {
  if (status >= 200 && status < 300) return 'success'
  if (status >= 400 && status < 500) return 'warning'
  return 'danger'
}

export function isStream(stream: boolean | undefined): string {
  return stream ? 'Y' : ''
}
