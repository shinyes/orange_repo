import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'

import { api } from '@/lib/api'

// /quiz — 刷题首页：科目列表。
export function QuizSubjectsPage() {
  const subjects = useQuery({ queryKey: ['quiz-subjects'], queryFn: api.subjects })

  return (
    <div className="mx-auto w-full max-w-2xl px-4 py-6 lg:max-w-4xl lg:px-8 lg:py-8">
      <h1 className="mb-4 text-lg font-semibold">开始刷题</h1>
      {subjects.isLoading && <p className="text-sm text-muted-foreground">加载科目中…</p>}
      {subjects.isError && <p className="text-sm text-red-600">科目加载失败</p>}
      <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
        {(subjects.data?.subjects ?? []).map((s) => (
          <Link
            key={s.id}
            to={`/quiz/${s.id}`}
            className="flex w-full items-center justify-between rounded-xl border bg-card p-4 text-left transition-colors hover:border-primary/50 hover:bg-primary/5"
          >
            <span className="font-medium">{s.name}</span>
            <span className="text-xs text-muted-foreground">{s.categories.length} 个分类</span>
          </Link>
        ))}
        {(subjects.data?.subjects ?? []).length === 0 && (
          <div className="col-span-full rounded-xl border border-dashed p-8 text-center text-sm text-muted-foreground">
            暂无科目，请联系管理员在系统管理中配置
          </div>
        )}
      </div>
    </div>
  )
}