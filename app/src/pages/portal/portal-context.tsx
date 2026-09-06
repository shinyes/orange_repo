// 门户会话与空间上下文：独立全屏页（列表/详情/作答/排行榜）脱离 SpaceShell 壳后，
// 经本模块获取登录用户与空间信息。SpaceShell（空间菜单首页）仍用其 Outlet context。
import { createContext, useContext } from 'react'
import { useQuery } from '@tanstack/react-query'

import { api } from '@/api'
import type { PortalSpace, User } from '@/api/types'

// ---------- 会话（user/onLogout；App 顶层提供） ----------

export interface PortalSession {
  user: User
  onLogout: () => void
}

const SessionCtx = createContext<PortalSession | null>(null)

export function PortalSessionProvider({ value, children }: { value: PortalSession; children: React.ReactNode }) {
  return <SessionCtx.Provider value={value}>{children}</SessionCtx.Provider>
}

export function usePortalSession(): PortalSession {
  const v = useContext(SessionCtx)
  if (!v) throw new Error('usePortalSession must be used within PortalSessionProvider')
  return v
}

// ---------- 空间（独立页按 sid 自取） ----------

export function useSpaceById(spaceId: number): PortalSpace | null | 'loading' {
  const q = useQuery({ queryKey: ['portal-spaces'], queryFn: api.portalSpaces })
  if (q.isLoading) return 'loading'
  if (q.isError || !q.data) return null
  return q.data.spaces.find((s) => s.id === spaceId) ?? null
}
