// 管理区域上下文（src/pages/admin/domain-context.tsx）：自原管理端前端 domain-context 迁移并适配单前端。
// 会话（/me、登录/登出）由 app/App.tsx 统一管理；本上下文只负责管理区所需的：
//   - user / role：当前登录用户（由 App 传入）；
//   - 当前管理域 domainId：
//       domain_admin 锁定 user.domainId（无需选择）；
//       global_admin 从 localStorage 'OrangeOJ:domain' 恢复上次选择，可切换；未选为 null。
// 域选择持久化，并同步给 src/api/admin.ts（所有带域请求自动附加 domainId=）。
// 切换域后清相关仓库/空间查询缓存（query key 不含域，需强制重取），防止跨域残留。
// 兼容命名：导出 useDomain（对齐 web 原命名，迁入组件零改动）。
import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { setDomain as syncApiDomain } from '@/api/admin'
import type { Role, User } from '@/api/types'

const DOMAIN_STORAGE_KEY = 'OrangeOJ:domain'

export interface AdminDomainContextValue {
  user: User
  role: Role
  /** 当前生效管理域（null=未选/全局）；仅仓库/空间管理数据接口使用。 */
  domainId: number | null
  /** 切换当前域（global_admin 域下拉用）：持久化并同步 api 模块。 */
  setDomainId: (id: number | null) => void
}

const Ctx = createContext<AdminDomainContextValue | null>(null)

function readStoredDomain(): number | null {
  try {
    const raw = localStorage.getItem(DOMAIN_STORAGE_KEY)
    if (raw == null || raw === '') return null
    const n = Number(raw)
    return Number.isFinite(n) && n > 0 ? n : null
  } catch {
    return null
  }
}

/** 按用户角色解析应生效的域（domain_admin 锁定归属域；global_admin 恢复上次选择）。 */
function resolveDomain(user: User): number | null {
  if (user.role === 'domain_admin') return user.domainId ?? null
  if (user.role === 'global_admin') return readStoredDomain()
  return null
}

export function AdminDomainProvider({ user, children }: { user: User; children: ReactNode }) {
  const qc = useQueryClient()
  const [domainId, setDomainIdState] = useState<number | null>(() => resolveDomain(user))
  const appliedDomainRef = useRef<number | null>(domainId)

  // 切换域后让仓库/空间数据全部重取（域不同、结果不同；query key 不带域）
  const invalidateDomainData = useCallback(() => {
    void qc.invalidateQueries({ queryKey: ['problems'] })
    void qc.invalidateQueries({ queryKey: ['tags'] })
    void qc.invalidateQueries({ queryKey: ['tag-order'] })
    void qc.invalidateQueries({ queryKey: ['trainings'] })
    void qc.invalidateQueries({ queryKey: ['practices'] })
    void qc.invalidateQueries({ queryKey: ['booklet-directories'] })
    void qc.invalidateQueries({ queryKey: ['group-list'] })
    void qc.invalidateQueries({ queryKey: ['problem'] })
    void qc.invalidateQueries({ queryKey: ['training'] })
    void qc.invalidateQueries({ queryKey: ['practice'] })
    void qc.invalidateQueries({ queryKey: ['space-problem-picker'] })
    void qc.invalidateQueries({ queryKey: ['admin', 'spaces'] })
  }, [qc])

  // 首次挂载：同步 api 模块（登录进入管理区时建立当前域）。
  useEffect(() => {
    syncApiDomain(domainId)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const setDomainId = useCallback(
    (id: number | null) => {
      setDomainIdState(id)
      syncApiDomain(id)
      if (appliedDomainRef.current !== id) {
        appliedDomainRef.current = id
        invalidateDomainData()
      }
      try {
        if (id == null) localStorage.removeItem(DOMAIN_STORAGE_KEY)
        else localStorage.setItem(DOMAIN_STORAGE_KEY, String(id))
      } catch {
        // localStorage 不可用时忽略（会话内仍生效）
      }
    },
    [invalidateDomainData],
  )

  const value = useMemo<AdminDomainContextValue>(
    () => ({ user, role: user.role, domainId, setDomainId }),
    [user, domainId, setDomainId],
  )
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>
}

export function useDomain(): AdminDomainContextValue {
  const v = useContext(Ctx)
  if (!v) throw new Error('useDomain must be used within AdminDomainProvider')
  return v
}
