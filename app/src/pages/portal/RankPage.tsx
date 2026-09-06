import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { ArrowLeftIcon, CrownIcon, Loader2Icon, MedalIcon, TrophyIcon } from 'lucide-react'

import { api } from '@/api'
import { usePortalCtx } from './SpaceShell'
import { cn } from '@/lib/utils'

// 排行榜（按域总榜，uuid 去重通过数）：名次 / 用户名 / 通过数；高亮自己。独立页形态，左上返回空间首页。
export function RankPage() {
  const { space, user } = usePortalCtx()
  const q = useQuery({
    queryKey: ['portal-rank', space.domainId],
    queryFn: () => api.portalRank(space.domainId),
  })

  return (
    <div className="mx-auto w-full max-w-2xl px-4 py-5 lg:px-6">
      <Link
        to={`/s/${space.id}`}
        className="inline-flex items-center gap-1.5 text-sm text-muted-foreground transition-colors hover:text-foreground"
      >
        <ArrowLeftIcon className="size-4" /> 返回
      </Link>
      <h1 className="mb-1 mt-2 text-lg font-semibold">排行榜</h1>
      <p className="mb-4 text-xs text-muted-foreground">空间「{space.name}」所属域的总榜（按题目通过数排序）</p>

      {q.isLoading && (
        <div className="flex items-center justify-center gap-2 py-16 text-sm text-muted-foreground">
          <Loader2Icon className="size-4 animate-spin" /> 加载排行榜…
        </div>
      )}
      {q.isError && (
        <div className="rounded-xl border border-dashed p-10 text-center text-sm text-muted-foreground">
          排行榜加载失败（{q.error instanceof Error ? q.error.message : '未知错误'}）
        </div>
      )}

      {q.data && (
        <div className="overflow-hidden rounded-2xl border bg-card">
          {q.data.rank.length === 0 ? (
            <div className="p-10 text-center text-sm text-muted-foreground">暂无排名数据</div>
          ) : (
            q.data.rank.map((row, i) => {
              const me = row.userId === user.id
              const medal = i < 3 ? <MedalIcon className={cn('size-4', i === 0 ? 'text-amber-400' : i === 1 ? 'text-slate-400' : 'text-orange-400')} /> : null
              return (
                <div
                  key={row.userId}
                  className={cn(
                    'flex items-center gap-3 border-b px-4 py-3 text-sm last:border-b-0',
                    me && 'bg-primary/5',
                  )}
                >
                  <span className={cn('flex w-8 shrink-0 items-center justify-center gap-1 text-xs font-semibold', i === 0 && 'text-amber-500')}>
                    {i < 3 ? medal : i + 1}
                  </span>
                  <span className="min-w-0 flex-1 truncate font-medium">
                    {row.username}
                    {me && <span className="ml-1.5 rounded bg-primary/10 px-1 py-0.5 text-[10px] text-primary">我</span>}
                  </span>
                  <span className="flex items-center gap-1 text-xs text-muted-foreground">
                    {i === 0 && <CrownIcon className="size-3.5 text-amber-500" />}
                    <TrophyIcon className="size-3.5" /> {row.solved} 题
                  </span>
                </div>
              )
            })
          )}
        </div>
      )}
    </div>
  )
}
