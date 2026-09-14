import { useCallback, useEffect, useRef } from 'react'
import { NavLink, Outlet, useLocation, useNavigate, useOutletContext, useParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { UserRoundIcon, FolderKanbanIcon, ClipboardListIcon, BookOpenIcon, TrophyIcon, ArrowLeftIcon, BlocksIcon } from 'lucide-react'

import { api } from '@/api'
import { saveSpaceId } from '@/api/space'
import type { PortalSpace, User } from '@/api/types'
import { cn } from '@/lib/utils'

export type PortalContext = { user: User; space: PortalSpace; onLogout: () => void }

// 空间壳：/s/:spaceId/* 布局——单行顶栏：返回空间列表/空间名 + 训练/练习/刷题/排行榜导航 + 我的。
export function SpaceShell({ user, onLogout }: { user: User; onLogout: () => void }) {
  const { spaceId } = useParams()
  const sid = Number(spaceId)
  const navigate = useNavigate()
  const location = useLocation()
  // Scratch 创作页整屏交给编辑器：隐藏本站顶栏（编辑器工具栏最右侧提供「退出」按钮）
  const isScratchEditor = location.pathname.replace(/\/+$/, '').endsWith('/scratch')
  const spacesQ = useQuery({ queryKey: ['portal-spaces'], queryFn: api.portalSpaces })
  const spaces = spacesQ.data?.spaces ?? null

  const space = spaces?.find((s) => s.id === sid) ?? null

  // 空间 id 合法且属于我的空间 → 记录为当前空间
  useEffect(() => {
    if (space) saveSpaceId(space.id)
  }, [space])

  // 空间列表已加载完成（含空列表）：若当前空间不在其中 → 弹回空间选择
  // 注意：这些守卫必须放在**所有 hooks 之后**，否则首帧与后续帧的 hooks 数量不一致
  //（React 报 error #310「Rendered more hooks than during the previous render」）。

  // 编辑器加载优化：Scratch 空间里提前与 Scratch 子域建连（preconnect/dns-prefetch 不传字节），
  // 鼠标悬停「Scratch」页签时再后台预取主包（gzip 后约 5.5MB）——点进去时通常已命中缓存。
  const cfgQ = useQuery({
    queryKey: ['app-config'],
    queryFn: api.appConfig,
    staleTime: 5 * 60_000,
    enabled: space?.kind === 'scratch',
  })
  const scratchOrigin = (cfgQ.data?.scratchUrl ?? '').replace(/\/$/, '')
  const prefetched = useRef(false)
  useEffect(() => {
    if (!scratchOrigin) return
    const links: HTMLLinkElement[] = []
    const add = (rel: string, href: string, as?: string) => {
      const l = document.createElement('link')
      l.rel = rel
      l.href = href
      if (as) l.as = as
      document.head.appendChild(l)
      links.push(l)
    }
    add('preconnect', scratchOrigin)
    add('dns-prefetch', scratchOrigin)
    return () => links.forEach((l) => l.remove())
  }, [scratchOrigin])
  const prefetchEditor = useCallback(() => {
    if (prefetched.current || !scratchOrigin) return
    prefetched.current = true
    const l = document.createElement('link')
    l.rel = 'prefetch'
    l.as = 'script'
    l.href = `${scratchOrigin}/scratch-gui-standalone.js`
    document.head.appendChild(l)
  }, [scratchOrigin])

  // ---- 以下是守卫与派生数据（不得再出现 hooks）----
  if (spacesQ.isError) {
    return <ShellError msg="空间列表加载失败" onBack={() => navigate('/')} />
  }
  if (spaces !== null && !space) {
    return <ShellError msg="空间不存在或无权访问" onBack={() => navigate('/')} />
  }
  if (spaces === null || !space) {
    return (
      <div className="flex h-dvh items-center justify-center text-sm text-muted-foreground">正在进入空间…</div>
    )
  }

  const tabs = [
    { to: `/s/${space.id}/training`, label: '训练', icon: FolderKanbanIcon },
    { to: `/s/${space.id}/practice`, label: '练习', icon: ClipboardListIcon },
    { to: `/s/${space.id}/quiz`, label: '刷题', icon: BookOpenIcon },
    // Scratch 空间（spaces.kind=scratch）：额外提供创作页；训练/练习/刷题保留
    ...(space.kind === 'scratch'
      ? [{ to: `/s/${space.id}/scratch`, label: 'Scratch', icon: BlocksIcon }]
      : []),
    // 排行榜不公开（域设置关闭）时直接不显示该 tab；管理员始终可见
    ...(space.canViewLeaderboard === false
      ? []
      : [{ to: `/s/${space.id}/rank`, label: '排行榜', icon: TrophyIcon }]),
  ]

  return (
    <div className="flex h-dvh flex-col overflow-hidden">
      {/* 顶栏（单行）：返回 + 空间名 + 训练/练习/刷题/排行榜 + 我的 */}
      {!isScratchEditor && (
      <header className="shrink-0 border-b bg-background">
        <div className="mx-auto flex h-11 w-full max-w-5xl items-center gap-1 px-2 lg:px-3">
          <NavLink
            to="/"
            className="-ml-1.5 flex min-w-0 shrink items-center gap-1.5 rounded-lg px-2 py-1.5 text-sm text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
            title="切换空间"
          >
            <ArrowLeftIcon className="size-4 shrink-0" />
            <span className="min-w-0 max-w-36 truncate font-semibold lg:max-w-48">{space.name}</span>
          </NavLink>

          {/* 空间内导航（并入顶栏单行） */}
          <nav className="ml-1 flex min-w-0 flex-1 items-center gap-0.5 overflow-x-auto">
            {tabs.map((t) => (
              <NavLink
                key={t.to}
                to={t.to}
                onMouseEnter={t.label === 'Scratch' ? prefetchEditor : undefined}
                onFocus={t.label === 'Scratch' ? prefetchEditor : undefined}
                className={({ isActive }) =>
                  cn(
                    'flex shrink-0 items-center gap-1.5 rounded-lg px-2.5 py-1.5 text-sm transition-colors',
                    isActive
                      ? 'bg-primary/10 font-medium text-primary'
                      : 'text-muted-foreground hover:bg-muted hover:text-foreground',
                  )
                }
              >
                <t.icon className="size-4" />
                <span className="hidden sm:inline">{t.label}</span>
              </NavLink>
            ))}
          </nav>

          <NavLink
            to="/mine"
            className={({ isActive }) =>
              cn(
                'flex shrink-0 items-center gap-1.5 rounded-full border py-1 pl-1 pr-2.5 text-xs transition-colors',
                isActive
                  ? 'border-primary/40 bg-primary/10 font-medium text-primary'
                  : 'border-border text-muted-foreground hover:border-primary/40 hover:text-foreground',
              )
            }
          >
            <span className="flex size-5 items-center justify-center rounded-full bg-primary/15 text-[10px] font-semibold text-primary">
              {user.username.slice(0, 1).toUpperCase()}
            </span>
            <span className="hidden max-w-20 truncate sm:inline">{user.username}</span>
            <UserRoundIcon className="size-3.5 lg:hidden" />
          </NavLink>
        </div>
      </header>
      )}

      {/* 内容区（编辑器页整屏、不滚动） */}
      <main className={cn('min-h-0 flex-1', isScratchEditor ? 'overflow-hidden' : 'overflow-y-auto')}>
        <Outlet context={{ user, space, onLogout }} />
      </main>
    </div>
  )
}

function ShellError({ msg, onBack }: { msg: string; onBack: () => void }) {
  return (
    <div className="flex h-dvh flex-col items-center justify-center gap-3 px-4 text-center">
      <p className="text-sm text-muted-foreground">{msg}</p>
      <button
        type="button"
        className="rounded-lg border border-input bg-background px-4 py-2 text-sm transition-colors hover:bg-muted"
        onClick={onBack}
      >
        返回空间列表
      </button>
    </div>
  )
}

export function usePortalCtx(): PortalContext {
  return useOutletContext<PortalContext>()
}

