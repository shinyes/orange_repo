import { useQuery } from '@tanstack/react-query'
import { Link, useParams } from 'react-router-dom'
import { ArrowLeftIcon, FolderKanbanIcon } from 'lucide-react'

import { api } from '@/lib/api'
import type { OjTrainingDetail } from '@/lib/types'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { ProgressBar } from './AssignmentCards'
import { typeLabel } from './oj-utils'

// 训练详情：章节手风琴 + 章节网格导航 + 上一题/下一题（页面即做题上下文入口）。
export function TrainingPage() {
  const { id } = useParams()
  const tid = Number(id)
  const q = useQuery({ queryKey: ['oj-training', tid], queryFn: () => api.ojTraining(tid) })
  const data = q.data

  if (q.isLoading) return <div className="p-10 text-center text-sm text-muted-foreground">加载中…</div>
  if (q.isError || !data) {
    return <ErrorBox msg="任务不存在或未发布" />
  }
  if (data.stale || (data.chapters.length === 0 && data.total === 0)) {
    return <ErrorBox msg="该训练内容已失效（主库训练可能已被删除）" />
  }
  const pct = data.total > 0 ? Math.round((data.accepted / data.total) * 100) : 0
  return (
    <div className="mx-auto w-full max-w-3xl px-4 py-5">
      <Link to="/training">
        <Button variant="ghost" size="sm" className="-ml-2 text-muted-foreground">
          <ArrowLeftIcon className="size-4" /> 返回训练
        </Button>
      </Link>
      <div className="mt-2 rounded-2xl border bg-card p-5">
        <div className="flex items-start justify-between gap-3">
          <div>
            <h1 className="flex items-center gap-2 text-lg font-semibold">
              <FolderKanbanIcon className="size-5 text-primary" />
              {data.title}
            </h1>
            {data.description && <p className="mt-1 text-sm text-muted-foreground">{data.description}</p>}
          </div>
          <Badge variant="secondary">训练</Badge>
        </div>
        <div className="mt-4">
          <ProgressBar accepted={data.accepted} total={data.total} pct={pct} />
        </div>
      </div>

      <div className="mt-4 space-y-3">
        {data.chapters.map((ch) => <ChapterBlock key={ch.id} chId={ch.id} title={ch.title} items={ch.items} />)}
      </div>
    </div>
  )
}

function ChapterBlock({ chId, title, items }: { chId: number; title: string; items: OjTrainingDetail['chapters'][number]['items'] }) {
  const allDone = items.length > 0 && items.every((i) => i.completed)
  return (
    <div className="rounded-2xl border bg-card">
      <div className={cn('flex items-center gap-2 border-b px-4 py-2.5 text-sm font-semibold', allDone && 'text-emerald-600')}>
        <span className="min-w-0 flex-1 truncate">{title || `第 ${chId} 章`}</span>
        {allDone && <Badge className="bg-emerald-50 text-emerald-700 hover:bg-emerald-50">已完成</Badge>}
        <span className="text-xs font-normal text-muted-foreground">{items.length} 题</span>
      </div>
      <div className="grid grid-cols-1 gap-2 p-3 sm:grid-cols-2">
        {items.map((it, idx) => (
          <ProblemRow key={it.problemId} problemId={it.problemId} title={it.title} type={it.type} completed={it.completed} idx={idx} />
        ))}
      </div>
    </div>
  )
}

export function ProblemRow({ problemId, title, type, completed, idx }: {
  problemId: number
  title: string
  type: string
  completed: boolean
  idx?: number
}) {
  return (
    <Link
      to={`/problem/${problemId}?back=${encodeURIComponent(location.pathname)}`}
      className={cn(
        'flex items-center gap-2.5 rounded-xl border p-3 transition-colors hover:border-primary/50 hover:bg-primary/5',
        completed && 'border-emerald-200 bg-emerald-50/40',
      )}
    >
      <span className={cn(
        'flex size-7 shrink-0 items-center justify-center rounded-lg text-xs font-semibold',
        completed ? 'bg-emerald-500 text-white' : 'bg-muted text-muted-foreground',
      )}>
        {completed ? '✓' : (idx ?? 0) + 1}
      </span>
      <span className="min-w-0 flex-1 truncate text-sm font-medium">{title || `#${problemId}`}</span>
      <Badge variant="outline" className="shrink-0 text-[10px]">{typeLabel(type)}</Badge>
    </Link>
  )
}

function ErrorBox({ msg }: { msg: string }) {
  return (
    <div className="mx-auto w-full max-w-3xl px-4 py-16 text-center">
      <div className="rounded-2xl border bg-card p-8">
        <p className="text-sm text-muted-foreground">{msg}</p>
        <Link to="/training" className="mt-4 inline-block">
          <Button variant="outline">返回训练</Button>
        </Link>
      </div>
    </div>
  )
}
