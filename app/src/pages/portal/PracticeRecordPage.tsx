// 练习答题卡回看页：从提交记录进入——查看某次交卷的逐题作答。
// 布局模仿练习作答页：桌面 左 sticky 题目导航（题号方块：对=绿/错=红）+ 右逐题回顾；
// 移动端 题目导航收进弹层。路由 /s/:spaceId/practice/:practiceId/record/:submissionId。
import { useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { LayoutGridIcon, Loader2Icon } from 'lucide-react'

import { api } from '@/api'
import type { ObjectiveAnswer, PracticeRecordItem } from '@/api/types'
import { Markdown, preserveLineBreaks } from '@/lib/markdown'
import { OPTION_LABELS } from '@/components/portal/objective'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog'
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
      <RecordSheet sid={sid} pid={pid} data={d} />
    </SpacePageShell>
  )
}

function RecordSheet({ sid, pid, data }: {
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
  const wrongCount = items.filter((i) => !i.correct).length
  const [navOpen, setNavOpen] = useState(false)

  const jumpTo = (problemId: number) => {
    setNavOpen(false)
    document.getElementById(`r-${problemId}`)?.scrollIntoView({ behavior: 'smooth', block: 'center' })
  }

  const nav = (
    <NavCard
      items={items}
      correct={items.length - wrongCount}
      wrong={wrongCount}
      createdAt={data.createdAt}
      onJump={jumpTo}
    />
  )

  return (
    <div className="mx-auto w-full max-w-6xl">
      {/* 页内顶部条：标题 + 汇总 + 移动端导航按钮 */}
      <div className="sticky top-0 z-30 border-b bg-white/95 backdrop-blur shadow-sm">
        <div className="mx-auto flex h-14 w-full max-w-6xl items-center gap-3 px-3">
          <h1 className="flex min-w-0 items-center gap-2 text-base font-bold">
            <span className="truncate">本次答题卡</span>
            <span className="tabular-nums text-xs font-normal text-muted-foreground">#{data.submissionId}</span>
          </h1>
          <div className="flex items-center gap-1.5 text-xs">
            <span className="rounded-md border border-emerald-300 bg-emerald-50 px-1.5 py-0.5 font-semibold text-emerald-700">
              答对 {total - wrongCount}
            </span>
            <span className="rounded-md border border-red-300 bg-red-50 px-1.5 py-0.5 font-semibold text-red-600">
              答错 {wrongCount}
            </span>
          </div>
          <div className="flex-1" />
          <Button variant="outline" size="sm" className="md:hidden" onClick={() => setNavOpen(true)}>
            <LayoutGridIcon className="size-3.5" /> 题目导航
          </Button>
          <Link
            to={`/s/${sid}/practice/${pid}`}
            className="inline-flex items-center rounded-lg border px-3 py-1.5 text-xs text-muted-foreground transition-colors hover:bg-muted"
          >
            返回练习
          </Link>
        </div>
      </div>

      <div className="mx-auto flex w-full max-w-6xl items-start gap-4 px-3 pt-4">
        {/* 左栏：答题卡导航（固定） */}
        <aside className="hidden w-56 shrink-0 self-start md:block">
          <div className="sticky top-[4.25rem] flex max-h-[calc(100dvh-5.5rem)] flex-col gap-3 overflow-y-auto pr-0.5">
            {nav}
          </div>
        </aside>

        {/* 右：逐题回顾 */}
        <div className="min-w-0 flex-1 space-y-3">
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

      {/* 移动端题目导航弹层 */}
      <Dialog open={navOpen} onOpenChange={setNavOpen}>
        <DialogContent className="sm:max-w-sm">
          <DialogHeader>
            <DialogTitle>题目导航</DialogTitle>
          </DialogHeader>
          {nav}
        </DialogContent>
      </Dialog>
    </div>
  )
}

// 左栏导航卡（答题卡网格：对=绿 错=红）
function NavCard({ items, correct, wrong, createdAt, onJump }: {
  items: PracticeRecordItem[]
  correct: number
  wrong: number
  createdAt: string
  onJump: (problemId: number) => void
}) {
  return (
    <div className="rounded-xl border bg-card p-3 shadow-sm">
      <div className="flex items-center justify-between gap-2">
        <span className="text-xs font-bold text-muted-foreground">题目导航</span>
        <span className="text-[10px] tabular-nums text-muted-foreground/80">{formatTime(createdAt)}</span>
      </div>
      <div className="mt-2 flex flex-wrap items-center gap-3 border-b pb-2 text-[11px]">
        <span className="inline-flex items-center gap-1">
          <span className="size-3 rounded border border-emerald-300 bg-emerald-100" />
          <span className="text-muted-foreground">答对 {correct}</span>
        </span>
        <span className="inline-flex items-center gap-1">
          <span className="size-3 rounded border border-red-300 bg-red-100" />
          <span className="text-muted-foreground">答错 {wrong}</span>
        </span>
      </div>
      <div className="mt-2 grid grid-cols-5 gap-1.5">
        {items.map((it) => (
          <button
            key={it.problemId}
            type="button"
            onClick={() => onJump(it.problemId)}
            title={`第 ${it.no} 题 · ${it.correct ? '答对' : '答错'}`}
            className={cn(
              'flex h-8 items-center justify-center rounded-md border text-xs font-semibold tabular-nums transition-colors',
              it.correct
                ? 'border-emerald-300 bg-emerald-100 text-emerald-700 hover:bg-emerald-200'
                : 'border-red-300 bg-red-50 text-red-600 hover:bg-red-100',
            )}
          >
            {it.no}
          </button>
        ))}
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
    <div id={`r-${item.problemId}`} className="scroll-mt-24 rounded-xl border bg-card p-4 shadow-sm">
      <div className="flex items-center gap-2">
        <span
          className={cn(
            'flex size-5 shrink-0 items-center justify-center rounded-full text-[11px] font-bold text-white',
            item.correct ? 'bg-emerald-500' : 'bg-red-500',
          )}
        >
          {item.correct ? '✓' : '✗'}
        </span>
        <span className="text-sm font-bold tabular-nums">{item.no}.</span>
        <span className="min-w-0 flex-1 truncate text-sm text-muted-foreground">
          {item.title || `题目 #${item.problemId}`}
        </span>
        <span className={cn('shrink-0 text-xs font-medium', item.correct ? 'text-emerald-600' : 'text-red-500')}>
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
                      isSel && !isCorrect && 'bg-red-50 text-red-600',
                      isCorrect && !isSel && 'text-emerald-700',
                      !isSel && !isCorrect && 'opacity-60',
                    )}
                  >
                    <span className="mt-0.5 w-4 shrink-0 font-semibold tabular-nums">{OPTION_LABELS[i] ?? i + 1}.</span>
                    <span className="min-w-0 flex-1">
                      <Markdown text={preserveLineBreaks(opt)} className="markdown-body text-sm md-clean" />
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
                      isSel && !isCorrect && 'bg-red-50 text-red-600',
                      isCorrect && !isSel && 'text-emerald-700',
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
        <p className="mt-2 rounded-lg border border-red-200 bg-red-50 px-3 py-1.5 text-xs text-red-600">
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
