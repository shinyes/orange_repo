import { useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowLeftIcon, CircleCheckBigIcon, FolderKanbanIcon, LockIcon, Loader2Icon, Code2Icon } from 'lucide-react'
import { toast } from 'sonner'

import { api } from '@/api'
import type { CorrectAnswer, ObjectiveAnswer, TrainingItemView } from '@/api/types'
import { ObjectiveQuestion } from '@/components/portal/objective'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle,
} from '@/components/ui/dialog'
import { cn } from '@/lib/utils'

// 训练详情：章节分组题目行；客观题点击弹层内联作答（即答即判 + 绿/红状态徽标）；
// 编程题点击跳现做题页（原路返回参数 back）。
export function TrainingDetail() {
  const { spaceId, trainingId } = useParams()
  const sid = Number(spaceId)
  const tid = Number(trainingId)
  const q = useQuery({
    queryKey: ['portal-training', sid, tid],
    queryFn: () => api.portalTraining(sid, tid),
  })
  const data = q.data

  if (q.isLoading) return <Center text="加载训练中…" />
  if (q.isError || !data) return <Center text="训练不存在或无权访问" />

  const { training, chapters } = data
  const all = (chapters ?? []).flatMap((c) => c.items ?? [])
  const objective = all.filter((i) => i.problemType === 'single_choice' || i.problemType === 'true_false')
  const solvedCount = objective.filter((i) => i.solved).length

  return (
    <div className="mx-auto w-full max-w-3xl px-4 py-5 lg:px-8">
      <Link
        to={`/s/${sid}/training`}
        className="inline-flex items-center gap-1.5 text-sm text-muted-foreground transition-colors hover:text-foreground"
      >
        <ArrowLeftIcon className="size-4" /> 返回训练列表
      </Link>

      <div className="mt-2 rounded-2xl border bg-card p-5">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <h1 className="flex items-center gap-2 text-lg font-semibold">
              <FolderKanbanIcon className="size-5 shrink-0 text-primary" />
              <span className="min-w-0 truncate">{training.title}</span>
            </h1>
            {training.description && <p className="mt-1 text-sm text-muted-foreground">{training.description}</p>}
          </div>
          <Badge variant="secondary" className="shrink-0">
            {training.maxAttempts > 0 ? `限答 ${training.maxAttempts} 次` : '不限次'}
          </Badge>
        </div>
        <div className="mt-3 flex items-center gap-4 text-xs text-muted-foreground">
          <span>共 {all.length} 题（客观题 {objective.length} 题参与限次）</span>
          <span className="text-emerald-600">已对 {solvedCount}</span>
        </div>
      </div>

      <div className="mt-4 space-y-3">
        {chapters.map((ch) => (
          <ChapterBlock key={ch.id} sid={sid} tid={tid} chapter={ch} maxAttempts={training.maxAttempts} />
        ))}
      </div>
    </div>
  )
}

function ChapterBlock({ sid, tid, chapter, maxAttempts }: {
  sid: number
  tid: number
  chapter: { id: number; title: string; items: TrainingItemView[] }
  maxAttempts: number
}) {
  const [active, setActive] = useState<TrainingItemView | null>(null)
  const items = chapter.items
  const greenCount = items.filter((i) => i.solved).length
  const allGreen = items.length > 0 && greenCount === items.filter((i) => i.problemType !== 'programming').length
  return (
    <div className="rounded-2xl border bg-card">
      <div className={cn('flex items-center gap-2 border-b px-4 py-2.5 text-sm font-semibold', allGreen && 'text-emerald-600')}>
        <span className="min-w-0 flex-1 truncate">{chapter.title || `第 ${chapter.id} 章`}</span>
        <span className="text-xs font-normal text-muted-foreground">{items.length} 题</span>
      </div>
      <div className="divide-y">
        {items.map((it, idx) => (
          <ItemRow
            key={it.problemId}
            item={it}
            idx={idx}
            sid={sid}
            tid={tid}
            maxAttempts={maxAttempts}
            onOpen={() => setActive(it)}
          />
        ))}
        {items.length === 0 && (
          <div className="p-6 text-center text-xs text-muted-foreground">本章暂无题目</div>
        )}
      </div>
      {active && (
        <TrainingObjectiveDialog
          key={active.problemId}
          sid={sid}
          tid={tid}
          item={active}
          maxAttempts={maxAttempts}
          open
          onClose={() => setActive(null)}
        />
      )}
    </div>
  )
}

function ItemRow({ item, idx, sid, tid, maxAttempts, onOpen }: {
  item: TrainingItemView
  idx: number
  sid: number
  tid: number
  maxAttempts: number
  onOpen: () => void
}) {
  const objective = item.problemType === 'single_choice' || item.problemType === 'true_false'
  const green = item.solved
  const red = !item.solved && item.locked

  let status: React.ReactNode
  if (!objective) {
    status = (
      <Badge variant="secondary" className="border-border">
        <Code2Icon className="size-3" /> 编程
      </Badge>
    )
  } else if (green) {
    status = (
      <Badge className="border-emerald-200 bg-emerald-50 text-emerald-700 hover:bg-emerald-50">
        <CircleCheckBigIcon className="size-3" /> 已答对
      </Badge>
    )
  } else if (red) {
    status = (
      <Badge className="border-red-200 bg-red-50 text-red-600 hover:bg-red-50">
        <LockIcon className="size-3" /> 已达上限
      </Badge>
    )
  } else {
    status = <Badge variant="outline">未做</Badge>
  }

  // 编程题 → 做题页（原路返回当前训练详情）
  if (!objective) {
    return (
      <Link
        to={`/problem/${item.problemId}?back=${encodeURIComponent(`/s/${sid}/training/${tid}`)}`}
        className="flex w-full items-center gap-3 px-4 py-3 text-left transition-colors hover:bg-primary/5"
      >
        <span className={cn(
          'flex size-7 shrink-0 items-center justify-center rounded-lg text-xs font-semibold',
          green ? 'bg-emerald-500 text-white' : red ? 'bg-red-500 text-white' : 'bg-muted text-muted-foreground',
        )}>
          {green ? '✓' : idx + 1}
        </span>
        <span className="min-w-0 flex-1 truncate text-sm font-medium">{item.problemTitle || `题目 #${item.problemId}`}</span>
        <ChevronAffordance />
        {status}
      </Link>
    )
  }

  const locked = green || red
  return (
    <button
      type="button"
      disabled={locked}
      onClick={onOpen}
      className={cn(
        'flex w-full items-center gap-3 px-4 py-3 text-left transition-colors',
        locked ? 'cursor-not-allowed opacity-80' : 'hover:bg-primary/5',
      )}
    >
      <span className={cn(
        'flex size-7 shrink-0 items-center justify-center rounded-lg text-xs font-semibold',
        green ? 'bg-emerald-500 text-white' : red ? 'bg-red-500 text-white' : 'bg-muted text-muted-foreground',
      )}>
        {green ? '✓' : idx + 1}
      </span>
      <span className="min-w-0 flex-1 truncate text-sm font-medium">{item.problemTitle || `题目 #${item.problemId}`}</span>
      <span className="shrink-0 text-[11px] text-muted-foreground">
        {maxAttempts > 0 ? `尝试 ${item.attempts}/${maxAttempts}` : `已尝试 ${item.attempts} 次`}
      </span>
      {status}
    </button>
  )
}

function ChevronAffordance() {
  return (
    <svg className="size-4 shrink-0 text-muted-foreground/60" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
      <path d="m9 18 6-6-6-6" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}

// 客观题弹层：取题目内容 → 点选作答（POST answer）→ 立即反馈对/错与正确项；
// 答错且仍有次数 → 可「再答一次」（不关弹层）。
function TrainingObjectiveDialog({ sid, tid, item, maxAttempts, open, onClose }: {
  sid: number
  tid: number
  item: TrainingItemView
  maxAttempts: number
  open: boolean
  onClose: () => void
}) {
  const qc = useQueryClient()
  const [selected, setSelected] = useState<ObjectiveAnswer | null>(null)
  const [feedback, setFeedback] = useState<{ correct: boolean; correctAnswer?: CorrectAnswer; lockedAfter?: boolean } | null>(null)
  const [busy, setBusy] = useState(false)
  // 弹层内的作答次数（随每次 POST 递增；闭层后由列表项状态接管）
  const [attemptsInDialog, setAttemptsInDialog] = useState(item.attempts)

  const contentQ = useQuery({
    queryKey: ['oj-problem', item.problemId],
    queryFn: () => api.ojProblem(item.problemId),
    enabled: open,
    retry: 1,
  })

  function resetLocal() {
    setSelected(null)
    setFeedback(null)
    setBusy(false)
    setAttemptsInDialog(item.attempts)
  }

  const locked = item.locked
  const used = locked ? item.attempts : attemptsInDialog
  const exhausted = maxAttempts > 0 && used >= maxAttempts

  async function answer(a: ObjectiveAnswer) {
    if (busy || feedback || locked) return
    setSelected(a)
    setBusy(true)
    try {
      const r = await api.portalTrainingAnswer(sid, tid, item.problemId, a)
      setFeedback({ correct: r.correct, correctAnswer: r.correctAnswer, lockedAfter: r.locked })
      setAttemptsInDialog(r.attempts)
      void qc.invalidateQueries({ queryKey: ['portal-training', sid, tid] })
      if (r.correct) toast.success('回答正确')
      else toast.error(r.locked ? '回答错误，本题次数已用尽' : '回答错误')
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '提交失败')
      setSelected(null)
    } finally {
      setBusy(false)
    }
  }

  function retry() {
    setSelected(null)
    setFeedback(null)
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(v) => { if (!v) { resetLocal(); onClose() } }}
    >
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>{item.problemTitle || `题目 #${item.problemId}`}</DialogTitle>
          <DialogDescription>
            {item.problemType === 'single_choice' ? '单选题' : '判断题'}
            {maxAttempts > 0 ? <> · 本训练限答 {maxAttempts} 次（已用 {used} 次）</> : <> · 不限作答次数（已答 {used} 次）</>}
            {(locked || exhausted) && ' · 本题已锁定'}
          </DialogDescription>
        </DialogHeader>

        <div className="max-h-[60vh] overflow-y-auto pr-1">
          {contentQ.isLoading && (
            <div className="flex items-center gap-2 py-10 text-sm text-muted-foreground">
              <Loader2Icon className="size-4 animate-spin" /> 正在加载题目…
            </div>
          )}
          {contentQ.isError && (
            <div className="py-8 text-center">
              <p className="text-sm text-muted-foreground">
                题目内容不可见（{contentQ.error instanceof Error ? contentQ.error.message : '加载失败'}）
              </p>
              <Button variant="outline" size="sm" className="mt-3" onClick={() => void contentQ.refetch()}>
                重试
              </Button>
            </div>
          )}
          {contentQ.data && (
            <ObjectiveQuestion
              problem={{
                type: contentQ.data.type as 'single_choice' | 'true_false',
                statementMd: contentQ.data.statementMd,
                bodyJson: contentQ.data.bodyJson,
              }}
              locked={locked || exhausted}
              busy={busy}
              selected={selected}
              feedback={feedback}
              onSelect={(a) => void answer(a)}
            />
          )}
        </div>

        <DialogFooter className="sm:justify-between">
          <Button variant="outline" onClick={() => { resetLocal(); onClose() }}>
            关闭
          </Button>
          {feedback && !feedback.correct && !feedback.lockedAfter && (
            <Button variant="secondary" onClick={retry}>
              再答一次
            </Button>
          )}
          <Button onClick={() => { resetLocal(); onClose() }} disabled={!feedback}>
            完成
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
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
