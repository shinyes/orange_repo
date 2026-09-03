import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { ClipboardCheckIcon, ClipboardXIcon } from 'lucide-react'

import { api } from '@/lib/api'

// /wrong — 错题集：「全部错题」+ 按分类分组入口。
export function WrongListPage() {
  const summary = useQuery({ queryKey: ['wrong-summary'], queryFn: api.wrongSummary })
  const total = summary.data?.total ?? 0

  return (
    <div className="mx-auto w-full max-w-2xl px-4 py-6 lg:max-w-4xl lg:px-8 lg:py-8">
      <h1 className="mb-4 text-lg font-semibold">错题集</h1>

      <Link
        to="/wrong/all"
        aria-disabled={total === 0}
        className={`flex w-full items-center justify-between rounded-xl border bg-card p-4 text-left ${
          total === 0 ? 'pointer-events-none opacity-50' : 'transition-colors hover:border-primary/50 hover:bg-primary/5'
        }`}
      >
        <span className="flex items-center gap-2 font-medium">
          <ClipboardCheckIcon className="size-5 text-primary" />
          全部错题
        </span>
        <span className="text-xs text-muted-foreground">{total} 题</span>
      </Link>

      <div className="mt-5">
        <div className="mb-2 text-xs font-medium text-muted-foreground">按分类</div>
        <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
          {(summary.data?.groups ?? []).map((g) => (
            <Link
              key={g.categoryId}
              to={`/wrong/${g.categoryId}`}
              className="flex w-full items-center justify-between rounded-xl border bg-card p-4 text-left transition-colors hover:border-primary/50 hover:bg-primary/5"
            >
              <span className="min-w-0">
                <span className="block truncate font-medium">{g.categoryName}</span>
                <span className="block text-xs text-muted-foreground">{g.subjectName}</span>
              </span>
              <span className="text-xs text-muted-foreground">{g.count} 题</span>
            </Link>
          ))}
          {total === 0 && (
            <div className="col-span-full rounded-xl border border-dashed p-10 text-center">
              <ClipboardXIcon className="mx-auto mb-2 size-8 text-muted-foreground" />
              <p className="text-sm text-muted-foreground">错题集为空，去刷题页练几道吧</p>
              <Link to="/quiz" className="mt-3 inline-block text-sm text-primary underline underline-offset-4">
                去刷题 →
              </Link>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}