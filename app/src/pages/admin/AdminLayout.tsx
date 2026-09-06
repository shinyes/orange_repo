// /admin 顶层壳（src/pages/admin/AdminLayout.tsx）：
// 原 web 管理端 Main 的栏头（品牌/角色/域切换/管理入口/设置/备份/登出）上移为顶部横向栏
// （路由化适配：域管理/空间管理从 view.kind 改为 NavLink 路由）。
// 结构：h-dvh 纵向 = 顶栏（固定） + Outlet 内容区（滚动由子页各自管理）。
import { Suspense, useState } from 'react'
import { NavLink, Outlet } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import {
  ArrowLeftIcon,
  Building2Icon,
  GlobeIcon,
  LayoutGridIcon,
  LogOutIcon,
  SettingsIcon,
  ShieldIcon,
  UserRoundIcon,
  UsersRoundIcon,
} from 'lucide-react'

import type { User } from '@/api/types'
import { adminApi } from '@/api/admin'
import { cn } from '@/lib/utils'
import { AdminDomainProvider, useDomain } from '@/pages/admin/domain-context'
import { AppStateProvider } from '@/pages/admin/app-context'
import { DomainSwitcher } from '@/pages/admin/Sidebar'
import { BackupMenu } from '@/pages/admin/BackupMenu'
import { SettingsDialog } from '@/pages/admin/settings-dialog'

export type AdminShellCtx = { user: User; onLogout: () => void }

export function AdminLayout({ user, onLogout }: { user: User; onLogout: () => void }) {
  return (
    <AdminDomainProvider user={user}>
      <AppStateProvider>
        <div className="flex h-dvh flex-col overflow-hidden">
          <AdminTop user={user} onLogout={onLogout} />
          <Suspense
            fallback={<div className="flex min-h-0 flex-1 items-center justify-center text-sm text-muted-foreground">加载中…</div>}
          >
            <main className="min-h-0 flex-1 overflow-hidden">
              <Outlet context={{ user, onLogout }} />
            </main>
          </Suspense>
        </div>
      </AppStateProvider>
    </AdminDomainProvider>
  )
}

function AdminTop({ user, onLogout }: { user: User; onLogout: () => void }) {
  const { domainId } = useDomain()
  const [settingsOpen, setSettingsOpen] = useState(false)
  const isGlobal = user.role === 'global_admin'
  const noDomain = domainId == null
  const canManageSpaces = user.role === 'domain_admin' || (isGlobal && !noDomain)
  const roleLabel = isGlobal ? '系统管理员' : '域管理员'

  // 当前域名（域名列表：global_admin 全部 / domain_admin 仅其域——见后端 handleListDomains）
  const domainsQ = useQuery({
    queryKey: ['admin', 'domains'],
    queryFn: () => adminApi.domains(),
    enabled: domainId != null,
  })
  const domainName =
    domainsQ.data?.domains.find((d) => d.id === domainId)?.name ?? (noDomain ? null : `域 ${domainId}`)

  const navItems = [
    { to: '/admin/problems', label: '题目管理', icon: ShieldIcon, show: true },
    { to: '/admin/spaces', label: '空间管理', icon: LayoutGridIcon, show: canManageSpaces },
    { to: '/admin/users', label: '用户管理', icon: UsersRoundIcon, show: isGlobal },
    { to: '/admin/domains', label: '域管理', icon: Building2Icon, show: isGlobal },
  ].filter((n) => n.show)

  return (
    <>
      <header className="shrink-0 border-b bg-background">
        <div className="flex h-13 items-center gap-2 px-3 lg:px-4">
          <NavLink
            to="/"
            className="-ml-2 flex shrink-0 items-center gap-1.5 rounded-lg px-2 py-1.5 text-sm text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
            title="返回门户"
          >
            <ArrowLeftIcon className="size-4" />
            <span className="hidden sm:inline">门户</span>
          </NavLink>
          <div className="flex min-w-0 items-center gap-1.5">
            <img src="/favicon.png" alt="OrangeOJ" className="size-6 rounded-md" />
            <span className="truncate text-sm font-semibold">管理端</span>
            <span className="hidden rounded bg-muted px-1.5 py-0.5 text-[10px] text-muted-foreground md:inline">
              {roleLabel}
            </span>
            {!noDomain && domainId != null && domainName != null && (
              <span className="hidden items-center gap-1 rounded bg-muted px-1.5 py-0.5 text-[10px] text-muted-foreground lg:inline-flex">
                <GlobeIcon className="size-3" /> {domainName} #{domainId}
              </span>
            )}
          </div>

          <nav className="ml-2 flex min-w-0 items-center gap-0.5 overflow-x-auto">
            {navItems.map((n) => (
              <NavLink
                key={n.to}
                to={n.to}
                className={({ isActive }) =>
                  cn(
                    'flex shrink-0 items-center gap-1.5 rounded-lg px-2.5 py-1.5 text-sm transition-colors',
                    isActive
                      ? 'bg-primary/10 font-medium text-primary'
                      : 'text-muted-foreground hover:bg-muted hover:text-foreground',
                  )
                }
              >
                <n.icon className="size-4" />
                {n.label}
              </NavLink>
            ))}
          </nav>

          {isGlobal && (
            <div className="ml-auto hidden w-40 shrink-0 sm:block">
              <DomainSwitcher />
            </div>
          )}
          <div className={cn('flex shrink-0 items-center gap-0.5', isGlobal ? '' : 'ml-auto')}>
            <button
              type="button"
              title="设置 / 修改密码 / 清理图片"
              onClick={() => setSettingsOpen(true)}
              className="inline-flex size-7 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
            >
              <SettingsIcon className="size-4" />
            </button>
            <BackupMenu />
            <button
              type="button"
              title="退出登录"
              onClick={onLogout}
              className="inline-flex size-7 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
            >
              <LogOutIcon className="size-4" />
            </button>
          </div>
          <span className="flex shrink-0 items-center gap-1 rounded-full border py-0.5 pl-0.5 pr-2 text-xs text-muted-foreground">
            <span className="flex size-5 items-center justify-center rounded-full bg-primary/15 text-[10px] font-semibold text-primary">
              {user.username.slice(0, 1).toUpperCase()}
            </span>
            <span className="hidden max-w-20 truncate lg:inline">{user.username}</span>
            <UserRoundIcon className="size-3.5 lg:hidden" />
          </span>
        </div>
      </header>
      <SettingsDialog open={settingsOpen} onOpenChange={setSettingsOpen} />
    </>
  )
}
