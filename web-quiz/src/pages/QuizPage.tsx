import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { ArrowLeftIcon, BookOpenIcon, RefreshCwIcon, TrophyIcon } from 'lucide-react'
import { toast } from 'sonner'

import { api } from '@/lib/api'
import type { Round, SubjectBrief } from '@/lib/types'
import { Button } from '@/components/ui/button'
import { QuestionCard } from '@/components/QuestionCard'

type Stage =
  | { kind: 'subjects' }
  | { kind: 'categories'; subject: SubjectBrief }
  | { kind: 'playing'; round: Round; idx: number; correct: number; wrong: number }
  | { kind: 'done'; correct: number; wrong: number; categoryId: number }

// 刷题页：选科目 → 选分类 → 随机顺序做题 → 本轮统计。
export function QuizPage() {
  const [stage, setStage] = useState<Stage>({ kind: 'subjects' })
  const subjects = useQuery({ queryKey: ['quiz-subjects'], queryFn: api.subjects })

  async function startRound(categoryId: number) {
    try {
      const round = await api.startRound(categoryId)
      if (round.problems.length === 0) {
        toast.info('该分类暂无符合条件的题目')
        return
      }
      setStage({ kind: 'playing', round, idx: 0, correct: 0, wrong: 0 })
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '抽题失败')
    }
  }

  if (stage.kind === 'subjects' || stage.kind === 'categories') {
    return (
      <div className="mx-auto w-full max-w-2xl px-4 py-6">
        <h1 className="mb-4 text-lg font-semibold">开始刷题</h1>
        {subjects.isLoading && <p className="text-sm text-muted-foreground">加载科目中…</p>}
        {subjects.isError && <p className="text-sm text-red-600">科目加载失败</p>}
        {stage.kind === 'subjects' ? (
          <div className="space-y-3">
            {(subjects.data?.subjects ?? []).map((s) => (
              <button
                key={s.id}
                type="button"
                onClick={() => setStage({ kind: 'categories', subject: s })}
                className="flex w-full items-center justify-between rounded-xl border bg-card p-4 text-left transition-colors hover:border-primary/50 hover:bg-primary/5"
              >
                <span className="font-medium">{s.name}</span>
                <span className="text-xs text-muted-foreground">{s.categories.length} 个分类</span>
              </button>
            ))}
            {(subjects.data?.subjects ?? []).length === 0 && (
              <div className="rounded-xl border border-dashed p-8 text-center text-sm text-muted-foreground">
                暂无科目，请联系管理员在系统管理中配置
              </div>
            )}
          </div>
        ) : (
          <div>
            <Button variant="ghost" size="sm" className="mb-3 -ml-2" onClick={() => setStage({ kind: 'subjects' })}>
              <ArrowLeftIcon className="size-4" /> 返回科目
            </Button>
            <div className="space-y-3">
              {stage.subject.categories.map((c) => (
                <button
                  key={c.id}
                  type="button"
                  disabled={c.questionCount === 0}
                  onClick={() => void startRound(c.id)}
                  className="flex w-full items-center justify-between rounded-xl border bg-card p-4 text-left transition-colors enabled:hover:border-primary/50 enabled:hover:bg-primary/5 disabled:opacity-50"
                >
                  <span className="font-medium">{c.name}</span>
                  <span className="text-xs text-muted-foreground">
                    {c.questionCount === 0 ? '暂无题目' : `${c.questionCount} 题`}
                  </span>
                </button>
              ))}
              {stage.subject.categories.length === 0 && (
                <div className="rounded-xl border border-dashed p-8 text-center text-sm text-muted-foreground">
                  该科目下暂无分类
                </div>
              )}
            </div>
          </div>
        )}
      </div>
    )
  }

  if (stage.kind === 'playing') {
    const { round, idx } = stage
    const problem = round.problems[idx]
    return (
      <QuestionCard
        key={problem.id}
        problem={problem}
        index={idx}
        total={round.problems.length}
        submit={(payload) => api.submit({ ...payload, problemId: problem.id, categoryId: round.categoryId })}
        nextLabel={idx + 1 < round.problems.length ? '下一题' : '完成'}
        onNext={() => {
          if (idx + 1 < round.problems.length) {
            setStage({ ...stage, idx: idx + 1 })
          } else {
            setStage({ kind: 'done', correct: stage.correct, wrong: stage.wrong, categoryId: round.categoryId })
          }
        }}
        onCorrect={() => setStage({ ...stage, correct: stage.correct + 1 })}
        onWrong={() => setStage({ ...stage, wrong: stage.wrong + 1 })}
      />
    )
  }

  // done
  return (
    <div className="mx-auto w-full max-w-2xl px-4 py-10">
      <div className="rounded-2xl border bg-card p-8 text-center">
        <TrophyIcon className="mx-auto mb-3 size-10 text-primary" />
        <h2 className="text-xl font-semibold">本轮完成！</h2>
        <div className="mt-4 flex justify-center gap-6 text-sm">
          <div className="rounded-xl bg-emerald-50 px-6 py-3">
            <div className="text-2xl font-bold text-emerald-600">{stage.correct}</div>
            <div className="text-xs text-muted-foreground">答对</div>
          </div>
          <div className="rounded-xl bg-red-50 px-6 py-3">
            <div className="text-2xl font-bold text-red-600">{stage.wrong}</div>
            <div className="text-xs text-muted-foreground">答错</div>
          </div>
        </div>
        <div className="mt-6 flex justify-center gap-3">
          <Button onClick={() => void startRound(stage.categoryId)}>
            <RefreshCwIcon className="size-4" /> 再练一轮
          </Button>
          <Button variant="outline" onClick={() => setStage({ kind: 'subjects' })}>
            <BookOpenIcon className="size-4" /> 返回分类
          </Button>
        </div>
      </div>
    </div>
  )
}