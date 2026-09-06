import { Link } from 'react-router-dom'
import { BookOpenIcon } from 'lucide-react'

import type { QuizBrief } from '@/api/types'
import { useSpaceHome } from './useSpaceHome'
import { usePortalCtx } from './SpaceShell'

// 空间刷题项目列表（空间壳内 tab 页），点击进入独立刷题页。
export function QuizList() {
  const { space } = usePortalCtx()
  const home = useSpaceHome(space)
  const list = home.data?.quizzes ?? []
  return (
    <div className="mx-auto w-full max-w-3xl px-4 py-6 lg:px-8">
      <h1 className="mb-1 text-lg font-semibold">刷题</h1>
      <p className="mb-4 text-xs text-muted-foreground">随机单题即时反馈：答对记通过（uuid 去重），一组全过即完成；答错不限制次数</p>
      {home.isLoading && <p className="py-10 text-center text-sm text-muted-foreground">加载中…</p>}
      <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
        {list.map((qz) => <QuizCard key={qz.id} q={qz} spaceId={space.id} />)}
      </div>
      {!home.isLoading && list.length === 0 && (
        <div className="rounded-xl border border-dashed p-12 text-center text-sm text-muted-foreground">
          本空间暂无刷题项目
        </div>
      )}
    </div>
  )
}

function QuizCard({ q, spaceId }: { q: QuizBrief; spaceId: number }) {
  return (
    <Link
      to={`/s/${spaceId}/quiz/${q.id}`}
      className="flex w-full flex-col gap-1.5 rounded-2xl border bg-card p-4 transition-colors hover:border-primary/50 hover:bg-primary/5"
    >
      <span className="flex items-center gap-2 font-medium">
        <BookOpenIcon className="size-4 shrink-0 text-primary" />
        <span className="min-w-0 truncate">{q.title}</span>
      </span>
      <span className="mt-1 text-xs text-muted-foreground">
        {q.sourceType === 'repo' ? '题单来源（随机抽题）' : '标签来源（随机抽题）'}
      </span>
    </Link>
  )
}
