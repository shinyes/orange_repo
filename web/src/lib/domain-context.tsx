// 域上下文：登录会话 + 当前域。
// - global_admin：无固定域；从 localStorage('OrangeOJ:domain') 恢复上次选择，未选则为 null
//   （仓库三栏在未选域时提示选域）。选择后持久化，并同步给 api 模块（所有带域请求附加 domainId=）。
// - domain_admin：锁定 user.domainId，无需选择，登录即生效。
// - member：后端登录已拒，不会到达此处。
import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { api, setDomain } from './api'
import type { MeUser } from './types'

const DOMAIN_STORAGE_KEY = 'OrangeOJ:domain'

type SessionStatus = 'loading' | 'anon' | 'authed'

interface DomainContextValue {
  status: SessionStatus
  user: MeUser | null
  /** 当前生效域（null=未选/全局）；仅仓库数据接口使用。 */
  domainId: number | null
  setDomainId: (id: number | null) => void
  /** 重新拉取 /me（登录成功后调用以取得 user）。 */
  refresh: () => Promise<void>
  logout: () => Promise<void>
  role: MeUser['role'] | null
}

const Ctx = createContext<DomainContextValue | null>(null)

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

/** 按用户角色解析应生效的域；无域（global_admin 未选）时返回 null。 */
function resolveDomain(user: MeUser): number | null {
  if (user.role === 'domain_admin') return user.domainId ?? null
  if (user.role === 'global_admin') return readStoredDomain()
  return null
}

export function DomainProvider({ children }: { children: ReactNode }) {
  const [status, setStatus] = useState<SessionStatus>('loading')
  const [user, setUser] = useState<MeUser | null>(null)
  const [domainId, setDomainIdState] = useState<number | null>(null)
  const qc = useQueryClient()
  // 切换域后让仓库数据全部重取（域不同、结果不同；query key 不带域）
  const appliedDomainRef = useRef<number | null>(null)

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
  }, [qc])

  /** 装载 /me 结果：解析 user 并按角色初始化域，同步 api 模块。 */
  const load = useCallback(async () => {
    try {
      const d = await api.me()
      if (!d.authenticated || !d.user) {
        setUser(null)
        setDomainIdState(null)
        setDomain(null)
        appliedDomainRef.current = null
        setStatus('anon')
        return
      }
      const u = d.user
      const resolved = resolveDomain(u)
      setUser(u)
      setDomainIdState(resolved)
      setDomain(resolved)
      if (appliedDomainRef.current !== resolved) {
        appliedDomainRef.current = resolved
        invalidateDomainData()
      }
      setStatus('authed')
    } catch {
      setUser(null)
      setDomainIdState(null)
      setDomain(null)
      appliedDomainRef.current = null
      setStatus('anon')
    }
  }, [invalidateDomainData])

  useEffect(() => {
    void load()
    const on401 = () => {
      qc.clear() // 会话失效：清空缓存，防换账号残留上一账号的仓库/空间数据
      setUser(null)
      setDomainIdState(null)
      setDomain(null)
      setStatus('anon')
    }
    window.addEventListener('OrangeOJ:unauthorized', on401)
    return () => window.removeEventListener('OrangeOJ:unauthorized', on401)
  }, [load, qc])

  const refresh = useCallback(async () => {
    setStatus('loading')
    await load()
  }, [load])

  const logout = useCallback(async () => {
    try {
      await api.logout()
    } finally {
      qc.clear() // 登出清空缓存，防换账号残留
      setUser(null)
      setDomainIdState(null)
      setDomain(null)
      setStatus('anon')
    }
  }, [qc])

  /** 切换当前域（global_admin 域下拉用）：持久化并同步 api 模块。 */
  const setDomainId = useCallback(
    (id: number | null) => {
      setDomainIdState(id)
      setDomain(id)
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

  const value = useMemo<DomainContextValue>(
    () => ({ status, user, domainId, setDomainId, refresh, logout, role: user?.role ?? null }),
    [status, user, domainId, setDomainId, refresh, logout],
  )
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>
}

export function useDomain(): DomainContextValue {
  const v = useContext(Ctx)
  if (!v) throw new Error('useDomain must be used within DomainProvider')
  return v
}
