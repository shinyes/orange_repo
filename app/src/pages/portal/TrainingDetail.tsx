// 训练做题页（模仿上游 OrangeOJ 训练页布局）：
// 全屏三区 —— 紧凑顶栏（返回训练列表 × + 训练标题 + 进度）
// 　内容行：左侧训练导航（章节可折叠，每格一题：绿=通过/红=达上限/圈=当前）
// 　　　　　+ 中央题目卡（客观题内嵌作答即判；编程题跳做题页可返回）
// 　底部条：上一题 / 下一题 + x/y
// 移动端：隐藏左侧导航，顶栏「题目」按钮弹浮窗导航。
import { useEffect, useMemo, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  ChevronLeftIcon,
  ChevronRightIcon,
  CircleCheckBigIcon,
  Code2Icon,
  Grid3X3Icon,
  Loader2Icon,
  XIcon,
} from 'lucide-react'
import { toast } from 'sonner'

import { api } from '@/api'
import type { CorrectAnswer, ObjectiveAnswer, TrainingItemView } from '@/api/types'
import { ObjectiveQuestion } from '@/components/portal/objective'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from '@/components/ui/dialog'
import { useSpaceById } from '@/pages/portal/portal-context'
import { cn } from '@/lib/utils'

type FlatItem = TrainingItemView & { chapterId: number; chapterTitle: string; chapterOrder: number }

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
  const groups = useMemo(() => {
    let base = 0
    return (chapters ?? []).map((ch, i) => {
      const items: FlatItem[] = (ch.items ?? []).map((it) => ({
        ...it, chapterId: ch.id, chapterTitle: ch.title || `第 ${i + 1} 章`, chapterOrder: i,
      }))
      const g = { chapterId: ch.id, chapterTitle: ch.title || `第 ${i + 1} 章`, order: i, base, items }
      base += items.length
      return g
    })
  }, [chapters])
  const all = useMemo(() => groups.flatMap((g) => g.items), [groups])
  const objective = all.filter((i) => i.problemType !== 'programming')
  const [activeIdx, setActiveIdx] = useState(0)
  const [navOpen, setNavOpen] = useState(false) // 移动端导航浮窗
  // 章节折叠（导航）：记录每章手动开关；当前章强制展开（模仿上游）
  const [collapsed, setCollapsed] = useState<Record<number, boolean>>({})
  const item = all[activeIdx] ?? null
  const solvedCount = objective.filter((i) => i.solved).length

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
  const currentGroup = item?.chapterOrder ?? 0

  const isGroupCollapsed = (g: { order: number; items: FlatItem[] }): boolean => {
    if (g.order === currentGroup) return false // 当前章始终展开
    if (collapsed[g.order] !== undefined) return collapsed[g.order]
    const allDone = g.items.length > 0 && g.items.every((it) => it.solved)
    return allDone // 默认：全通过的章折叠
  }

  if (all.length === 0) {
    return (
      <div className="flex h-dvh flex-col bg-background">
        <CompactTop title={training.title} backTo={`/s/${sid}/training`} />
        <div className="flex flex-1 items-center justify-center text-sm text-muted-foreground">本训练暂无题目</div>
      </div>
    )
  }

  const itemObjective = item && item.problemType !== 'programming'

  return (
    <div className="flex h-dvh flex-col overflow-hidden bg-background">
      {/* 紧凑顶栏：训练标题 + 进度 + 返回列表 ×（模仿上游） */}
      <header className="sticky top-0 z-40 shrink-0 border-b bg-background shadow-sm">
        <div className="flex min-h-10 items-center justify-between gap-2 px-2 md:px-4">
          <div className="flex min-w-0 items-center gap-2">
            <span className="truncate text-xs font-semibold md:text-sm">{training.title}</span>
            {training.maxAttempts > 0 && (
              <Badge variant="secondary" className="hidden text-[10px] sm:inline-flex">限答 {training.maxAttempts} 次</Badge>
            )}
            <span className="hidden text-[11px] text-muted-foreground md:inline">
              {activeIdx + 1} / {all.length} · 已通过 {solvedCount}
            </span>
          </div>
          <div className="flex shrink-0 items-center gap-1">
            {/* 移动端题目导航按钮 */}
            <Button size="sm" variant="outline" className="h-7 px-2 text-xs md:hidden" onClick={() => setNavOpen(true)}>
              <Grid3X3Icon className="size-3.5" /> 题目
            </Button>
            <Link to={`/s/${sid}/training`} title="返回训练列表">
              <Button size="icon" variant="ghost" className="h-7 w-7 md:h-8 md:w-8">
                <XIcon className="size-4" />
              </Button>
            </Link>
          </div>
        </div>
      </header>

      {/* 内容行：左导航（PC）+ 中央题目 */}
      <div className="flex min-h-0 flex-1 flex-col gap-1.5 overflow-hidden p-1.5 md:flex-row md:gap-3 md:p-3">
        {/* 左侧训练导航（PC） */}
        <aside className="hidden w-44 shrink-0 overflow-y-auto rounded-lg border-r bg-muted/20 p-2 md:block">
          <div className="space-y-2.5">
            {groups.map((g) => {
              if (g.items.length === 0) return null
              const collapsedG = isGroupCollapsed(g)
              const allDone = g.items.length > 0 && g.items.every((it) => it.solved)
              return (
                <div key={g.chapterId}>
                  <button
                    type="button"
                    className="flex w-full items-center gap-1 text-left"
                    onClick={() => setCollapsed((p) => ({ ...p, [g.order]: !isGroupCollapsed(g) }))}
                  >
                    <span className={cn('min-w-0 flex-1 truncate text-[11px] font-semibold tracking-wide', allDone ? 'text-emerald-600' : 'text-muted-foreground')}>
                      {g.chapterTitle}
                    </span>
                    <span className="shrink-0 text-[9px] text-muted-foreground">{collapsedG ? '▶' : '▼'}</span>
                  </button>
                  {!collapsedG && (
                    <div className="mt-1 grid grid-cols-5 gap-1">
                      {g.items.map((it, j) => {
                        const idx = g.base + j
                        const isCurrent = idx === activeIdx
                        const obj = it.problemType !== 'programming'
                        return (
                          <button
                            key={it.problemId}
                            type="button"
                            title={it.problemTitle || `题目 #${it.problemId}`}
                            onClick={() => go(idx)}
                            className={cn(
                              'flex h-7 w-full items-center justify-center rounded border text-xs font-medium transition-colors',
                              it.solved && obj
                                ? 'border-green-400 bg-green-50 text-green-700 hover:bg-green-100'
                                : !it.solved && it.locked && obj
                                  ? 'border-red-300 bg-red-50 text-red-600 hover:bg-red-100'
                                  : 'border-border bg-background hover:border-primary hover:bg-slate-50',
                              isCurrent && 'ring-2 ring-primary ring-offset-1',
                            )}
                          >
                            {g.base + j + 1}
                          </button>
                        )
                      })}
                    </div>
                  )}
                </div>
              )
            })}
            <p className="pt-1 text-[10px] leading-relaxed text-muted-foreground">
              绿=已通过 · 红=达上限 · 点击格子跳题
            </p>
          </div>
        </aside>

        {/* 中央：题目卡 */}
        <div className="min-h-0 flex-1 overflow-y-auto">
          <div className="mx-auto max-w-3xl">
            {item && (
              <div className="rounded-xl border bg-card">
                <div className="p-3 md:p-4">
                  {/* 题目头（章节 + 题号 + 状态） */}
                  <div className="mb-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
                    <span className="rounded bg-muted px-1.5 py-0.5 font-medium">{item.chapterTitle}</span>
                    <span>第 {activeIdx + 1} / {all.length} 题</span>
                    {item.solved && itemObjective && (
                      <span className="inline-flex items-center gap-1 text-emerald-600">
                        <CircleCheckBigIcon className="size-3.5" /> 已通过
                      </span>
                    )}
                    {!item.solved && item.locked && itemObjective && (
                      <span className="inline-flex items-center gap-1 text-red-600">
                        已达上限，可回顾
                      </span>
                    )}
                  </div>

                  {itemObjective ? (
                    <ObjectiveCard
                      key={`${sid}-${tid}-${item.problemId}-${item.solved ? 1 : 0}-${item.locked ? 1 : 0}`}
                      sid={sid} tid={tid} item={item} maxAttempts={training.maxAttempts}
                      onAnswered={invalidate}
                    />
                  ) : (
                    // 编程题：前往做题页（原路返回本训练）
                    <div className="flex flex-col items-center gap-3 py-12 text-center">
                      <Code2Icon className="size-12 text-muted-foreground/40" />
                      <div>
                        <p className="text-lg font-medium">{item.problemTitle || `题目 #${item.problemId}`}</p>
                        <p className="mt-1 text-xs text-muted-foreground">编程题请进入代码编辑器作答并评测（通过后自动标记）</p>
                      </div>
                      <Link
                        to={`/problem/${item.problemId}?back=${encodeURIComponent(`/s/${sid}/training/${tid}`)}`}
                        className="mt-2"
                      >
                        <Button size="lg" className="min-w-44">前往做题 →</Button>
                      </Link>
                    </div>
                  )}
                </div>
              </div>
            )}

            {/* 最后一题完成提示 */}
            {activeIdx === all.length - 1 && all.length > 0 && (
              <p className="mt-3 text-center text-xs text-muted-foreground">
                {objective.length > 0 && solvedCount < objective.length
                  ? `已是最后一题（客观题已通过 ${solvedCount}/${objective.length}）`
                  : '训练全部完成 🎉 可在左侧回顾任意题目'}
              </p>
            )}
          </div>
        </div>
      </div>

      {/* 底部导航条（上一题/下一题） */}
      {all.length > 1 && (
        <footer className="shrink-0 border-t bg-background">
          <div className="flex items-center justify-center gap-4 px-4 py-2">
            <Button variant="outline" size="sm" disabled={activeIdx === 0} onClick={() => go(activeIdx - 1)}>
              <ChevronLeftIcon className="mr-1 size-3.5" /> 上一题
            </Button>
            <span className="flex items-center gap-1.5 text-xs text-muted-foreground">
              {activeIdx + 1} / {all.length}
            </span>
            <Button variant="outline" size="sm" disabled={activeIdx >= all.length - 1} onClick={() => go(activeIdx + 1)}>
              下一题 <ChevronRightIcon className="ml-1 size-3.5" />
            </Button>
          </div>
        </footer>
      )}

      {/* 移动端导航浮窗 */}
      <Dialog open={navOpen} onOpenChange={setNavOpen}>
        <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2 text-base">
              <Grid3X3Icon className="size-4 text-primary" /> 题目导航
            </DialogTitle>
            <DialogDescription>{training.title} · 绿=已通过 红=达上限</DialogDescription>
          </DialogHeader>
          <div className="space-y-3">
            {groups.map((g) => {
              if (g.items.length === 0) return null
              return (
                <div key={g.chapterId}>
                  <div className="mb-1 flex items-center justify-between text-[11px] font-medium text-muted-foreground">
                    <span className="truncate">{g.chapterTitle}</span>
                    <span>{g.items.length}</span>
                  </div>
                  <div className="grid grid-cols-6 gap-1.5">
                    {g.items.map((it, j) => {
                      const idx = g.base + j
                      const obj = it.problemType !== 'programming'
                      return (
                        <button
                          key={it.problemId}
                          type="button"
                          onClick={() => go(idx)}
                          className={cn(
                            'flex aspect-square items-center justify-center rounded-md text-xs font-semibold transition-transform hover:scale-110',
                            idx === activeIdx && 'ring-2 ring-primary ring-offset-1',
                            it.solved && obj ? 'bg-emerald-500 text-white'
                              : !it.solved && it.locked && obj ? 'bg-red-500 text-white'
                                : !obj ? 'border border-dashed bg-muted/40 text-muted-foreground'
                                  : 'bg-muted text-muted-foreground hover:bg-muted/70',
                          )}
                        >
                          {idx + 1}
                        </button>
                      )
                    })}
                  </div>
                </div>
              )
            })}
          </div>
        </DialogContent>
      </Dialog>
    </div>
  )
}

// ---------- 紧凑顶栏 ----------

function CompactTop({ title, backTo }: { title: string; backTo: string }) {
  return (
    <header className="shrink-0 border-b bg-background">
      <div className="flex min-h-10 items-center justify-between px-3">
        <span className="truncate text-sm font-semibold">{title}</span>
        <Link to={backTo} title="返回训练列表">
          <Button size="icon" variant="ghost" className="h-7 w-7">
            <XIcon className="size-4" />
          </Button>
        </Link>
      </div>
    </header>
  )
}

// ---------- 客观题内嵌作答卡（先选答案 → 提交即判，保留回顾） ----------

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

  async function submit() {
    if (busy || feedback || readOnly || selected == null) return
    setBusy(true)
    try {
      const r = await api.portalTrainingAnswer(sid, tid, item.problemId, selected)
      setFeedback({ correct: r.correct, correctAnswer: r.correctAnswer })
      if (r.correct) toast.success('回答正确')
      else toast.error(r.locked ? '回答错误，本题次数已用尽' : '回答错误')
      onAnswered()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '提交失败')
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
          onSelect={(a) => setSelected(a)}
        />
      )}
      {!readOnly && !feedback && selected != null && (
        <div className="mt-3 flex justify-end">
          <Button onClick={() => void submit()} disabled={busy}>
            {busy ? '提交中…' : '提交答案'}
          </Button>
        </div>
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

function Center({ text }: { text: string }) {
  return (
    <div className="mx-auto w-full max-w-3xl px-4 py-16 text-center">
      <div className="rounded-2xl border bg-card p-8">
        <p className="text-sm text-muted-foreground">{text}</p>
      </div>
    </div>
  )
}
