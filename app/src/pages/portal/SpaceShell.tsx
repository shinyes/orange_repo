import { useEffect } from 'react'
import { Link, NavLink, Outlet, useNavigate, useOutletContext, useParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { LayoutGridIcon, UserRoundIcon, FolderKanbanIcon, ClipboardListIcon, BookOpenIcon, TrophyIcon, ArrowLeftIcon } from 'lucide-react'

import { api } from '@/api'
import { saveSpaceId } from '@/api/space'
import type { PortalSpace, User } from '@/api/types'
import { cn } from '@/lib/utils'

export type PortalContext = { user: User; space: PortalSpace; onLogout: () => void }

// 空间壳：/s/:spaceId/* 布局——顶栏（返回空间列表/空间名/我的）+ 顶部 Tab 导航（训练/练习/刷题/排行榜）。
export function SpaceShell({ user, onLogout }: { user: User; onLogout: () => void }) {
  const { spaceId } = useParams()
  const sid = Number(spaceId)
  const navigate = useNavigate()
  const spacesQ = useQuery({ queryKey: ['portal-spaces'], queryFn: api.portalSpaces })
  const spaces = spacesQ.data?.spaces ?? null

  const space = spaces?.find((s) => s.id === sid) ?? null

  // 空间 id 合法且属于我的空间 → 记录为当前空间
  useEffect(() => {
    if (space) saveSpaceId(space.id)
  }, [space])

  // 空间列表已加载完成（含空列表）：若当前空间不在其中 → 弹回空间选择
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
    { to: `/s/${space.id}/rank`, label: '排行榜', icon: TrophyIcon },
  ]

  return (
    <div className="flex h-dvh flex-col overflow-hidden">
      {/* 顶栏 */}
      <header className="shrink-0 border-b bg-background">
        <div className="mx-auto flex h-13 w-full max-w-5xl items-center gap-2 px-3 lg:px-5">
          <NavLink
            to="/"
            className="-ml-2 flex shrink-0 items-center gap-1.5 rounded-lg px-2 py-1.5 text-sm text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
            title="切换空间"
          >
            <ArrowLeftIcon className="size-4" />
          </NavLink>
          <div className="flex min-w-0 flex-1 items-center gap-2">
            <LayoutGridIcon className="size-4 shrink-0 text-primary" />
            <span className="truncate text-sm font-semibold">{space.name}</span>
            {user.role !== 'member' && (
              <span className="hidden shrink-0 rounded bg-muted px-1.5 py-0.5 text-[10px] text-muted-foreground sm:inline">
                {user.role === 'domain_admin' ? '域管理' : '系统管理'}
              </span>
            )}
          </div>
          <SpaceAdminEntry user={user} />
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

        {/* 顶部 Tab 导航（PC/移动通用） */}
        <nav className="mx-auto flex w-full max-w-5xl gap-1 overflow-x-auto px-2 pb-1 lg:px-4">
          {tabs.map((t) => (
            <NavLink
              key={t.to}
              to={t.to}
              className={({ isActive }) =>
                cn(
                  'flex shrink-0 items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm transition-colors',
                  isActive
                    ? 'bg-primary/10 font-medium text-primary'
                    : 'text-muted-foreground hover:bg-muted hover:text-foreground',
                )
              }
            >
              <t.icon className="size-4" />
              {t.label}
            </NavLink>
          ))}
        </nav>
      </header>

      {/* 内容区 */}
      <main className="min-h-0 flex-1 overflow-y-auto">
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

// 管理员的空间管理入口：本应用内导航至 /admin（域/空间/题目内容统一在管理区维护）。
function SpaceAdminEntry({ user }: { user: User }) {
  if (user.role !== 'domain_admin' && user.role !== 'global_admin') return null
  return (
    <Link
      to="/admin/spaces"
      title="空间内容（训练/练习/刷题与成员）在管理区维护"
      className="hidden shrink-0 rounded-lg border border-dashed px-2.5 py-1 text-[11px] text-muted-foreground transition-colors hover:border-primary/40 hover:text-foreground md:inline-flex md:items-center"
    >
      空间管理 →
    </Link>
  )
}

export function usePortalCtx(): PortalContext {
  return useOutletContext<PortalContext>()
}

