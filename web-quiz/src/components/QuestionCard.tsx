import { useState } from 'react'
import { CheckCircle2Icon, ChevronRightIcon, XCircleIcon } from 'lucide-react'
import { toast } from 'sonner'

import { Markdown } from '@/lib/markdown'
import type { QuizProblem, SubmitResult } from '@/lib/types'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'

interface QuestionCardProps {
  problem: QuizProblem
  submit: (payload: { optionIndex?: number; answer?: boolean }) => Promise<SubmitResult>
  onNext: () => void
  index: number
  total: number
  nextLabel?: string
  onCorrect?: () => void // 答对时的钩子（错题集练习用于提示移除）
  onWrong?: () => void // 答错时的钩子（本轮统计）
}

const OPTION_LABELS = ['A', 'B', 'C', 'D', 'E', 'F', 'G', 'H']

// 答题卡：点选项即自动提交；答后展示正确/错误、正确答案与「查看解析」。
export function QuestionCard({ problem, submit, onNext, index, total, nextLabel = '下一题', onCorrect, onWrong }: QuestionCardProps) {
  const [result, setResult] = useState<SubmitResult | null>(null)
  const [picked, setPicked] = useState<number | null>(null)
  const [pickedTF, setPickedTF] = useState<boolean | null>(null)
  const [busy, setBusy] = useState(false)
  const [showExplanation, setShowExplanation] = useState(false)

  const options = (problem.bodyJson.options as string[] | undefined) ?? []
  const answered = result !== null

  async function handleSubmit(payload: { optionIndex?: number; answer?: boolean }, pickIndex: number | null, pickTF: boolean | null) {
    if (busy || answered) return
    setBusy(true)
    try {
      const r = await submit(payload)
      setResult(r)
      setPicked(pickIndex)
      setPickedTF(pickTF)
      if (r.correct) {
        onCorrect?.()
      } else {
        toast.error('✗ 回答错误')
        onWrong?.()
      }
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '提交失败，请重试')
    } finally {
      setBusy(false)
    }
  }

  const correctIndex = answered && !result.correct ? (result.correctAnswer.answerIndex ?? -1) : -1
  const correctTF = answered && !result.correct ? (result.correctAnswer.answer ?? null) : null

  return (
    <div className="mx-auto w-full max-w-2xl px-4 py-6">
      {/* 进度 */}
      <div className="mb-4 flex items-center justify-between text-xs text-muted-foreground">
        <span>
          第 {index + 1} / {total} 题
        </span>
        {answered && (
          <span className={result.correct ? 'text-emerald-600' : 'text-red-600'}>
            {result.correct ? '✓ 正确' : '✗ 错误'}
          </span>
        )}
      </div>

      <div className="rounded-2xl border bg-card p-5">
        <div className="mb-3 flex items-center gap-2">
          <Badge>{problem.type === 'single_choice' ? '单选题' : '判断题'}</Badge>
          <h2 className="min-w-0 flex-1 truncate text-base font-semibold">{problem.title}</h2>
        </div>
        <Markdown text={problem.statementMd || '（暂无题面）'} className="markdown-body text-[17px] leading-relaxed" />

        {/* 选项区 */}
        <div className="mt-5 space-y-2.5">
          {problem.type === 'single_choice' ? (
            options.map((opt, i) => {
              const isCorrect = answered && i === (result.correct ? picked : correctIndex)
              const isWrongPick = answered && !result.correct && i === picked
              return (
                <button
                  key={i}
                  type="button"
                  disabled={answered || busy}
                  onClick={() => void handleSubmit({ optionIndex: i }, i, null)}
                  className={cn(
                    'flex w-full items-start gap-2.5 rounded-xl border p-3 text-left text-sm transition-colors',
                    !answered && 'hover:border-primary/60 hover:bg-primary/5 disabled:opacity-60',
                    isCorrect && 'border-emerald-500 bg-emerald-50',
                    isWrongPick && 'border-red-500 bg-red-50',
                    answered && !isCorrect && !isWrongPick && 'opacity-50',
                  )}
                >
                  <span
                    className={cn(
                      'mt-0.5 flex size-6 shrink-0 items-center justify-center rounded-full text-xs font-medium',
                      isCorrect ? 'bg-emerald-500 text-white' : isWrongPick ? 'bg-red-500 text-white' : 'bg-muted text-muted-foreground',
                    )}
                  >
                    {OPTION_LABELS[i] ?? i + 1}
                  </span>
                  <span className="min-w-0 flex-1">
                    <Markdown text={opt} className="markdown-body text-sm" />
                  </span>
                  {isCorrect && <CheckCircle2Icon className="mt-0.5 size-4 shrink-0 text-emerald-600" />}
                  {isWrongPick && <XCircleIcon className="mt-0.5 size-4 shrink-0 text-red-600" />}
                </button>
              )
            })
          ) : (
            <div className="grid grid-cols-2 gap-2.5">
              {[true, false].map((v) => {
                const isCorrect = answered && (result.correct ? pickedTF === v : correctTF === v)
                const isWrongPick = answered && !result.correct && pickedTF === v
                return (
                  <button
                    key={String(v)}
                    type="button"
                    disabled={answered || busy}
                    onClick={() => void handleSubmit({ answer: v }, null, v)}
                    className={cn(
                      'rounded-xl border p-4 text-base font-medium transition-colors',
                      !answered && 'hover:border-primary/60 hover:bg-primary/5 disabled:opacity-60',
                      isCorrect && 'border-emerald-500 bg-emerald-50 text-emerald-700',
                      isWrongPick && 'border-red-500 bg-red-50 text-red-700',
                      answered && !isCorrect && !isWrongPick && 'opacity-50',
                    )}
                  >
                    {v ? '✓ 正确' : '✗ 错误'}
                  </button>
                )
              })}
            </div>
          )}
          {problem.type === 'single_choice' && options.length === 0 && (
            <div className="rounded-lg border border-dashed p-4 text-center text-sm text-muted-foreground">题目选项缺失</div>
          )}
        </div>

        {/* 解析区 */}
        {answered && result.hasExplanation && (
          <div className="mt-4 border-t pt-4">
            {showExplanation ? (
              <div className="rounded-xl border bg-muted/30 p-4">
                <div className="mb-2 text-xs font-medium text-muted-foreground">解析</div>
                <Markdown text={result.explanation || '（暂无解析内容）'} className="markdown-body text-sm" />
              </div>
            ) : (
              <Button variant="outline" size="sm" onClick={() => setShowExplanation(true)}>
                查看解析
              </Button>
            )}
          </div>
        )}

        {/* 下一题 */}
        {answered && (
          <div className="mt-5 flex justify-end">
            <Button onClick={onNext}>
              {nextLabel}
              <ChevronRightIcon className="size-4" />
            </Button>
          </div>
        )}
      </div>
    </div>
  )
}