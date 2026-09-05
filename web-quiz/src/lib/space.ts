import type { PortalSpace } from '@/lib/types'

// 当前空间持久化（'oj:space'）：成员多空间切换、管理员空间切换都记录；
// 单空间用户同样写入，便于下次直达。
const SPACE_KEY = 'oj:space'

export function savedSpaceId(): number | null {
  const raw = localStorage.getItem(SPACE_KEY)
  if (!raw) return null
  const n = Number(raw)
  return Number.isFinite(n) && n > 0 ? n : null
}

export function saveSpaceId(id: number): void {
  localStorage.setItem(SPACE_KEY, String(id))
}

// 在可用空间列表中解析「应进入的空间」：
// - 存储里有效的空间 → 它；
// - 否则仅一个空间 → 它；
// - 否则 null（需用户在空间选择页挑选）。
export function resolveEntrySpace(spaces: PortalSpace[]): PortalSpace | null {
  if (!spaces || spaces.length === 0) return null
  const saved = savedSpaceId()
  const hit = spaces.find((s) => s.id === saved)
  return hit ?? (spaces.length === 1 ? spaces[0] : null)
}
