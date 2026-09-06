// 训练做题页（单题沉浸流）：进入训练直接开始第一题——
// 主区逐题展示（客观题内嵌作答即判；编程题跳做题页可返回）；
// 题目导航器：PC 固定悬浮（右侧），移动端顶栏按钮 → 浮窗；
// 格子颜色：已通过绿 / 达上限红 / 进行中高亮 / 未做灰。
import { useEffect, useMemo, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  ChevronLeftIcon,
  ChevronRightIcon,
  CircleCheckBigIcon,
  Code2Icon,
  FolderKanbanIcon,
  Grid3X3Icon,
  LockIcon,
  Loader2Icon,
} from 'lucide-react'
import { toast } from 'sonner'

import { api } from '@/api'
import type { CorrectAnswer, ObjectiveAnswer, TrainingItemView } from '@/api/types'
import { ObjectiveQuestion } from '@/components/portal/objective'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from '@/components/ui/dialog'
import { useSpaceById } from '@/pages/portal/portal-context'
import { PageContainer, SpacePageShell } from '@/pages/portal/SpacePageShell'
import { cn } from '@/lib/utils'

type FlatItem = TrainingItemView & { chapterId: number; chapterTitle: string }

export function TrainingDetail() {
  const { spaceId, trainingId } = useParams()
  const sid = Number(spaceId)
  const tid = Number(trainingId)
  const space = useSpaceById(sid)
  const q = useQuery({
    queryKey: ['portal-training', sid, tid],
    queryFn: () => api.portalTraining(sid, tid),
  })
  const data = q.data

  if (space === 'loading' || q.isLoading) return <Center text="加载训练中…" />
  if (space === null || q.isError || !data) return <Center text="训练不存在或无权访问" />

  return <TrainingFlow sid={sid} tid={tid} data={data} />
}

function TrainingFlow({ sid, tid, data }: {
  sid: number
  tid: number
  data: { training: { id: number; title: string; description?: string; maxAttempts: number }; chapters: { id: number; title: string; items: TrainingItemView[] }[] }
}) {
  const qc = useQueryClient()
  const { training, chapters } = data
  const all: FlatItem[] = useMemo(
    () => (chapters ?? []).flatMap((ch) => (ch.items ?? []).map((it) => ({ ...it, chapterId: ch.id, chapterTitle: ch.title || `第 ${ch.id} 章` }))),
    [chapters],
  )
  const objective = all.filter((i) => i.problemType !== 'programming')
  const [activeIdx, setActiveIdx] = useState(0)
  const [navOpen, setNavOpen] = useState(false) // 移动端导航浮窗
  const item = all[activeIdx] ?? null
  const solvedCount = objective.filter((i) => i.solved).length

  // 数据刷新后若当前题已不存在则回退
  useEffect(() => {
    if (all.length === 0) return
    if (activeIdx >= all.length) setActiveIdx(all.length - 1)
  }, [all.length, activeIdx])

  const invalidate = () => void qc.invalidateQueries({ queryKey: ['portal-training', sid, tid] })
  const go = (i: number) => {
    if (i < 0 || i >= all.length) return
    setActiveIdx(i)
    setNavOpen(false)
  }

  if (all.length === 0) {
    return (
      <SpacePageShell spaceId={sid} backTo={`/s/${sid}/training`} backLabel="返回训练列表">
        <PageContainer>
          <TrainingHeader title={training.title} description={training.description} maxAttempts={training.maxAttempts} total={0} solved={0} />
          <div className="rounded-xl border border-dashed p-12 text-center text-sm text-muted-foreground">
            本训练暂无题目
          </div>
        </PageContainer>
      </SpacePageShell>
    )
  }

  const itemObjective = item && item.problemType !== 'programming'

  return (
    <SpacePageShell spaceId={sid} backTo={`/s/${sid}/training`} backLabel="返回训练列表">
      {/* 移动端：顶部浮动「题目导航」按钮 */}
      <div className="sticky top-0 z-20 flex items-center justify-between gap-2 border-b bg-background/95 px-3 py-2 backdrop-blur lg:hidden">
        <span className="min-w-0 truncate text-xs font-medium text-muted-foreground">
          {training.title}
        </span>
        <Button size="sm" variant="outline" onClick={() => setNavOpen(true)}>
          <Grid3X3Icon className="size-4" /> 题目 {activeIdx + 1}/{all.length}
        </Button>
      </div>

      {/* 主区 */}
      <PageContainer className="lg:pl-72">
        <TrainingHeader title={training.title} description={training.description} maxAttempts={training.maxAttempts} total={all.length} solved={solvedCount} />

        {item && (
          <div className="mt-4 rounded-2xl border bg-card p-4 sm:p-5">
            {/* 题头：章节/序号/状态 */}
            <div className="mb-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
              <span className="rounded bg-muted px-1.5 py-0.5 font-medium">{item.chapterTitle}</span>
              <span>第 {activeIdx + 1} / {all.length} 题</span>
              {itemObjective && (
                <span>{training.maxAttempts > 0 ? `限答 ${training.maxAttempts} 次` : '不限次'}</span>
              )}
              <span className="ml-auto flex items-center gap-1.5">
                {item.solved && (
                  <Badge className="border-emerald-200 bg-emerald-50 text-emerald-700 hover:bg-emerald-50">
                    <CircleCheckBigIcon className="size-3" /> 已答对
                  </Badge>
                )}
                {!item.solved && item.locked && itemObjective && (
                  <Badge className="border-red-200 bg-red-50 text-red-600 hover:bg-red-50">
                    <LockIcon className="size-3" /> 已达上限
                  </Badge>
                )}
              </span>
            </div>

            {itemObjective ? (
              <ObjectiveCard
                key={`${sid}-${tid}-${item.problemId}-${item.solved ? 1 : 0}-${item.locked ? 1 : 0}`}
                sid={sid} tid={tid} item={item} maxAttempts={training.maxAttempts}
                onAnswered={invalidate}
              />
            ) : (
              // 编程题：前往做题页（原路返回）
              <div className="flex flex-col items-center gap-3 py-10 text-center">
                <Code2Icon className="size-10 text-muted-foreground/50" />
                <div>
                  <p className="text-base font-medium">{item.problemTitle || `题目 #${item.problemId}`}</p>
                  <p className="mt-1 text-xs text-muted-foreground">编程题请在代码编辑器中作答并评测</p>
                </div>
                <Link
                  to={`/problem/${item.problemId}?back=${encodeURIComponent(`/s/${sid}/training/${tid}`)}`}
                  className="mt-1"
                >
                  <Button size="lg" className="min-w-40">
                    前往做题 →
                  </Button>
                </Link>
              </div>
            )}
          </div>
        )}

        {/* 上一题 / 下一题 */}
        {all.length > 1 && (
          <div className="mt-4 flex items-center justify-between gap-3">
            <Button variant="outline" disabled={activeIdx === 0} onClick={() => go(activeIdx - 1)}>
              <ChevronLeftIcon className="size-4" /> 上一题
            </Button>
            <span className="text-xs text-muted-foreground">{activeIdx + 1} / {all.length}</span>
            <Button disabled={activeIdx >= all.length - 1} onClick={() => go(activeIdx + 1)}>
              下一题 <ChevronRightIcon className="size-4" />
            </Button>
          </div>
        )}
        {activeIdx === all.length - 1 && all.length > 0 && (
          <p className="mt-2 text-center text-xs text-muted-foreground">
            已是最后一题{objective.length > 0 && solvedCount < objective.length ? `（客观题已通过 ${solvedCount}/${objective.length}）` : '（全部完成 🎉）'}
          </p>
        )}
      </PageContainer>

      {/* PC：悬浮导航器（固定右下，章节分组 + 题目格子） */}
      <TrainingNavigator
        all={all}
        activeIdx={activeIdx}
        onSelect={go}
        className="fixed top-1/2 left-4 z-20 hidden max-h-[70vh] w-64 -translate-y-1/2 lg:block"
      />

      {/* 移动端：导航浮窗 */}
      <Dialog open={navOpen} onOpenChange={setNavOpen}>
        <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2 text-base">
              <Grid3X3Icon className="size-4 text-primary" /> 题目导航
            </DialogTitle>
            <DialogDescription>
              {training.title} · 绿=已通过 红=已达上限 蓝框=当前题
            </DialogDescription>
          </DialogHeader>
          <TrainingNavigator all={all} activeIdx={activeIdx} onSelect={go} className="" />
        </DialogContent>
      </Dialog>
    </SpacePageShell>
  )
}

// ---------- 客观题内嵌作答卡（即答即判） ----------

function ObjectiveCard({ sid, tid, item, maxAttempts, onAnswered }: {
  sid: number
  tid: number
  item: TrainingItemView
  maxAttempts: number
  onAnswered: () => void
}) {
  const [selected, setSelected] = useState<ObjectiveAnswer | null>(null)
  const [feedback, setFeedback] = useState<{ correct: boolean; correctAnswer?: CorrectAnswer } | null>(null)
  const [busy, setBusy] = useState(false)

  const contentQ = useQuery({
    queryKey: ['oj-problem', item.problemId],
    queryFn: () => api.ojProblem(item.problemId),
    retry: 1,
  })

  const readOnly = item.solved || (item.locked && maxAttempts > 0 && item.attempts >= maxAttempts)

  async function answer(a: ObjectiveAnswer) {
    if (busy || feedback || readOnly) return
    setSelected(a)
    setBusy(true)
    try {
      const r = await api.portalTrainingAnswer(sid, tid, item.problemId, a)
      setFeedback({ correct: r.correct, correctAnswer: r.correctAnswer })
      if (r.correct) toast.success('回答正确')
      else toast.error(r.locked ? '回答错误，本题次数已用尽' : '回答错误')
      onAnswered()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '提交失败')
      setSelected(null)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div>
      {contentQ.isLoading && (
        <div className="flex items-center gap-2 py-8 text-sm text-muted-foreground">
          <Loader2Icon className="size-4 animate-spin" /> 正在加载题目…
        </div>
      )}
      {contentQ.isError && (
        <div className="py-6 text-center text-sm text-muted-foreground">
          题目内容加载失败（{contentQ.error instanceof Error ? contentQ.error.message : '未知错误'}）
        </div>
      )}
      {contentQ.data && (
        <ObjectiveQuestion
          problem={{
            type: contentQ.data.type as 'single_choice' | 'true_false',
            statementMd: contentQ.data.statementMd,
            bodyJson: contentQ.data.bodyJson,
          }}
          locked={readOnly}
          busy={busy}
          selected={selected}
          feedback={feedback}
          onSelect={(a) => void answer(a)}
        />
      )}
      {feedback && !feedback.correct && !readOnly && (
        <div className="mt-3 flex justify-end">
          <Button size="sm" variant="outline" onClick={() => { setSelected(null); setFeedback(null) }}>
            再答一次
          </Button>
        </div>
      )}
      {readOnly && (
        <p className="mt-3 rounded-lg bg-muted/60 px-3 py-2 text-xs text-muted-foreground">
          {item.solved ? '本题已通过，可查看回顾（不可再作答）。' : '本题次数已用尽，可查看回顾。'}
        </p>
      )}
    </div>
  )
}

// ---------- 题目导航（章节分组 + 格子） ----------

function TrainingNavigator({ all, activeIdx, onSelect, className }: {
  all: FlatItem[]
  activeIdx: number
  onSelect: (i: number) => void
  className?: string
}) {
  // 按章节分组
  const groups = useMemo(() => {
    const map = new Map<string, { chapterId: number; chapterTitle: string; items: FlatItem[]; base: number }>()
    all.forEach((it, i) => {
      const key = String(it.chapterId)
      if (!map.has(key)) map.set(key, { chapterId: it.chapterId, chapterTitle: it.chapterTitle, items: [], base: i })
      map.get(key)!.items.push(it)
    })
    return [...map.values()]
  }, [all])

  return (
    <div className={cn('rounded-2xl border bg-card/95 p-3 shadow-lg backdrop-blur', className)}>
      <div className="mb-2 flex items-center gap-1.5 text-sm font-semibold">
        <FolderKanbanIcon className="size-4 text-primary" />
        题目导航
        <span className="ml-auto text-[10px] font-normal text-muted-foreground">{all.length} 题</span>
      </div>
      <div className="max-h-[52vh] space-y-3 overflow-y-auto pr-1">
        {groups.map((g) => (
          <div key={g.chapterId}>
            <div className="mb-1 flex items-center justify-between text-[11px] text-muted-foreground">
              <span className="truncate">{g.chapterTitle}</span>
              <span>{g.items.length}</span>
            </div>
            <div className="grid grid-cols-6 gap-1.5">
              {g.items.map((it, j) => {
                const idx = g.base + j
                const isActive = idx === activeIdx
                const objective = it.problemType !== 'programming'
                const green = it.solved
                const red = !it.solved && it.locked
                return (
                  <button
                    key={it.problemId}
                    type="button"
                    title={`${it.chapterTitle} · ${it.problemTitle || `题目 #${it.problemId}`}`}
                    onClick={() => onSelect(idx)}
                    className={cn(
                      'flex aspect-square items-center justify-center rounded-md text-xs font-semibold transition-transform hover:scale-110',
                      isActive && 'ring-2 ring-primary ring-offset-1',
                      green && objective ? 'bg-emerald-500 text-white'
                        : red && objective ? 'bg-red-500 text-white'
                          : !objective ? 'border border-dashed bg-muted/40 text-muted-foreground'
                            : 'bg-muted text-muted-foreground hover:bg-muted/70',
                    )}
                  >
                    {idx + 1}
                  </button>
                )
              })}
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}

// ---------- 头部 ----------

function TrainingHeader({ title, description, maxAttempts, total, solved }: {
  title: string
  description?: string
  maxAttempts: number
  total: number
  solved: number
}) {
  return (
    <div className="mt-2 rounded-2xl border bg-card p-5">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <h1 className="flex items-center gap-2 text-lg font-semibold">
            <FolderKanbanIcon className="size-5 shrink-0 text-primary" />
            <span className="min-w-0 truncate">{title}</span>
          </h1>
          {description && <p className="mt-1 text-sm text-muted-foreground">{description}</p>}
        </div>
        <Badge variant="secondary" className="shrink-0">
          {maxAttempts > 0 ? `限答 ${maxAttempts} 次` : '不限次'}
        </Badge>
      </div>
      <div className="mt-3 flex items-center gap-4 text-xs text-muted-foreground">
        <span>共 {total} 题</span>
        <span className="text-emerald-600">已通过 {solved}</span>
        <span className="ml-auto text-muted-foreground/70">绿=通过 · 红=达上限</span>
      </div>
    </div>
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
