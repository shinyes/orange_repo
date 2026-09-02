import { useQuery } from '@tanstack/react-query'
import { Link, useParams } from 'react-router-dom'
import { ArrowLeftIcon, ClipboardListIcon } from 'lucide-react'

import { api } from '@/lib/api'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { ProgressBar } from './AssignmentCards'
import { ProblemRow } from './TrainingPage'

// 练习详情：平铺题单。
export function PracticePage() {
  const { id } = useParams()
  const pid = Number(id)
  const q = useQuery({ queryKey: ['oj-practice', pid], queryFn: () => api.ojPractice(pid) })
  const data = q.data

  if (q.isLoading) return <div className="p-10 text-center text-sm text-muted-foreground">加载中…</div>
  if (q.isError || !data) {
    return <ErrorBox msg="任务不存在或未发布" />
  }
  if (data.stale || (data.items.length === 0 && data.total === 0)) {
    return <ErrorBox msg="该练习内容已失效（主库练习可能已被删除）" />
  }
  const pct = data.total > 0 ? Math.round((data.accepted / data.total) * 100) : 0
  return (
    <div className="mx-auto w-full max-w-3xl px-4 py-5">
      <Link to="/practice">
        <Button variant="ghost" size="sm" className="-ml-2 text-muted-foreground">
          <ArrowLeftIcon className="size-4" /> 返回练习
        </Button>
      </Link>
      <div className="mt-2 rounded-2xl border bg-card p-5">
        <div className="flex items-start justify-between gap-3">
          <div>
            <h1 className="flex items-center gap-2 text-lg font-semibold">
              <ClipboardListIcon className="size-5 text-primary" />
              {data.title}
            </h1>
            {data.description && <p className="mt-1 text-sm text-muted-foreground">{data.description}</p>}
          </div>
          <Badge variant="secondary">练习</Badge>
        </div>
        <div className="mt-4">
          <ProgressBar accepted={data.accepted} total={data.total} pct={pct} />
        </div>
      </div>

      <div className="mt-4 space-y-2">
        {data.items.map((it, idx) => (
          <ProblemRow key={it.problemId} problemId={it.problemId} title={it.title} type={it.type} completed={it.completed} idx={idx} />
        ))}
      </div>
    </div>
  )
}

function ErrorBox({ msg }: { msg: string }) {
  return (
    <div className="mx-auto w-full max-w-3xl px-4 py-16 text-center">
      <div className="rounded-2xl border bg-card p-8">
        <p className="text-sm text-muted-foreground">{msg}</p>
        <Link to="/practice" className="mt-4 inline-block">
          <Button variant="outline">返回练习</Button>
        </Link>
      </div>
    </div>
  )
}
