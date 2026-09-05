import { useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowLeftIcon, ClipboardListIcon, Code2Icon, Loader2Icon, SendIcon } from 'lucide-react'
import { toast } from 'sonner'

import { api } from '@/lib/api'
import type {
  ObjectiveAnswer, PracticeDetail, PracticeResultItem,
} from '@/lib/types'
import { ObjectiveQuestion } from '@/components/portal/objective'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription,
  AlertDialogFooter, AlertDialogHeader, AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { cn } from '@/lib/utils'

// 练习整卷：列出全部题目（客观题题面+选项就地可答可改；编程题跳现做题页），
// 交卷前可任意改选，交卷（POST submit）后展示逐题对错 + 得分 + 历史。
export function PracticeSolve() {
  const { spaceId, practiceId } = useParams()
  const sid = Number(spaceId)
  const pid = Number(practiceId)
  const q = useQuery({
    queryKey: ['portal-practice', sid, pid],
    queryFn: () => api.portalPractice(sid, pid),
  })
  const data = q.data

  if (q.isLoading) return <Center text="加载练习中…" />
  if (q.isError || !data) return <Center text="练习不存在或无权访问" />

  return <PracticePaper key={pid} sid={sid} pid={pid} data={data} />
}

function PracticePaper({ sid, pid, data }: {
  sid: number
  pid: number
  data: PracticeDetail
}) {
  const { practice, items } = data
  const objectiveItems = items.filter((i) => i.problemType === 'single_choice' || i.problemType === 'true_false')
  const programmingItems = items.filter((i) => i.problemType === 'programming')
  const qc = useQueryClient()

  // 每客观题已选题（单选=number，判断=boolean）
  const [answers, setAnswers] = useState<Record<number, ObjectiveAnswer>>({})
  const [confirmOpen, setConfirmOpen] = useState(false)
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

  return (
    <div className="mx-auto w-full max-w-3xl px-4 py-5 lg:px-8">
      <Link
        to={`/s/${sid}/practice`}
        className="inline-flex items-center gap-1.5 text-sm text-muted-foreground transition-colors hover:text-foreground"
      >
        <ArrowLeftIcon className="size-4" /> 返回练习列表
      </Link>

      <div className="mt-2 rounded-2xl border bg-card p-5">
        <h1 className="flex items-center gap-2 text-lg font-semibold">
          <ClipboardListIcon className="size-5 text-primary" />
          {practice.title}
        </h1>
        {practice.description && <p className="mt-1 text-sm text-muted-foreground">{practice.description}</p>}
        <div className="mt-3 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-muted-foreground">
          <span>{items.length} 题</span>
          <span>客观题已答 {answeredCount}/{objectiveItems.length}</span>
          {!result && (
            <span className="text-primary">交卷后统一评分，可重复交卷</span>
          )}
        </div>
      </div>

      {/* 交卷结果 */}
      {result && (
        <ResultPanel
          result={result}
          items={objectiveItems}
          onDismiss={() => setResult(null)}
        />
      )}

      {/* 题目卷面 */}
      <div className="mt-4 space-y-4">
        {objectiveItems.map((it, idx) => (
          <ObjectiveBlock
            key={it.problemId}
            item={it}
            idx={idx}
            selected={answers[it.problemId] ?? null}
            onToggle={(a) => toggleAnswer(it.problemId, a)}
          />
        ))}
        {programmingItems.map((it, idx) => (
          <ProgrammingBlock key={it.problemId} item={it} idx={objectiveItems.length + idx} sid={sid} pid={pid} />
        ))}
        {items.length === 0 && (
          <div className="rounded-xl border border-dashed p-12 text-center text-sm text-muted-foreground">
            本练习暂无题目
          </div>
        )}
      </div>

      {/* 交卷按钮（常驻） */}
      <div className="sticky bottom-3 z-10 mt-5">
        <div className="flex items-center justify-between rounded-2xl border bg-card/95 p-3 shadow-sm backdrop-blur">
          <span className="text-xs text-muted-foreground">
            已答 {answeredCount}/{objectiveItems.length} 道客观题
          </span>
          <Button
            className="min-h-9"
            disabled={submitting || answeredCount === 0 || !!result}
            onClick={() => setConfirmOpen(true)}
          >
            {submitting ? <Loader2Icon className="size-4 animate-spin" /> : <SendIcon className="size-4" />}
            交卷
          </Button>
        </div>
      </div>

      {/* 交卷确认 */}
      <AlertDialog open={confirmOpen} onOpenChange={setConfirmOpen}>
        <AlertDialogContent size="default">
          <AlertDialogHeader>
            <AlertDialogTitle>确认交卷？</AlertDialogTitle>
            <AlertDialogDescription>
              已作答客观题 {answeredCount}/{objectiveItems.length} 题（{programmingItems.length > 0 ? `另有 ${programmingItems.length} 道编程题请到做题页提交，不计入本次卷面；` : ''}交卷后立即评分并记录，可再次交卷重做）。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel onClick={() => setConfirmOpen(false)}>再检查一下</AlertDialogCancel>
            <AlertDialogAction onClick={() => void submitPaper()}>确认交卷</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <HistoryList sid={sid} pid={pid} />
    </div>
  )
}

// ---------- 客观题块（就地作答；交卷后展示对错） ----------

function ObjectiveBlock({ item, idx, selected, onToggle }: {
  item: PracticeDetail['items'][number]
  idx: number
  selected: ObjectiveAnswer | null
  onToggle: (a: ObjectiveAnswer) => void
}) {
  const contentQ = useQuery({
    queryKey: ['oj-problem', item.problemId],
    queryFn: () => api.ojProblem(item.problemId),
  })
  return (
    <div className="rounded-2xl border bg-card p-5">
      <div className="mb-2 flex items-center gap-2">
        <span className="flex size-6 shrink-0 items-center justify-center rounded-md bg-muted text-xs font-semibold">{idx + 1}</span>
        <span className="min-w-0 flex-1 truncate text-sm font-semibold">{item.problemTitle || `题目 #${item.problemId}`}</span>
        <Badge variant="secondary">{item.problemType === 'single_choice' ? '单选' : '判断'}</Badge>
      </div>
      {contentQ.isLoading && <p className="py-6 text-center text-xs text-muted-foreground">题目加载中…</p>}
      {contentQ.isError && (
        <p className="py-6 text-center text-xs text-muted-foreground">
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
  )
}

// ---------- 编程题块（跳现做题页） ----------

function ProgrammingBlock({ item, idx, sid, pid }: {
  item: PracticeDetail['items'][number]
  idx: number
  sid: number
  pid: number
}) {
  return (
    <div className="rounded-2xl border bg-card p-5">
      <div className="flex items-center gap-2">
        <span className="flex size-6 shrink-0 items-center justify-center rounded-md bg-muted text-xs font-semibold">{idx + 1}</span>
        <span className="min-w-0 flex-1 truncate text-sm font-semibold">{item.problemTitle || `题目 #${item.problemId}`}</span>
        <Badge variant="secondary"><Code2Icon className="size-3" /> 编程</Badge>
      </div>
      <p className="mt-2 text-xs text-muted-foreground">
        编程题请在答题页编写代码并提交（客观题评分与交卷不包含编程题）。
      </p>
      <Link
        to={`/problem/${item.problemId}?back=${encodeURIComponent(`/s/${sid}/practice/${pid}`)}`}
        className="mt-3 inline-flex items-center gap-1.5 rounded-lg border border-border bg-background px-3 py-1.5 text-xs transition-colors hover:bg-muted"
      >
        前往做题页 <ArrowRightInline />
      </Link>
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
  return (
    <div className="mt-4 rounded-2xl border border-emerald-200 bg-emerald-50/50 p-5">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <div className="text-sm font-semibold">本次交卷结果（#{result.submissionId}）</div>
          <div className="mt-1 text-xs text-muted-foreground">
            答对 {result.objectiveCorrect} / 已作答 {result.objectiveTotal} 题（{pct}%）
            {unanswered.length > 0 && <> · 未作答 {unanswered.length} 题不计分</>}
          </div>
        </div>
        <Button variant="outline" size="sm" onClick={onDismiss}>收起结果，继续作答</Button>
      </div>
      <div className="mt-3 space-y-1.5">
        {items.map((it) => {
          const r = byProblem.get(it.problemId)
          if (!r) {
            return (
              <div key={it.problemId} className="flex items-center gap-2 text-xs text-muted-foreground">
                <span className="size-2 shrink-0 rounded-full bg-muted-foreground/40" />
                <span className="min-w-0 flex-1 truncate">{it.problemTitle || `题目 #${it.problemId}`}</span>
                <span>未作答</span>
              </div>
            )
          }
          return (
            <div key={it.problemId} className="flex items-center gap-2 text-xs">
              <span className={cn('size-2 shrink-0 rounded-full', r.correct ? 'bg-emerald-500' : 'bg-red-500')} />
              <span className="min-w-0 flex-1 truncate">{it.problemTitle || `题目 #${it.problemId}`}</span>
              <span className={r.correct ? 'text-emerald-600' : 'text-red-600'}>
                {r.correct ? '正确' : `错误（正确项：${correctAnswerText(r)}）`}
              </span>
            </div>
          )
        })}
      </div>
    </div>
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
