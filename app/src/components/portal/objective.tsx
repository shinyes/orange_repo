import { CheckCircle2Icon, XCircleIcon } from 'lucide-react'
import { Markdown, preserveLineBreaks } from '@/lib/markdown'
import type { CorrectAnswer, ObjectiveAnswer, QuizProblem } from '@/api/types'
import { cn } from '@/lib/utils'

export const OPTION_LABELS = ['A', 'B', 'C', 'D', 'E', 'F', 'G', 'H'] as const

export function problemTypeText(t?: string): string {
  switch (t) {
    case 'programming':
      return '编程'
    case 'single_choice':
      return '单选'
    case 'true_false':
      return '判断'
    default:
      return t || '题目'
  }
}

// 客观题作答视图（题面 + 选项按钮），供 训练内联作答 / 练习整卷 / 刷题单题 复用。
export interface ObjectiveQuestionProps {
  problem: Pick<QuizProblem, 'type' | 'statementMd' | 'bodyJson'>
  /** 完全锁定（已达上限/已答对）：不可再选 */
  locked?: boolean
  /** 提交进行中 */
  busy?: boolean
  /** 用户当前已选（尚未判分或判分中显示为主色勾选态） */
  selected?: ObjectiveAnswer | null
  /** 判分反馈；非空时展示对/错 + 正确项，并禁止修改 */
  feedback?: { correct: boolean; correctAnswer?: CorrectAnswer } | null
  /** silent：有 feedback 但不显示结果横幅（训练回顾态复用判分结果展示选项红绿） */
  silent?: boolean
  /** 答错且未揭示答案时的提示（如“还可再答 2 次”） */
  wrongHint?: string
  /** 用户点选（自动提交流由调用方在回调内发起请求） */
  onSelect?: (a: ObjectiveAnswer) => void
  showTitle?: string
}

export function ObjectiveQuestion({
  problem,
  locked,
  busy,
  selected,
  feedback,
  silent,
  wrongHint,
  onSelect,
  showTitle,
}: ObjectiveQuestionProps) {
  const options = (problem.bodyJson?.options as string[] | undefined) ?? []
  const answered = !!feedback
  const correctVal: ObjectiveAnswer | undefined = feedback
    ? feedback.correct
      ? selected ?? undefined
      : (feedback.correctAnswer?.answerIndex ?? feedback.correctAnswer?.answer)
    : undefined

  const clickable = !answered && !locked && !busy && !!onSelect
  // 是否正确答案已被揭示（次数用尽/回顾态才有 correctAnswer；未用尽时不下发）
  const revealedAnswer = !!feedback && !feedback.correct && (
    problem.type === 'single_choice'
      ? feedback.correctAnswer?.answerIndex != null
      : feedback.correctAnswer?.answer != null
  )

  // 选项框底色/边框统一逻辑（单选/判断共用）。
  function optionCls(value: ObjectiveAnswer): string {
    const isPicked = !answered && selected === value
    const isCorrectOption = answered && correctVal === value
    const isWrongPick = answered && !feedback!.correct && selected === value
    return cn(
      'rounded-xl border transition-colors',
      clickable && 'hover:border-primary/60 hover:bg-primary/5 disabled:opacity-60',
      isPicked && 'border-primary/70 bg-primary/5',
      isCorrectOption && 'border-emerald-500 bg-emerald-50',
      isWrongPick && 'border-red-500 bg-red-50',
      answered && !isCorrectOption && !isWrongPick && 'opacity-50',
    )
  }

  return (
    <div>
      {showTitle && <div className="mb-2 text-sm font-semibold">{showTitle}</div>}
      <Markdown
        text={preserveLineBreaks(problem.statementMd || '（暂无题面）')}
        className="markdown-body text-[16px] leading-relaxed"
      />
      <div className="mt-4 space-y-2.5">
        {problem.type === 'single_choice' ? (
          options.length > 0 ? (
            options.map((opt, i) => (
              <button
                key={i}
                type="button"
                disabled={!clickable}
                onClick={() => onSelect?.(i)}
                className={cn('flex w-full items-start gap-2.5 p-3 text-left text-sm', optionCls(i))}
              >
                <OptionBadge value={i} answered={answered} correctVal={correctVal} selected={selected} label={OPTION_LABELS[i] ?? i + 1} />
                <span className="min-w-0 flex-1">
                  <Markdown text={preserveLineBreaks(opt)} className="markdown-body text-sm" />
                </span>
                <ResultIcon value={i} answered={answered} correctVal={correctVal} selected={selected} />
              </button>
            ))
          ) : (
            <div className="rounded-lg border border-dashed p-4 text-center text-sm text-muted-foreground">题目选项缺失</div>
          )
        ) : (
          <div className="grid grid-cols-2 gap-2.5">
            {[true, false].map((v) => (
              <button
                key={String(v)}
                type="button"
                disabled={!clickable}
                onClick={() => onSelect?.(v)}
                className={cn('flex items-center justify-center p-4 text-base', optionCls(v))}
              >
                <span className="font-medium">{v ? '正确' : '错误'}</span>
              </button>
            ))}
          </div>
        )}
      </div>

      {answered && !silent && (
        <div
          className={cn(
            'mt-4 flex items-center gap-2 rounded-xl border p-3 text-sm',
            feedback!.correct
              ? 'border-emerald-200 bg-emerald-50 text-emerald-700'
              : 'border-red-200 bg-red-50 text-red-700',
          )}
        >
          {feedback!.correct ? <CheckCircle2Icon className="size-4 shrink-0" /> : <XCircleIcon className="size-4 shrink-0" />}
          <span className="min-w-0">
            {feedback!.correct ? '回答正确' : '回答错误'}
            {!feedback!.correct && revealedAnswer && (
              <span className="ml-1 text-xs opacity-80">
                （正确项：{problem.type === 'single_choice'
                  ? (() => {
                    const idx = feedback!.correctAnswer?.answerIndex
                    return idx != null ? `选项 ${OPTION_LABELS[idx] ?? idx}` : '—'
                  })()
                  : feedback!.correctAnswer?.answer == null ? '—' : (feedback!.correctAnswer.answer ? '对' : '错')}）
              </span>
            )}
            {!feedback!.correct && !revealedAnswer && wrongHint && (
              <span className="ml-1 text-xs opacity-80">（{wrongHint}）</span>
            )}
          </span>
        </div>
      )}
    </div>
  )
}

function OptionBadge({ value, answered, correctVal, selected, label }: {
  value: ObjectiveAnswer
  answered: boolean
  correctVal?: ObjectiveAnswer
  selected?: ObjectiveAnswer | null
  label: string | number
}) {
  const isCorrectOption = answered && correctVal === value
  const isPicked = !answered && selected === value
  const isWrongPick = answered && !isCorrectOption && selected === value
  return (
    <span
      className={cn(
        'mt-0.5 flex size-6 shrink-0 items-center justify-center rounded-full text-xs font-medium',
        isCorrectOption
          ? 'bg-emerald-500 text-white'
          : isWrongPick || isPicked
            ? isWrongPick ? 'bg-red-500 text-white' : 'bg-primary text-primary-foreground'
            : 'bg-muted text-muted-foreground',
      )}
    >
      {label}
    </span>
  )
}

function ResultIcon({ value, answered, correctVal, selected }: {
  value: ObjectiveAnswer
  answered: boolean
  correctVal?: ObjectiveAnswer
  selected?: ObjectiveAnswer | null
}) {
  if (!answered) return null
  const isCorrectOption = correctVal === value
  const isWrongPick = selected === value && !isCorrectOption
  if (isCorrectOption) return <CheckCircle2Icon className="mt-0.5 size-4 shrink-0 text-emerald-600" />
  if (isWrongPick) return <XCircleIcon className="mt-0.5 size-4 shrink-0 text-red-600" />
  return null
}
