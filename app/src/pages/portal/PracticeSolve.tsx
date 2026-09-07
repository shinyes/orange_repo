import { useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { ClipboardListIcon, Code2Icon, LayoutGridIcon, Loader2Icon, SendIcon } from 'lucide-react'
import { toast } from 'sonner'

import { api } from '@/api'
import type {
  ObjectiveAnswer, PracticeDetail, PracticeResultItem,
} from '@/api/types'
import { ObjectiveQuestion } from '@/components/portal/objective'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription,
  AlertDialogFooter, AlertDialogHeader, AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import {
  Dialog, DialogContent, DialogHeader, DialogTitle,
} from '@/components/ui/dialog'
import { useSpaceById } from '@/pages/portal/portal-context'
import { SpacePageShell } from '@/pages/portal/SpacePageShell'
import { cn } from '@/lib/utils'

type PracticeItem = PracticeDetail['items'][number]
type Verdict = 'idle' | 'answered' | 'correct' | 'wrong' | 'missing'

// 练习整卷：列出全部题目（客观题题面+选项就地可答可改；编程题跳现做题页），
// 交卷前可任意改选，交卷（POST submit）后展示逐题对错 + 得分 + 历史。
export function PracticeSolve() {
  const { spaceId, practiceId } = useParams()
  const sid = Number(spaceId)
  const pid = Number(practiceId)
  const space = useSpaceById(sid)
  const q = useQuery({
    queryKey: ['portal-practice', sid, pid],
    queryFn: () => api.portalPractice(sid, pid),
  })
  const data = q.data

  if (space === 'loading' || q.isLoading) return <Center text="加载练习中…" />
  if (space === null || q.isError || !data) return <Center text="练习不存在或无权访问" />

  return (
    <SpacePageShell spaceId={sid} backTo={`/s/${sid}/practice`} backLabel="返回练习列表">
      <div className="min-h-full bg-[#eef2f7]">
        <PracticePaper key={pid} sid={sid} pid={pid} data={data} />
      </div>
    </SpacePageShell>
  )
}

const CN_NUM = ['一', '二', '三', '四', '五', '六', '七', '八']

function PracticePaper({ sid, pid, data }: {
  sid: number
  pid: number
  data: PracticeDetail
}) {
  const { practice, items: rawItems } = data
  const items = rawItems ?? []
  const objectiveItems = items.filter((i) => i.problemType === 'single_choice' || i.problemType === 'true_false')
  const qc = useQueryClient()

  // 每客观题已选题（单选=number，判断=boolean）
  const [answers, setAnswers] = useState<Record<number, ObjectiveAnswer>>({})
  const [confirmOpen, setConfirmOpen] = useState(false)
  const [navOpen, setNavOpen] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [result, setResult] = useState<{ submissionId: number; results: PracticeResultItem[]; objectiveCorrect: number; objectiveTotal: number } | null>(null)

  function toggleAnswer(problemId: number, a: ObjectiveAnswer) {
    setAnswers((prev) => {
      const next = { ...prev }
      if (next[problemId] === a) delete next[problemId]
      else next[problemId] = a
      return next
    })
  }

  async function submitPaper() {
    setSubmitting(true)
    try {
      const payload = objectiveItems.map((it) => ({
        problemId: it.problemId,
        answer: answers[it.problemId],
        uuid: it.problemUuid,
      })).filter((a) => a.answer !== undefined)
      const r = await api.portalPracticeSubmit(sid, pid, payload)
      setResult(r)
      void qc.invalidateQueries({ queryKey: ['portal-practice-submissions', sid, pid] })
      toast.success(`交卷成功：答对 ${r.objectiveCorrect}/${r.objectiveTotal}`)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '交卷失败')
    } finally {
      setSubmitting(false)
      setConfirmOpen(false)
    }
  }

  const answeredCount = Object.keys(answers).length

  // 按题型分组（固定顺序：单选→判断→编程，只渲染非空组），跨组连续编号
  const grouped = ([
    { key: 'single_choice' as const, title: '单选题', items: items.filter((i) => i.problemType === 'single_choice') },
    { key: 'true_false' as const, title: '判断题', items: items.filter((i) => i.problemType === 'true_false') },
    { key: 'programming' as const, title: '编程题', items: items.filter((i) => i.problemType === 'programming') },
  ]).filter((g) => g.items.length > 0)
  let seq = 1
  const noByPid = new Map<number, number>()
  for (const g of grouped) {
    for (const it of g.items) noByPid.set(it.problemId, seq++)
  }
  const sectionTitles = grouped.map((g, gi) => `${CN_NUM[gi] ?? ''}、${g.title}`)

  // 交卷结果 → 每题判定
  const resultMap = new Map((result?.results ?? []).map((r) => [r.problemId, r]))
  function verdictOf(problemId: number): Verdict {
    if (!result) return answers[problemId] !== undefined ? 'answered' : 'idle'
    const r = resultMap.get(problemId)
    if (!r) return 'missing'
    return r.correct ? 'correct' : 'wrong'
  }
  function resultItemOf(problemId: number): PracticeResultItem | undefined {
    return resultMap.get(problemId)
  }

  function jumpTo(problemId: number) {
    setNavOpen(false)
    document.getElementById(`pq-${problemId}`)?.scrollIntoView({ behavior: 'smooth', block: 'center' })
  }

  const navSections = grouped.map((g, gi) => ({
    title: sectionTitles[gi],
    items: g.items.map((it) => ({
      problemId: it.problemId,
      no: noByPid.get(it.problemId) ?? 0,
      programming: g.key === 'programming',
    })),
  }))

  return (
    <div className="mx-auto w-full max-w-6xl px-3 py-4">
      <div className="flex items-start gap-4">
        {/* 左：题号导航（桌面） */}
        <aside className="hidden w-56 shrink-0 md:block">
          <div className="sticky top-4 max-h-[calc(100vh-140px)] overflow-y-auto rounded-2xl border bg-card p-3">
            <PaperNav
              sections={navSections}
              verdictOf={verdictOf}
              answeredCount={answeredCount}
              objectiveTotal={objectiveItems.length}
              onJump={jumpTo}
            />
          </div>
        </aside>

        {/* 右：卷面 */}
        <div className="min-w-0 flex-1">
          {/* 头部卡：标题 + 交卷 */}
          <div className="rounded-2xl border bg-card p-4">
            <div className="flex items-center justify-between gap-3">
              <h1 className="flex min-w-0 items-center gap-2 text-base font-bold">
                <ClipboardListIcon className="size-4 shrink-0 text-primary" />
                <span className="min-w-0 truncate">{practice.title}</span>
                <Badge variant="secondary" className="shrink-0">{items.length} 题</Badge>
              </h1>
              <div className="flex shrink-0 items-center gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  className="md:hidden"
                  onClick={() => setNavOpen(true)}
                >
                  <LayoutGridIcon className="size-3.5" />
                  题目导航
                </Button>
                <Button
                  size="sm"
                  className="min-w-20"
                  disabled={submitting || answeredCount === 0 || !!result}
                  onClick={() => setConfirmOpen(true)}
                >
                  {submitting ? <Loader2Icon className="size-4 animate-spin" /> : <SendIcon className="size-4" />}
                  {submitting ? '提交中' : '交卷'}
                </Button>
              </div>
            </div>
            {practice.description && (
              <p className="mt-1.5 truncate text-xs text-muted-foreground">{practice.description}</p>
            )}
            <div className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
              <span>客观题已答 <span className="font-semibold text-foreground">{answeredCount}/{objectiveItems.length}</span></span>
              {objectiveItems.length > 0 && <span>{objectiveItems.length} 道客观题</span>}
              {grouped.some((g) => g.key === 'programming') && <span>{items.length - objectiveItems.length} 道编程题（做题页提交）</span>}
              {!result && <span className="text-primary">交卷后统一评分，可重复交卷</span>}
            </div>
          </div>

          {/* 交卷结果（横向通栏） */}
          {result && (
            <ResultPanel
              result={result}
              items={objectiveItems}
              onDismiss={() => setResult(null)}
            />
          )}

          {/* 题目分组节卡 */}
          <div className="mt-3 space-y-3">
            {grouped.map((g, gi) => {
              const isProgramming = g.key === 'programming'
              const children = isProgramming
                ? g.items.map((it) => (
                  <ProgrammingBlock
                    key={it.problemId}
                    item={it}
                    no={noByPid.get(it.problemId) ?? 0}
                    sid={sid}
                    pid={pid}
                  />
                ))
                : g.items.map((it) => (
                  <ObjectiveBlock
                    key={it.problemId}
                    item={it}
                    no={noByPid.get(it.problemId) ?? 0}
                    verdict={verdictOf(it.problemId)}
                    resultItem={resultItemOf(it.problemId)}
                    selected={answers[it.problemId] ?? null}
                    onToggle={(a) => toggleAnswer(it.problemId, a)}
                  />
                ))
              return (
                <section key={g.key} className="overflow-hidden rounded-2xl border bg-card">
                  <div className="border-b border-border/70 px-4 pt-3 pb-2 text-xs font-bold text-muted-foreground">
                    {sectionTitles[gi]}
                    <span className="ml-2 font-normal opacity-70">{g.items.length} 题</span>
                  </div>
                  <div className="divide-y divide-border">{children}</div>
                </section>
              )
            })}
            {items.length === 0 && (
              <div className="rounded-xl border border-dashed p-12 text-center text-sm text-muted-foreground">
                本练习暂无题目
              </div>
            )}
          </div>

          <HistoryList sid={sid} pid={pid} />
        </div>
      </div>

      {/* 交卷确认 */}
      <AlertDialog open={confirmOpen} onOpenChange={setConfirmOpen}>
        <AlertDialogContent size="default">
          <AlertDialogHeader>
            <AlertDialogTitle>确认交卷？</AlertDialogTitle>
            <AlertDialogDescription>
              已作答客观题 {answeredCount}/{objectiveItems.length} 题（{objectiveItems.length - answeredCount > 0 ? `尚有 ${objectiveItems.length - answeredCount} 题未作答；` : ''}{grouped.some((g) => g.key === 'programming') ? '编程题请到做题页提交，不计入本次卷面；' : ''}交卷后立即评分并记录，可再次交卷重做）。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel onClick={() => setConfirmOpen(false)}>再检查一下</AlertDialogCancel>
            <AlertDialogAction onClick={() => void submitPaper()}>确认交卷</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* 移动端：题目导航抽屉 */}
      <Dialog open={navOpen} onOpenChange={setNavOpen}>
        <DialogContent className="sm:max-w-sm">
          <DialogHeader>
            <DialogTitle>题目导航</DialogTitle>
          </DialogHeader>
          <PaperNav
            sections={navSections}
            verdictOf={verdictOf}
            answeredCount={answeredCount}
            objectiveTotal={objectiveItems.length}
            onJump={jumpTo}
          />
        </DialogContent>
      </Dialog>
    </div>
  )
}

// ---------- 题号导航 ----------

function PaperNav({ sections, verdictOf, answeredCount, objectiveTotal, onJump }: {
  sections: { title: string; items: { problemId: number; no: number; programming: boolean }[] }[]
  verdictOf: (problemId: number) => Verdict
  answeredCount: number
  objectiveTotal: number
  onJump: (problemId: number) => void
}) {
  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between gap-2">
        <span className="text-xs font-bold text-muted-foreground">题目导航</span>
        <span className="text-[11px] text-muted-foreground">已答 {answeredCount}/{objectiveTotal}</span>
      </div>
      {sections.map((sec) => (
        <div key={sec.title}>
          <div className="mb-1.5 text-[11px] font-medium text-muted-foreground/80">{sec.title}</div>
          <div className="grid grid-cols-5 gap-1.5">
            {sec.items.map((it) => {
              const v = verdictOf(it.problemId)
              return (
                <button
                  key={it.problemId}
                  type="button"
                  title={it.programming ? `第 ${it.no} 题（编程，前往做题页）` : `第 ${it.no} 题`}
                  onClick={() => onJump(it.problemId)}
                  className={cn(
                    'flex h-8 items-center justify-center rounded-md border text-xs font-semibold tabular-nums transition-colors',
                    it.programming
                      ? 'border-dashed border-border bg-card text-muted-foreground/60 hover:bg-muted/60'
                      : v === 'correct'
                        ? 'border-emerald-300 bg-emerald-100 text-emerald-700'
                        : v === 'wrong' || v === 'missing'
                          ? 'border-orange-400 bg-orange-50 text-orange-700'
                          : v === 'answered'
                            ? 'border-sky-300 bg-sky-100 text-blue-700'
                            : 'border-border bg-white text-muted-foreground hover:border-primary/50 hover:text-foreground',
                  )}
                >
                  {it.no}
                </button>
              )
            })}
          </div>
        </div>
      ))}
    </div>
  )
}

// ---------- 客观题块（就地作答；交卷后展示对错） ----------

function ObjectiveBlock({ item, no, verdict, resultItem, selected, onToggle }: {
  item: PracticeItem
  no: number
  verdict: Verdict
  resultItem?: PracticeResultItem
  selected: ObjectiveAnswer | null
  onToggle: (a: ObjectiveAnswer) => void
}) {
  const contentQ = useQuery({
    queryKey: ['oj-problem', item.problemId],
    queryFn: () => api.ojProblem(item.problemId),
  })
  return (
    <div id={`pq-${item.problemId}`} className="scroll-mt-24 p-4">
      <div className="flex items-start gap-2">
        <span className="mt-0.5 min-w-[1.6rem] text-right text-sm font-bold tabular-nums text-foreground">{no}.</span>
        <div className="min-w-0 flex-1 space-y-2">
          <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
            <span className="text-sm font-semibold">{item.problemTitle || `题目 #${item.problemId}`}</span>
            <Badge variant="secondary" className="px-1.5 py-0 text-[10px] font-medium">
              {item.problemType === 'single_choice' ? '单选' : '判断'}
            </Badge>
            {verdict !== 'idle' && verdict !== 'answered' && <VerdictChip verdict={verdict} />}
          </div>
          {verdict === 'wrong' && resultItem && (
            <p className="text-xs text-orange-700">正确项：{correctAnswerText(resultItem)}</p>
          )}
          {contentQ.isLoading && <p className="py-4 text-center text-xs text-muted-foreground">题目加载中…</p>}
          {contentQ.isError && (
            <p className="py-4 text-center text-xs text-muted-foreground">
              题目内容不可见（{contentQ.error instanceof Error ? contentQ.error.message : '加载失败'}）
            </p>
          )}
          {contentQ.data && (
            <ObjectiveQuestion
              problem={{
                type: contentQ.data.type as 'single_choice' | 'true_false',
                statementMd: contentQ.data.statementMd,
                bodyJson: contentQ.data.bodyJson,
              }}
              selected={selected}
              onSelect={(a) => onToggle(a)}
            />
          )}
        </div>
      </div>
    </div>
  )
}

function VerdictChip({ verdict }: { verdict: Extract<Verdict, 'correct' | 'wrong' | 'missing'> }) {
  return (
    <span
      className={cn(
        'inline-flex items-center rounded-md border px-1.5 py-0.5 text-[11px] font-semibold',
        verdict === 'correct' ? 'border-emerald-300 bg-emerald-50 text-emerald-700'
          : 'border-orange-300 bg-orange-50 text-orange-700',
      )}
    >
      {verdict === 'correct' ? '正确' : verdict === 'wrong' ? '回答错误' : '未作答'}
    </span>
  )
}

// ---------- 编程题块（跳现做题页） ----------

function ProgrammingBlock({ item, no, sid, pid }: {
  item: PracticeItem
  no: number
  sid: number
  pid: number
}) {
  return (
    <div id={`pq-${item.problemId}`} className="scroll-mt-24 p-4">
      <div className="flex items-start gap-2">
        <span className="mt-0.5 min-w-[1.6rem] text-right text-sm font-bold tabular-nums text-foreground">{no}.</span>
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
            <span className="text-sm font-semibold">{item.problemTitle || `题目 #${item.problemId}`}</span>
            <Badge variant="secondary" className="px-1.5 py-0 text-[10px] font-medium">
              <Code2Icon className="size-3" /> 编程
            </Badge>
          </div>
          <p className="mt-1 text-xs text-muted-foreground">
            编程题请在答题页编写代码并提交（客观题评分与交卷不包含编程题）。
          </p>
          <Link
            to={`/problem/${item.problemId}?back=${encodeURIComponent(`/s/${sid}/practice/${pid}`)}`}
            className="mt-2.5 inline-flex items-center gap-1.5 rounded-lg border border-border bg-background px-3 py-1.5 text-xs transition-colors hover:bg-muted"
          >
            前往做题页 <ArrowRightInline />
          </Link>
        </div>
      </div>
    </div>
  )
}

function ArrowRightInline() {
  return <span aria-hidden>→</span>
}

// ---------- 交卷结果 ----------

function ResultPanel({ result, items, onDismiss }: {
  result: { submissionId: number; results: PracticeResultItem[]; objectiveCorrect: number; objectiveTotal: number }
  items: PracticeDetail['items']
  onDismiss: () => void
}) {
  const byProblem = new Map(result.results.map((r) => [r.problemId, r]))
  const pct = result.objectiveTotal > 0 ? Math.round((result.objectiveCorrect / result.objectiveTotal) * 100) : 0
  const unanswered = items.filter((it) => !byProblem.has(it.problemId))
  const wrong = items.filter((it) => {
    const r = byProblem.get(it.problemId)
    return !!r && !r.correct
  })
  const perfect = wrong.length === 0 && unanswered.length === 0
  return (
    <div
      className={cn(
        'mt-3 flex flex-wrap items-center justify-between gap-x-4 gap-y-2 rounded-2xl border px-4 py-3',
        perfect ? 'border-emerald-200 bg-emerald-50/60' : 'border-orange-200 bg-orange-50/50',
      )}
    >
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-x-2 text-sm font-semibold">
          <span>本次交卷结果（#{result.submissionId}）</span>
          <span className="text-xs font-normal text-muted-foreground">
            答对 {result.objectiveCorrect} / 已作答 {result.objectiveTotal} 题（{pct}%）
            {unanswered.length > 0 && <> · 未作答 {unanswered.length} 题不计分</>}
          </span>
        </div>
        <div className="mt-1.5 flex flex-wrap items-center gap-1.5">
          <ResultStat label={`答对 ${result.objectiveCorrect}`} className="border-emerald-300 bg-emerald-50 text-emerald-700" />
          {wrong.length > 0 && <ResultStat label={`答错 ${wrong.length}`} className="border-orange-300 bg-orange-50 text-orange-700" />}
          {unanswered.length > 0 && <ResultStat label={`未作答 ${unanswered.length}`} className="border-border bg-muted/50 text-muted-foreground" />}
        </div>
      </div>
      <Button variant="outline" size="sm" onClick={onDismiss}>收起结果，继续作答</Button>
    </div>
  )
}

function ResultStat({ label, className }: { label: string; className?: string }) {
  return (
    <span className={cn('inline-flex items-center rounded-md border px-1.5 py-0.5 text-[11px] font-semibold', className)}>
      {label}
    </span>
  )
}

function correctAnswerText(r: PracticeResultItem): string {
  const ca = r.correctAnswer
  if (r.type === 'true_false') {
    if (ca === true || ca === false) return ca ? '对' : '错'
    if (ca && typeof ca === 'object' && 'answer' in ca && typeof ca.answer === 'boolean') return ca.answer ? '对' : '错'
    return '—'
  }
  const L = ['A', 'B', 'C', 'D', 'E', 'F', 'G', 'H']
  if (typeof ca === 'number') return `选项 ${L[ca] ?? ca}`
  if (ca && typeof ca === 'object' && 'answerIndex' in ca && typeof ca.answerIndex === 'number') {
    return `选项 ${L[ca.answerIndex] ?? ca.answerIndex}`
  }
  return '—'
}

// ---------- 交卷历史 ----------

function HistoryList({ sid, pid }: { sid: number; pid: number }) {
  const q = useQuery({
    queryKey: ['portal-practice-submissions', sid, pid],
    queryFn: () => api.portalPracticeSubmissions(sid, pid),
  })
  const list = q.data?.submissions ?? []
  if (!q.isLoading && list.length === 0) return null
  return (
    <div className="mt-6">
      <div className="mb-2 text-xs font-medium text-muted-foreground">交卷历史（最近 {Math.min(list.length, 10)} 次）</div>
      <div className="overflow-hidden rounded-xl border bg-card">
        {list.slice(0, 10).map((s) => (
          <div key={s.id} className="flex items-center gap-3 border-b px-4 py-2.5 text-xs last:border-b-0">
            <span className="text-muted-foreground">#{s.id}</span>
            <span className="font-medium text-emerald-600">答对 {s.objectiveCorrect} 题</span>
            <span className="ml-auto text-muted-foreground">{formatTime(s.createdAt)}</span>
          </div>
        ))}
      </div>
    </div>
  )
}

function formatTime(iso: string): string {
  try {
    return new Date(iso).toLocaleString('zh-CN', { hour12: false })
  } catch {
    return iso
  }
}

function Center({ text }: { text: string }) {
  return (
    <div className="mx-auto w-full max-w-3xl px-4 py-16 text-center lg:px-8">
      <div className="rounded-2xl border bg-card p-8">
        <p className="text-sm text-muted-foreground">{text}</p>
      </div>
    </div>
  )
}
