import { useQuery } from '@tanstack/react-query'
import { Link, useParams } from 'react-router-dom'
import { ArrowLeftIcon } from 'lucide-react'

import { api } from '@/lib/api'
import { Button } from '@/components/ui/button'

// /quiz/:subjectId — 科目下的分类列表（返回 = 浏览器后退或顶部按钮）。
export function QuizCategoriesPage() {
  const { subjectId } = useParams()
  const subjects = useQuery({ queryKey: ['quiz-subjects'], queryFn: api.subjects })
  const subject = subjects.data?.subjects.find((s) => String(s.id) === subjectId)

  return (
    <div className="mx-auto w-full max-w-2xl px-4 py-6">
      <Link to="/quiz">
        <Button variant="ghost" size="sm" className="-ml-2 mb-3 text-muted-foreground">
          <ArrowLeftIcon className="size-4" /> 返回科目
        </Button>
      </Link>
      <h1 className="mb-4 text-lg font-semibold">{subject?.name ?? '分类'}</h1>
      <div className="space-y-3">
        {(subject?.categories ?? []).map((c) => (
          <Link
            key={c.id}
            to={`/quiz/${subjectId}/${c.id}`}
            aria-disabled={c.questionCount === 0}
            className={`flex w-full items-center justify-between rounded-xl border bg-card p-4 text-left transition-colors ${
              c.questionCount === 0
                ? 'pointer-events-none opacity-50'
                : 'hover:border-primary/50 hover:bg-primary/5'
            }`}
          >
            <span className="font-medium">{c.name}</span>
            <span className="text-xs text-muted-foreground">
              {c.questionCount === 0 ? '暂无题目' : `${c.questionCount} 题`}
            </span>
          </Link>
        ))}
        {subject && subject.categories.length === 0 && (
          <div className="rounded-xl border border-dashed p-8 text-center text-sm text-muted-foreground">
            该科目下暂无分类
          </div>
        )}
        {!subject && !subjects.isLoading && (
          <div className="rounded-xl border border-dashed p-8 text-center text-sm text-muted-foreground">
            科目不存在，请返回重新选择
          </div>
        )}
      </div>
    </div>
  )
}