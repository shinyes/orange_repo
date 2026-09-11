// 练习答题卡回看页：从提交记录进入——查看某次交卷的逐题作答。
// 布局模仿练习作答页：桌面 左 sticky 题目导航（题号方块：对=绿/错=红）+ 右逐题回顾；
// 移动端 题目导航收进弹层。路由 /s/:spaceId/practice/:practiceId/record/:submissionId。
import { useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { Code2Icon, LayoutGridIcon, Loader2Icon } from 'lucide-react'

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
  // 客观题（作答+未作答）；编程题仅导航占位
  const objItems = items.filter((i) => i.type !== 'programming')
  const answeredItems = objItems.filter((i) => i.answered)
  const correctCount = answeredItems.filter((i) => i.correct).length
  const wrongCount = answeredItems.length - correctCount
  const missingCount = objItems.length - answeredItems.length
  const [navOpen, setNavOpen] = useState(false)
  const navigate = useNavigate()

  const jumpTo = (problemId: number) => {
    setNavOpen(false)
    document.getElementById(`r-${problemId}`)?.scrollIntoView({ behavior: 'smooth', block: 'center' })
  }

  // 编程题：进入做题页回顾（只读，不可作答；带练习上下文隔离提交历史）
  const openProgReview = (problemId: number) => {
    setNavOpen(false)
    navigate(`/problem/${problemId}?review=1&practiceId=${pid}&back=${encodeURIComponent(`/s/${sid}/practice/${pid}/record/${data.submissionId}`)}`)
  }

  const nav = (
    <NavCard
      items={items}
      correct={correctCount}
      wrong={wrongCount}
      missing={missingCount}
      createdAt={data.createdAt}
      onJump={jumpTo}
      onOpenProg={openProgReview}
    />
  )

  return (
    <div className="flex h-full min-h-0 flex-col">
      {/* 顶部固定行（页内标题条；不再随滚动） */}
      <div className="shrink-0 border-b bg-background shadow-sm">
        <div className="mx-auto flex h-11 w-full max-w-6xl items-center gap-3 px-3">
          <h1 className="flex min-w-0 items-center gap-2 text-base font-bold">
            <span className="truncate">本次答题卡</span>
            <span className="tabular-nums text-xs font-normal text-muted-foreground">#{data.submissionId}</span>
          </h1>
          <div className="flex items-center gap-1.5 text-xs">
            <span className="rounded-md border border-emerald-300 bg-emerald-50 px-1.5 py-0.5 font-semibold text-emerald-700">
              答对 {correctCount}
            </span>
            <span className="rounded-md border border-red-300 bg-red-50 px-1.5 py-0.5 font-semibold text-red-600">
              答错 {wrongCount}
            </span>
            {missingCount > 0 && (
              <span className="rounded-md border border-border bg-muted px-1.5 py-0.5 font-semibold text-muted-foreground">
                未答 {missingCount}
              </span>
            )}
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

      {/* 内容行：左导航固定，右回顾为唯一滚动区 */}
      <div className="mx-auto flex min-h-0 w-full max-w-6xl flex-1">
        {/* 左栏：答题卡导航（自身随内容滚动，不随右栏滚动） */}
        <aside className="hidden w-56 shrink-0 overflow-y-auto p-2.5 md:block">
          {nav}
        </aside>

        {/* 右栏：逐题回顾（唯一内容滚动区；显示练习全部题目） */}
        <div className="min-h-0 flex-1 overflow-y-auto">
          <div className="space-y-3 p-3">
            {items.map((it) =>
              it.type === 'programming' ? (
                <ProgrammingReviewCard key={it.problemId} item={it} practiceId={pid} asOf={data?.createdAt} onOpen={() => openProgReview(it.problemId)} />
              ) : (
                <ReviewCard key={it.problemId} item={it} />
              ),
            )}
            {items.length === 0 && (
              <div className="rounded-xl border border-dashed p-12 text-center text-sm text-muted-foreground">
                本练习无题目
              </div>
            )}
          </div>
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

// 左栏导航卡（答题卡网格：对=绿 错=红 未答=白 编程=虚线灰）
function NavCard({ items, correct, wrong, missing, createdAt, onJump, onOpenProg }: {
  items: PracticeRecordItem[]
  correct: number
  wrong: number
  missing: number
  createdAt: string
  onJump: (problemId: number) => void
  onOpenProg: (problemId: number) => void
}) {
  return (
    <div className="rounded-xl border bg-card p-3 shadow-sm">
      <div className="flex items-center justify-between gap-2">
        <span className="text-xs font-bold text-muted-foreground">题目导航</span>
        <span className="text-[10px] tabular-nums text-muted-foreground/80">{formatTime(createdAt)}</span>
      </div>
      <div className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 border-b pb-2 text-[11px]">
        <span className="inline-flex items-center gap-1">
          <span className="size-3 rounded border border-emerald-300 bg-emerald-100" />
          <span className="text-muted-foreground">答对 {correct}</span>
        </span>
        <span className="inline-flex items-center gap-1">
          <span className="size-3 rounded border border-red-300 bg-red-100" />
          <span className="text-muted-foreground">答错 {wrong}</span>
        </span>
        {missing > 0 && (
          <span className="inline-flex items-center gap-1">
            <span className="size-3 rounded border border-border bg-white" />
            <span className="text-muted-foreground">未答 {missing}</span>
          </span>
        )}
      </div>
      <div className="mt-2 grid grid-cols-5 gap-1.5">
        {items.map((it) => {
          const isProg = it.type === 'programming'
          const answered = !!it.answered
          return (
            <button
              key={it.problemId}
              type="button"
              onClick={() => (isProg ? onOpenProg(it.problemId) : onJump(it.problemId))}
              title={`第 ${it.no} 题${isProg ? '（编程，点击进入回顾）' : answered ? (it.correct ? ' · 答对' : ' · 答错') : ' · 未作答'}`}
              className={cn(
                'flex h-8 items-center justify-center rounded-md border text-xs font-semibold tabular-nums transition-colors',
                isProg
                  ? 'border-dashed border-border bg-card text-muted-foreground/60 hover:bg-muted/60'
                  : answered
                    ? it.correct
                      ? 'border-emerald-300 bg-emerald-100 text-emerald-700 hover:bg-emerald-200'
                      : 'border-red-300 bg-red-50 text-red-600 hover:bg-red-100'
                    : 'border-border bg-white text-muted-foreground hover:border-primary/50 hover:text-foreground',
              )}
            >
              {it.no}
            </button>
          )
        })}
      </div>
    </div>
  )
}

// 编程题回顾卡：状态圆（该次交卷时点为止的账号提交记录：AC=通过绿 / 有提交未AC=未通过红 / 无=未作答灰）
// + 题号，与客观题一致；点击进入做题页只读回顾。asOf 为该次交卷时间——之后的新提交不影响本卡状态。
function ProgrammingReviewCard({ item, practiceId, asOf, onOpen }: {
  item: PracticeRecordItem
  practiceId: number
  asOf?: string
  onOpen: () => void
}) {
  const subsQ = useQuery({
    queryKey: ['oj-submissions', 'p', item.problemId, practiceId],
    queryFn: () => api.ojSubmissions(item.problemId, undefined, practiceId),
  })
  const subs = subsQ.data?.submissions ?? []
  // 仅正式提交（submit）计入作答状态：run/test（运行/自测/评测基准）不算"做过"；
  // 截至该次交卷时点（RFC3339 数值比较，避免字符串格式/小数秒失真）
  const asOfMs = asOf ? new Date(asOf).getTime() : NaN
  const submits = subs.filter(
    (s) => s.submitType === 'submit' && (!asOf || (Number.isFinite(asOfMs) && new Date(s.createdAt ?? '').getTime() <= asOfMs)),
  )
  const passed = submits.some((s) => s.verdict === 'AC' || s.verdict === 'OK')
  const attempted = !passed && submits.some((s) => s.status === 'done')
  const stateMeta = passed
    ? { cls: 'bg-emerald-500', mark: '✓', label: '已通过' }
    : attempted
      ? { cls: 'bg-red-500', mark: '✗', label: '未通过' }
      : { cls: 'bg-muted-foreground/50', mark: '–', label: '未作答' }
  return (
    <div id={`r-${item.problemId}`} className="scroll-mt-24 rounded-xl border bg-card p-4 shadow-sm">
      <div className="flex items-center gap-2">
        <span
          title={stateMeta.label}
          className={cn(
            'flex size-5 shrink-0 items-center justify-center rounded-full text-[11px] font-bold text-white',
            stateMeta.cls,
          )}
        >
          {stateMeta.mark}
        </span>
        <span className="text-sm font-bold tabular-nums">{item.no}.</span>
        <span className="min-w-0 flex-1 truncate text-sm font-semibold">{item.title || `题目 #${item.problemId}`}</span>
        <button
          type="button"
          onClick={onOpen}
          className="inline-flex shrink-0 items-center gap-1.5 rounded-lg border border-orange-300 bg-orange-50 px-3 py-1.5 text-xs font-semibold text-orange-600 transition-colors hover:bg-orange-100"
        >
          <Code2Icon className="size-3.5" /> 查看题目与记录
        </button>
      </div>
    </div>
  )
}

function ReviewCard({ item }: { item: PracticeRecordItem }) {
  const contentQ = useQuery({
    queryKey: ['oj-problem', item.problemId],
    queryFn: () => api.ojProblem(item.problemId),
  })
  const answered = !!item.answered
  const wrong = answered && !item.correct
  const selected = item.answer as ObjectiveAnswer | null | undefined
  // 答对：用户所选即正确项（绿色显示）；答错/未答：用服务端下发的正确项
  const correctVal: ObjectiveAnswer | undefined = wrong || !answered
    ? (item.correctAnswer as ObjectiveAnswer | undefined)
    : (selected as ObjectiveAnswer | undefined)

  return (
    <div id={`r-${item.problemId}`} className="scroll-mt-24 rounded-xl border bg-card p-4 shadow-sm">
      <div className="flex items-center gap-2">
        <span
          className={cn(
            'flex size-5 shrink-0 items-center justify-center rounded-full text-[11px] font-bold text-white',
            !answered ? 'bg-muted-foreground/50' : item.correct ? 'bg-emerald-500' : 'bg-red-500',
          )}
        >
          {!answered ? '–' : item.correct ? '✓' : '✗'}
        </span>
        <span className="text-sm font-bold tabular-nums">{item.no}.</span>
        <span className="min-w-0 flex-1 truncate text-sm text-muted-foreground">
          {item.title || `题目 #${item.problemId}`}
        </span>
        <span className={cn('shrink-0 text-xs font-medium', !answered ? 'text-muted-foreground' : item.correct ? 'text-emerald-600' : 'text-red-500')}>
          {!answered ? '未作答' : item.correct ? '回答正确' : '回答错误'}
        </span>
      </div>
      {contentQ.isLoading && <p className="py-4 text-center text-xs text-muted-foreground">题目加载中…</p>}
      {contentQ.isError && (
        <p className="py-4 text-center text-xs text-muted-foreground">
          题目内容不可见（{contentQ.error instanceof Error ? contentQ.error.message : '加载失败'}）
        </p>
      )}
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
                      isCorrect && 'bg-emerald-50 text-emerald-700',
                      isSel && !isCorrect && 'bg-red-50 text-red-600',
                      !isSel && !isCorrect && 'opacity-60',
                    )}
                  >
                    <span className="mt-0.5 w-4 shrink-0 font-semibold tabular-nums">{OPTION_LABELS[i] ?? i + 1}.</span>
                    <span className="min-w-0 flex-1">
                      <Markdown text={preserveLineBreaks(opt)} className="markdown-body text-sm md-clean" />
                    </span>
                    {isCorrect && isSel && <span className="shrink-0 text-xs font-medium">✓ 你的选择</span>}
                    {isCorrect && !isSel && <span className="shrink-0 text-xs">正确项</span>}
                    {isSel && !isCorrect && <span className="shrink-0 text-xs">你的选择</span>}
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
                      isCorrect && 'bg-emerald-50 text-emerald-700',
                      isSel && !isCorrect && 'bg-red-50 text-red-600',
                      !isSel && !isCorrect && 'opacity-60',
                    )}
                  >
                    <span className="min-w-0 flex-1 font-medium">{v ? '正确' : '错误'}</span>
                    {isCorrect && isSel && <span className="text-xs font-medium">✓ 你的选择</span>}
                    {isCorrect && !isSel && <span className="text-xs">正确项</span>}
                    {isSel && !isCorrect && <span className="text-xs">你的选择</span>}
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
