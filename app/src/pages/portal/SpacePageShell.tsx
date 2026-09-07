// 独立全屏页外壳（portal）：点空间内卡片进入的列表/详情/作答/排行榜页共用——
// 自带顶栏：左「← 返回 上级页」+ 空间名（点回空间菜单）；右「我的」入口。
// 与 SpaceShell（空间菜单首页壳）视觉同构，营造"进入新页面"的完整观感。
import { Link, NavLink } from 'react-router-dom'
import { ArrowLeftIcon, UserRoundIcon } from 'lucide-react'

import { usePortalSession } from './portal-context'
import { cn } from '@/lib/utils'

export function SpacePageShell({
  spaceId,
  backTo,
  backLabel,
  spaceName,
  headerExtra,
  children,
}: {
  spaceId: number
  /** 返回目标（相对或绝对路径） */
  backTo: string
  /** 返回按钮文字（如 返回训练列表 / 返回空间） */
  backLabel: string
  spaceName?: string
  /** 顶栏右端扩展区域（在「我的」入口前；供练习页放 提交/保存/全部记录 等按钮） */
  headerExtra?: React.ReactNode
  children: React.ReactNode
}) {
  const { user } = usePortalSession()
  return (
    <div className="flex h-dvh flex-col overflow-hidden">
      <header className="shrink-0 border-b bg-background">
        <div className="mx-auto flex h-14 w-full max-w-5xl items-center gap-2 px-2 lg:px-3">
          <Link
            to={backTo}
            className="-ml-1.5 inline-flex min-w-0 items-center gap-1.5 rounded-lg px-2 py-1.5 text-sm text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
          >
            <ArrowLeftIcon className="size-4 shrink-0" />
            <span className="min-w-0 truncate">{backLabel}</span>
          </Link>
          {spaceName && (
            <Link
              to={`/s/${spaceId}`}
              className="hidden min-w-0 items-center gap-1 rounded-lg px-2 py-1.5 text-xs text-muted-foreground transition-colors hover:bg-muted hover:text-foreground sm:inline-flex"
              title="回到空间首页"
            >
              {spaceName}
            </Link>
          )}
          <div className="flex-1" />
          {headerExtra}
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
      <main className="min-h-0 flex-1 overflow-y-auto">{children}</main>
    </div>
  )
}

/** 页内内容居中容器（与 SpaceShell 内容宽度一致）。 */
export function PageContainer({ children, className }: { children: React.ReactNode; className?: string }) {
  return <div className={cn('mx-auto w-full max-w-3xl px-4 py-5 lg:px-8', className)}>{children}</div>
}
