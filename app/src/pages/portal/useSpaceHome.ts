import { useQuery } from '@tanstack/react-query'

import { api } from '@/api'
import type { PortalSpace } from '@/api/types'
import { usePortalCtx } from './SpaceShell'

// 空间首页数据（训练/练习/刷题三区简报）：列表页共用一条查询，避免重复请求。
export function useSpaceHome(space?: PortalSpace) {
  const ctx = usePortalCtx()
  const sid = space?.id ?? ctx.space.id
  return useQuery({
    queryKey: ['portal-space-home', sid],
    queryFn: () => api.portalSpaceHome(sid),
  })
}
