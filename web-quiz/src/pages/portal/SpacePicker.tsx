import { useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link, Navigate } from 'react-router-dom'
import { Globe2Icon, LayoutGridIcon } from 'lucide-react'

import { api } from '@/lib/api'
import { resolveEntrySpace, savedSpaceId, saveSpaceId } from '@/lib/space'
import type { PortalSpace, Role, User } from '@/lib/types'
import { cn } from '@/lib/utils'

// 空间选择页（/）：登录后列出「我的空间」卡片；member 单空间自动直入；
// global_admin 额外展示域信息；存储的当前空间在卡片上标注「上次进入」。
export function SpacePicker({ user }: { user: User }) {
  const q = useQuery({ queryKey: ['portal-spaces'], queryFn: api.portalSpaces })
  const spaces = q.data?.spaces ?? []

  // 持久化当前空间：单空间（自动进入）或有效存储命中时写入。
  useEffect(() => {
    const entry = resolveEntrySpace(spaces)
    if (entry) saveSpaceId(entry.id)
  }, [spaces])

  if (q.isLoading) {
    return (
      <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
        正在加载空间…
      </div>
    )
  }
  if (q.isError) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-3 px-4 text-center">
        <p className="text-sm text-muted-foreground">空间列表加载失败（{q.error instanceof Error ? q.error.message : '未知错误'}）</p>
        <button
          type="button"
          className="rounded-lg border border-input bg-background px-4 py-2 text-sm transition-colors hover:bg-muted"
          onClick={() => void q.refetch()}
        >
          重试
        </button>
      </div>
    )
  }

  // member 单空间：自动进入（无需选择）
  if (user.role === 'member' && spaces.length === 1) {
    return <Navigate to={`/s/${spaces[0].id}/training`} replace />
  }

  const saved = savedSpaceId()

  return (
    <div className="mx-auto w-full max-w-2xl px-4 py-8 lg:max-w-3xl">
      <div className="mb-1 flex items-center gap-2 text-lg font-semibold">
        <img src="/favicon.png" alt="" className="size-7 rounded-lg" />
        我的空间
      </div>
      <p className="mb-5 text-xs text-muted-foreground">{roleNote(user.role)}</p>

      {spaces.length === 0 && (
        <div className="rounded-2xl border border-dashed p-12 text-center">
          <LayoutGridIcon className="mx-auto mb-3 size-8 text-muted-foreground" />
          <p className="text-sm text-muted-foreground">暂无可用空间</p>
          <p className="mt-1 text-xs text-muted-foreground/70">请联系管理员将你加入空间</p>
        </div>
      )}

      <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
        {spaces.map((s) => (
          <SpaceCard key={s.id} space={s} isSaved={s.id === saved} showDomain={user.role === 'global_admin'} />
        ))}
      </div>
    </div>
  )
}

function roleNote(role: Role): string {
  switch (role) {
    case 'member':
      return '选择空间进入做题：'
    case 'domain_admin':
      return '域管理员：可进入本域空间做题（内容在主站仓库页管理）：'
    case 'global_admin':
      return '系统管理员：可查看并进入全部域空间：'
  }
}

function SpaceCard({ space, isSaved, showDomain }: { space: PortalSpace; isSaved: boolean; showDomain: boolean }) {
  return (
    <Link
      to={`/s/${space.id}/training`}
      onClick={() => saveSpaceId(space.id)}
      className={cn(
        'flex w-full flex-col gap-1.5 rounded-2xl border bg-card p-5 transition-colors hover:border-primary/50 hover:bg-primary/5',
        isSaved && 'border-primary/50 bg-primary/5',
      )}
    >
      <span className="flex items-center gap-2 font-medium">
        <LayoutGridIcon className="size-4 shrink-0 text-primary" />
        <span className="truncate">{space.name}</span>
        {isSaved && <span className="rounded bg-primary/10 px-1.5 py-0.5 text-[10px] text-primary">上次进入</span>}
      </span>
      {showDomain && (
        <span className="flex items-center gap-1 text-xs text-muted-foreground">
          <Globe2Icon className="size-3.5" />
          域 #{space.domainId}
        </span>
      )}
      <span className="mt-2 inline-flex h-7 w-fit items-center gap-1 rounded-lg border border-border bg-background px-2.5 text-xs text-muted-foreground transition-colors hover:bg-muted hover:text-foreground">
        进入空间 →
      </span>
    </Link>
  )
}
