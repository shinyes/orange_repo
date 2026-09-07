// 练习答题卡回看页：从提交记录进入——查看某次交卷的逐题作答（题号网格 + 逐题回顾）。
// 路由 /s/:spaceId/practice/:practiceId/record/:submissionId（SpacePageShell 壳内全屏）。
import { useMemo } from 'react'
import { Link, useParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { ClipboardListIcon, Loader2Icon } from 'lucide-react'

import { api } from '@/api'
import type { ObjectiveAnswer, PracticeRecordItem } from '@/api/types'
import { Markdown, preserveLineBreaks } from '@/lib/markdown'
import { OPTION_LABELS } from '@/components/portal/objective'
import { cn } from '@/lib/utils'
import { useSpaceById } from '@/pages/portal/portal-context'
import { PageContainer, SpacePageShell } from '@/pages/portal/SpacePageShell'

export function PracticeRecordPage() {
  const { spaceId, practiceId, submissionId } = useParams()
  const sid = Number(spaceId)
  const pid = Number(practiceId)
  const rid = Number(submissionId)
  const space = useSpaceById(sid)
  const q = useQuery({
    queryKey: ['practice-record', sid, pid, rid],
    queryFn: () => api.portalPracticeSubmissionDetail(sid, pid, rid),
  })

  if (space === 'loading' || q.isLoading) {
    return (
      <SpacePageShell spaceId={sid} backTo={`/s/${sid}/practice/${pid}`} backLabel="返回练习">
        <div className="flex items-center justify-center gap-2 py-20 text-sm text-muted-foreground">
          <Loader2Icon className="size-4 animate-spin" /> 加载答题卡…
        </div>
      </SpacePageShell>
    )
  }
  if (space === null || q.isError || !q.data) {
    return (
      <SpacePageShell spaceId={sid} backTo={`/s/${sid}/practice/${pid}`} backLabel="返回练习">
        <PageContainer>
          <div className="rounded-xl border border-dashed p-12 text-center text-sm text-muted-foreground">
            答题卡不存在或无权查看
          </div>
        </PageContainer>
      </SpacePageShell>
    )
  }
  const d = q.data
  return (
    <SpacePageShell spaceId={sid} backTo={`/s/${sid}/practice/${pid}`} backLabel="返回练习">
      <PracticeRecordView sid={sid} pid={pid} data={d} />
    </SpacePageShell>
  )
}

function PracticeRecordView({ sid, pid, data }: {
  sid: number
  pid: number
  data: {
    submissionId: number
    practiceId: number
    createdAt: string
    objectiveCorrect: number
    items: PracticeRecordItem[]
  }
}) {
  const items = data.items
  const total = items.length
  const correct = items.filter((i) => i.correct).length
  const wrong = total - correct

  // 题号分组（单/判 → 答对绿/错橙；编程题本快照不含——练习条目的编程题单独展示略）
  const grid = useMemo(() => items, [items])

  return (
    <div className="mx-auto w-full max-w-4xl px-3 py-5">
      {/* 汇总卡 */}
      <div className="rounded-2xl border bg-card p-5 shadow-sm">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <div className="flex items-center gap-2">
            <ClipboardListIcon className="size-5 text-orange-500" />
            <span className="text-base font-bold">本次答题卡</span>
            <span className="text-xs text-muted-foreground">#{data.submissionId}</span>
          </div>
          <Link
            to={`/s/${sid}/practice/${pid}`}
            className="rounded-lg border px-3 py-1.5 text-xs transition-colors hover:bg-muted"
          >
            返回继续作答
          </Link>
        </div>
        <div className="mt-2 flex flex-wrap items-center gap-1.5 text-xs text-muted-foreground">
          <span className="tabular-nums">{formatTime(data.createdAt)}</span>
          <span className="mx-1 text-muted-foreground/50">·</span>
          <span className="font-semibold text-emerald-600">答对 {correct}/{total}</span>
          {wrong > 0 && <span className="font-semibold text-orange-600">答错 {wrong}</span>}
        </div>
        {/* 题号网格（答题卡） */}
        {grid.length > 0 && (
          <div className="mt-4 flex flex-wrap gap-1.5">
            {grid.map((it) => (
              <a
                key={it.problemId}
                href={`#r-${it.problemId}`}
                className={cn(
                  'flex size-8 items-center justify-center rounded-md border text-xs font-semibold tabular-nums',
                  it.correct
                    ? 'border-emerald-300 bg-emerald-100 text-emerald-700'
                    : 'border-orange-300 bg-orange-50 text-orange-700',
                )}
              >
                {it.no}
              </a>
            ))}
          </div>
        )}
      </div>

      {/* 逐题回顾 */}
      <div className="mt-4 space-y-3">
        {items.map((it) => (
          <ReviewCard key={it.problemId} item={it} />
        ))}
        {items.length === 0 && (
          <div className="rounded-xl border border-dashed p-12 text-center text-sm text-muted-foreground">
            该次交卷无客观题作答记录
          </div>
        )}
      </div>
    </div>
  )
}

function ReviewCard({ item }: { item: PracticeRecordItem }) {
  const contentQ = useQuery({
    queryKey: ['oj-problem', item.problemId],
    queryFn: () => api.ojProblem(item.problemId),
  })
  const wrong = !item.correct
  const selected = item.answer as ObjectiveAnswer | null | undefined
  const correctVal = item.correctAnswer as ObjectiveAnswer | undefined

  return (
    <div id={`r-${item.problemId}`} className="scroll-mt-20 rounded-xl border bg-card p-4">
      <div className="flex items-center gap-2">
        <span className={cn('flex size-6 shrink-0 items-center justify-center rounded-full text-[11px] font-bold text-white',
          item.correct ? 'bg-emerald-500' : 'bg-orange-500')}>
          {item.correct ? '✓' : '✗'}
        </span>
        <span className="text-sm font-bold tabular-nums">{item.no}.</span>
        <span className="min-w-0 flex-1 truncate text-sm text-muted-foreground">
          {item.title || `题目 #${item.problemId}`}
        </span>
        <span className={cn('shrink-0 text-xs font-medium', item.correct ? 'text-emerald-600' : 'text-orange-600')}>
          {item.correct ? '回答正确' : '回答错误'}
        </span>
      </div>
      {contentQ.isLoading && <p className="py-4 text-center text-xs text-muted-foreground">题目加载中…</p>}
      {contentQ.data && (
        <div className="mt-2">
          <Markdown
            text={preserveLineBreaks(contentQ.data.statementMd || '（暂无题面）')}
            className="markdown-body text-[15px] md-line"
          />
          <div className="mt-2 space-y-1">
            {contentQ.data.type === 'single_choice'
              ? (contentQ.data.bodyJson.options ?? []).map((opt, i) => {
                const isSel = selected === i
                const isCorrect = correctVal === i
                return (
                  <div
                    key={i}
                    className={cn(
                      'flex items-start gap-2 rounded-md px-2 py-1 text-sm',
                      isSel && !isCorrect && 'bg-red-50 text-red-700',
                      isCorrect && 'text-emerald-700',
                      !isSel && !isCorrect && 'opacity-60',
                    )}
                  >
                    <span className="mt-0.5 w-4 shrink-0 font-semibold tabular-nums">{OPTION_LABELS[i] ?? i + 1}.</span>
                    <span className="min-w-0 flex-1 md-clean">
                      <Markdown text={preserveLineBreaks(opt)} className="markdown-body text-sm" />
                    </span>
                    {isSel && <span className="shrink-0 text-xs">你的选择</span>}
                    {isCorrect && !isSel && <span className="shrink-0 text-xs">正确项</span>}
                  </div>
                )
              })
              : ([true, false] as const).map((v) => {
                const isSel = selected === v
                const isCorrect = correctVal === v
                return (
                  <div
                    key={String(v)}
                    className={cn(
                      'flex items-center gap-2 rounded-md px-2 py-1 text-sm',
                      isSel && !isCorrect && 'bg-red-50 text-red-700',
                      isCorrect && 'text-emerald-700',
                      !isSel && !isCorrect && 'opacity-60',
                    )}
                  >
                    <span className="min-w-0 flex-1 font-medium">{v ? '正确' : '错误'}</span>
                    {isSel && <span className="text-xs">你的选择</span>}
                    {isCorrect && !isSel && <span className="text-xs">正确项</span>}
                  </div>
                )
              })}
          </div>
        </div>
      )}
      {wrong && (
        <p className="mt-2 rounded-lg border border-orange-200 bg-orange-50 px-3 py-1.5 text-xs text-orange-700">
          正确答案：{answerText(item)}
        </p>
      )}
    </div>
  )
}

function answerText(item: PracticeRecordItem): string {
  const ca = item.correctAnswer
  if (item.type === 'true_false') {
    return ca === true ? '正确' : '错误'
  }
  const L = ['A', 'B', 'C', 'D', 'E', 'F', 'G', 'H']
  if (typeof ca === 'number') return `选项 ${L[ca] ?? ca}`
  return '—'
}

function formatTime(iso: string): string {
  try {
    return new Date(iso).toLocaleString('zh-CN', { hour12: false })
  } catch {
    return iso
  }
}
