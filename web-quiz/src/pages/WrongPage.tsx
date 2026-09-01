import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { ArrowLeftIcon, ClipboardCheckIcon, ClipboardXIcon, TrophyIcon } from 'lucide-react'
import { toast } from 'sonner'

import { api } from '@/lib/api'
import type { WrongRound } from '@/lib/types'
import { Button } from '@/components/ui/button'
import { QuestionCard } from '@/components/QuestionCard'

type Stage =
  | { kind: 'list' }
  | { kind: 'playing'; round: WrongRound; idx: number; correct: number; wrong: number }

// 错题页：「全部错题」+ 按分类分组；答对即自动从错题集移除。
export function WrongPage() {
  const [stage, setStage] = useState<Stage>({ kind: 'list' })
  const summary = useQuery({ queryKey: ['wrong-summary'], queryFn: api.wrongSummary })

  async function startWrongRound(categoryId?: number) {
    try {
      const round = await api.startWrongRound(categoryId)
      if (round.problems.length === 0) {
        toast.info('错题集是空的')
        return
      }
      setStage({ kind: 'playing', round, idx: 0, correct: 0, wrong: 0 })
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '加载错题失败')
    }
  }

  if (stage.kind === 'list') {
    const total = summary.data?.total ?? 0
    return (
      <div className="mx-auto w-full max-w-2xl px-4 py-6">
        <h1 className="mb-4 text-lg font-semibold">错题集</h1>

        {/* 全部错题入口 */}
        <button
          type="button"
          onClick={() => void startWrongRound()}
          disabled={total === 0}
          className="flex w-full items-center justify-between rounded-xl border bg-card p-4 text-left transition-colors enabled:hover:border-primary/50 enabled:hover:bg-primary/5 disabled:opacity-50"
        >
          <span className="flex items-center gap-2 font-medium">
            <ClipboardCheckIcon className="size-5 text-primary" />
            全部错题
          </span>
          <span className="text-xs text-muted-foreground">{total} 题</span>
        </button>

        {/* 按分类分组 */}
        <div className="mt-5 space-y-3">
          <div className="text-xs font-medium text-muted-foreground">按分类</div>
          {(summary.data?.groups ?? []).map((g) => (
            <button
              key={g.categoryId}
              type="button"
              onClick={() => void startWrongRound(g.categoryId)}
              className="flex w-full items-center justify-between rounded-xl border bg-card p-4 text-left transition-colors hover:border-primary/50 hover:bg-primary/5"
            >
              <span className="min-w-0">
                <span className="block truncate font-medium">{g.categoryName}</span>
                <span className="block text-xs text-muted-foreground">{g.subjectName}</span>
              </span>
              <span className="text-xs text-muted-foreground">{g.count} 题</span>
            </button>
          ))}
          {total === 0 && (
            <div className="rounded-xl border border-dashed p-10 text-center">
              <ClipboardXIcon className="mx-auto mb-2 size-8 text-muted-foreground" />
              <p className="text-sm text-muted-foreground">错题集为空，去刷题页练几道吧</p>
            </div>
          )}
        </div>
      </div>
    )
  }

  // 错题练习
  const { round, idx } = stage
  const problem = round.problems[idx]
  return (
    <div>
      <div className="mx-auto flex w-full max-w-2xl px-4 pt-4">
        <Button variant="ghost" size="sm" className="-ml-2 text-muted-foreground" onClick={() => setStage({ kind: 'list' })}>
          <ArrowLeftIcon className="size-4" /> 错题列表
        </Button>
      </div>
      <QuestionCard
        key={problem.id}
        problem={problem}
        index={idx}
        total={round.problems.length}
        submit={(payload) => api.submit({ ...payload, problemId: problem.id, categoryId: problem.categoryId })}
        onNext={() => {
          if (idx + 1 < round.problems.length) {
            setStage({ ...stage, idx: idx + 1 })
          } else {
            setStage({ kind: 'list' })
            void summary.refetch()
          }
        }}
        onCorrect={() => {
          setStage({ ...stage, correct: stage.correct + 1 })
          toast('已从错题集移除', { description: '答对啦，这道题已自动移出错题集' })
        }}
        onWrong={() => setStage({ ...stage, wrong: stage.wrong + 1 })}
      />
      {stage.correct === round.problems.length && stage.wrong === 0 && (
        <div className="mx-auto w-full max-w-2xl px-4 pb-6">
          <div className="flex items-center gap-2 rounded-xl bg-emerald-50 p-3 text-sm text-emerald-700">
            <TrophyIcon className="size-4" />
            本轮全部答对，错题已全部移出错题集
          </div>
        </div>
      )}
    </div>
  )
}