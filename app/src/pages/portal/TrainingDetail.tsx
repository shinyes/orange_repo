// 训练做题页（模仿上游布局 + 内嵌编程答题）：
// 空间壳顶栏（返回训练列表/我的）下，三区——左训练导航（章节分组，每格一题常显不折叠）、
// 中央题目区（客观题先选后提交即判；编程题页内编辑器 运行/测试/提交/控制台）、
// 底部上一题/下一题。移动端顶栏「题目」按钮 → 浮窗导航。
import { useEffect, useMemo, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  ChevronLeftIcon,
  ChevronRightIcon,
  CircleCheckBigIcon,
  Code2Icon,
  Grid3X3Icon,
  Loader2Icon,
  RotateCcwIcon,
  ZoomInIcon,
  ZoomOutIcon,
} from 'lucide-react'
import { toast } from 'sonner'

import { api } from '@/api'
import type { CorrectAnswer, ObjectiveAnswer, TrainingItemView } from '@/api/types'
import { TrainingProgrammingCard } from '@/components/portal/TrainingProgrammingCard'
import { SplitPane } from '@/components/portal/SplitPane'
import { ObjectiveQuestion } from '@/components/portal/objective'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from '@/components/ui/dialog'
import { Markdown, preserveLineBreaks } from '@/lib/markdown'
import { useSpaceById } from '@/pages/portal/portal-context'
import { PageContainer, SpacePageShell } from '@/pages/portal/SpacePageShell'
import { cn } from '@/lib/utils'

type FlatItem = TrainingItemView & { chapterId: number; chapterTitle: string; chapterOrder: number }

export function TrainingDetail() {
  const { spaceId, trainingId, no } = useParams()
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

  return <TrainingFlow sid={sid} tid={tid} data={data} urlNo={no ? Number(no) : undefined} />
}

function TrainingFlow({ sid, tid, data, urlNo }: {
  sid: number
  tid: number
  data: { training: { id: number; title: string; description?: string; maxAttempts: number }; chapters: { id: number; title: string; items: TrainingItemView[] }[] }
  /** URL 中的题号（1 基；无=首次进入待补） */
  urlNo?: number
}) {
  const qc = useQueryClient()
  const navigate = useNavigate()
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
  const [navOpen, setNavOpen] = useState(false) // 移动端导航浮窗
  // 题面字号缩放（zoom 视觉缩放整块题面）
  const [statementScale, setStatementScale] = useState(1)

  // 当前题号由 URL 派生（1 基；越界 clamp）——刷新/前进后退保持在对应题
  const activeIdx = urlNo == null ? 0 : Math.min(Math.max(1, urlNo), all.length) - 1
  const item = all[activeIdx] ?? null

  // 首次进入（URL 无题号）→ 补写 ?q 路径（replace，不产生历史记录）
  useEffect(() => {
    if (urlNo == null && all.length > 0) {
      navigate(`/s/${sid}/training/${tid}/q/1`, { replace: true })
    }
  }, [urlNo, all.length, sid, tid, navigate])

  const invalidate = () => void qc.invalidateQueries({ queryKey: ['portal-training', sid, tid] })
  const go = (i: number) => {
    if (i < 0 || i >= all.length) return
    setNavOpen(false)
    navigate(`/s/${sid}/training/${tid}/q/${i + 1}`)
  }

  if (all.length === 0) {
    return (
      <SpacePageShell spaceId={sid} backTo={`/s/${sid}/training`} backLabel="返回训练列表">
        <PageContainer>
          <div className="rounded-xl border border-dashed p-12 text-center text-sm text-muted-foreground">本训练暂无题目</div>
        </PageContainer>
      </SpacePageShell>
    )
  }

  const itemObjective = item && item.problemType !== 'programming'
  const itemSolved = item?.solved ?? false

  return (
    <SpacePageShell spaceId={sid} backTo={`/s/${sid}/training`} backLabel="返回训练列表">
      <div className="flex h-full min-h-0 flex-col">
        {/* 移动端顶部：训练名 + 题目导航按钮 */}
        <div className="flex shrink-0 items-center justify-between gap-2 border-b bg-background px-3 py-1.5 lg:hidden">
          <span className="min-w-0 truncate text-xs font-medium text-muted-foreground">
            {training.title} · {activeIdx + 1}/{all.length}
          </span>
          <Button size="sm" variant="outline" onClick={() => setNavOpen(true)}>
            <Grid3X3Icon className="size-4" /> 题目
          </Button>
        </div>

        {/* 内容区：flex 行（左导航 + 题区/编辑区），唯一滚动由各自子区承担 */}
        <div className="flex min-h-0 flex-1 flex-col md:flex-row">
          {/* 左训练导航（PC） */}
          <aside className="hidden w-48 shrink-0 overflow-y-auto border-r bg-muted/20 p-2.5 md:block">
            <div className="space-y-3">
              {groups.map((g) => {
                if (g.items.length === 0) return null
                const allDone = g.items.every((it) => it.solved)
                return (
                  <div key={g.chapterId}>
                    <div className={cn('mb-1 flex items-center justify-between text-[11px] font-semibold', allDone ? 'text-emerald-600' : 'text-muted-foreground')}>
                      <span className="truncate">{g.chapterTitle}</span>
                      <span className="shrink-0 text-[9px] font-normal">{g.items.length}</span>
                    </div>
                    <div className="grid grid-cols-5 gap-1">
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
                            {idx + 1}
                          </button>
                        )
                      })}
                    </div>
                  </div>
                )
              })}
            </div>
            <p className="mt-3 border-t pt-2 text-[10px] leading-relaxed text-muted-foreground">
              绿=已通过 · 红=达上限
            </p>
          </aside>

          {/* 中央：题目区（客观题整宽；编程题=可拖拽分栏：左题面/右编辑器） */}
          <div className="min-h-0 min-w-0 flex-1">
            {item &&
              (itemObjective ? (
                <div className="h-full overflow-y-auto">
                  <PageContainer className="py-4">
                    {/* 题目头（题面文字缩放控件置右） */}
                    <div className="mb-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
                      <span className="rounded bg-muted px-1.5 py-0.5 font-medium">{item.chapterTitle}</span>
                      <span>第 {activeIdx + 1} / {all.length} 题</span>
                      <span>{training.maxAttempts > 0 ? `限答 ${training.maxAttempts} 次` : '不限次'}</span>
                      {!item.solved && item.locked && (
                        <span className="inline-flex items-center gap-1 text-red-600">已达上限，可回顾</span>
                      )}
                      <span className="ml-auto">
                        <ZoomControls scale={statementScale} onChange={setStatementScale} />
                      </span>
                    </div>
                    <div style={{ zoom: statementScale }}>
                      <ObjectiveCard
                        key={`${item.problemId}`}
                        sid={sid} tid={tid} item={item} maxAttempts={training.maxAttempts}
                        onAnswered={invalidate}
                      />
                    </div>
                  </PageContainer>
                </div>
              ) : (
                <SplitPane
                  left={
                    <PageContainer className="py-4">
                      {/* 题目头（题面文字缩放控件置右） */}
                      <div className="mb-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
                        <span className="rounded bg-muted px-1.5 py-0.5 font-medium">{item.chapterTitle}</span>
                        <span>第 {activeIdx + 1} / {all.length} 题</span>
                        <span className="ml-auto">
                          <ZoomControls scale={statementScale} onChange={setStatementScale} />
                        </span>
                      </div>
                      <div style={{ zoom: statementScale }}>
                        <ProgrammingStatement problemId={item.problemId} itemSolved={itemSolved} />
                      </div>
                    </PageContainer>
                  }
                  right={
                    <div className="flex h-full min-h-0 flex-col bg-background p-3">
                      <div className="mb-2 flex shrink-0 items-center gap-1.5 text-xs font-medium text-muted-foreground">
                        {itemSolved && (
                          <span className="inline-flex items-center gap-1 text-emerald-600">
                            <CircleCheckBigIcon className="size-3.5" /> 已通过
                          </span>
                        )}
                        代码编辑器
                      </div>
                      <div className="min-h-0 flex-1">
                        <TrainingProgrammingCard
                          key={`${item.problemId}`}
                          problemId={item.problemId}
                          trainingId={tid}
                          solved={itemSolved}
                          onSolved={invalidate}
                        />
                      </div>
                    </div>
                  }
                />
              ))}
          </div>
        </div>

        {/* 底部导航条 */}
        {all.length > 1 && (
          <footer className="shrink-0 border-t bg-background">
            <div className="flex items-center justify-center gap-4 px-4 py-2">
              <Button variant="outline" size="sm" disabled={activeIdx === 0} onClick={() => go(activeIdx - 1)}>
                <ChevronLeftIcon className="mr-1 size-3.5" /> 上一题
              </Button>
              <span className="text-xs text-muted-foreground">{activeIdx + 1} / {all.length}</span>
              <Button variant="outline" size="sm" disabled={activeIdx >= all.length - 1} onClick={() => go(activeIdx + 1)}>
                下一题 <ChevronRightIcon className="ml-1 size-3.5" />
              </Button>
            </div>
          </footer>
        )}
      </div>

      {/* 移动端导航浮窗（章节分组常显） */}
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
    </SpacePageShell>
  )
}

// ---------- 编程题题面（题干/格式/样例 完整展示） ----------

function ProgrammingStatement({ problemId, itemSolved }: { problemId: number; itemSolved: boolean }) {
  const q = useQuery({
    queryKey: ['oj-problem', problemId],
    queryFn: () => api.ojProblem(problemId),
    retry: 1,
  })
  if (q.isLoading) {
    return <div className="flex items-center gap-2 py-8 text-sm text-muted-foreground"><Loader2Icon className="size-4 animate-spin" /> 加载题面…</div>
  }
  if (q.isError || !q.data) {
    return <p className="py-6 text-center text-sm text-muted-foreground">题面加载失败（{q.error instanceof Error ? q.error.message : '未知错误'}）</p>
  }
  const p = q.data
  const body = p.bodyJson as { inputFormat?: string; outputFormat?: string; samples?: { input?: string; output?: string }[] }
  return (
    <div className="space-y-3 rounded-2xl border bg-card p-4 md:p-5">
      <div className="flex flex-wrap items-center gap-2">
        <h1 className="flex min-w-0 flex-1 items-center gap-2 text-lg font-semibold">
          <Code2Icon className="size-5 shrink-0 text-primary" />
          <span className="min-w-0 truncate">{p.title}</span>
        </h1>
        {itemSolved && (
          <span className="inline-flex items-center gap-1 text-xs text-emerald-600">
            <CircleCheckBigIcon className="size-4" /> 已通过
          </span>
        )}
      </div>
      <div className="rounded-xl bg-muted/50 p-3">
        <Markdown text={preserveLineBreaks(p.statementMd || '（暂无题面）')} className="markdown-body text-[15px] leading-relaxed" />
      </div>
      {body.inputFormat && (
        <Section title="输入格式"><Markdown text={preserveLineBreaks(body.inputFormat)} className="markdown-body text-sm" /></Section>
      )}
      {body.outputFormat && (
        <Section title="输出格式"><Markdown text={preserveLineBreaks(body.outputFormat)} className="markdown-body text-sm" /></Section>
      )}
      {(body.samples ?? []).length > 0 && (
        <div>
          <div className="mb-1.5 text-sm font-semibold">样例</div>
          <div className="space-y-2">
            {(body.samples ?? []).map((s, i) => (
              <div key={i} className="grid grid-cols-1 gap-2 sm:grid-cols-2">
                <SampleBox label={`输入样例 ${i + 1}`} text={s.input ?? ''} />
                <SampleBox label={`输出样例 ${i + 1}`} text={s.output ?? ''} />
              </div>
            ))}
          </div>
        </div>
      )}
      <div className="text-xs text-muted-foreground">时间限制：{p.timeLimitMs} ms · 内存限制：{p.memoryLimitMiB} MiB</div>
    </div>
  )
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div>
      <div className="mb-1.5 text-sm font-semibold">{title}</div>
      {children}
    </div>
  )
}

function SampleBox({ label, text }: { label: string; text: string }) {
  return (
    <div className="rounded-lg border bg-muted/30">
      <div className="border-b px-2 py-1 text-[11px] font-medium text-muted-foreground">{label}</div>
      <pre className="overflow-x-auto px-2 py-1.5 font-mono text-xs whitespace-pre-wrap">{text}</pre>
    </div>
  )
}

// ---------- 客观题内嵌作答卡（先选答案 → 提交即判） ----------

// 本会话内判分结果记忆（切题/重进仍显示绿/红标；服务端不返回答案故仅会话级）
const lastResultCache = new Map<number, { selected: ObjectiveAnswer; feedback: { correct: boolean; correctAnswer?: CorrectAnswer } }>()

function ObjectiveCard({ sid, tid, item, maxAttempts, onAnswered }: {
  sid: number
  tid: number
  item: TrainingItemView
  maxAttempts: number
  onAnswered: () => void
}) {
  const cached = lastResultCache.get(item.problemId)
  const [selected, setSelected] = useState<ObjectiveAnswer | null>(cached?.selected ?? null)
  const [feedback, setFeedback] = useState<{ correct: boolean; correctAnswer?: CorrectAnswer } | null>(cached?.feedback ?? null)
  const [busy, setBusy] = useState(false)

  const contentQ = useQuery({
    queryKey: ['oj-problem', item.problemId],
    queryFn: () => api.ojProblem(item.problemId),
    retry: 1,
  })

  const readOnly = item.solved || (item.locked && maxAttempts > 0 && item.attempts >= maxAttempts)
  // 回顾态：无本地判分但服务端带正确答案（此前答过）→ 静默标出正确项
  const reviewFeedback = readOnly && !feedback && item.correctAnswer
    ? ({ correct: false, correctAnswer: item.correctAnswer } as { correct: boolean; correctAnswer?: CorrectAnswer })
    : null
  const showFeedback = feedback ?? reviewFeedback
  const showSilent = reviewFeedback != null

  async function submit() {
    if (busy || feedback || readOnly || selected == null) return
    setBusy(true)
    try {
      const r = await api.portalTrainingAnswer(sid, tid, item.problemId, selected)
      const fb = { correct: r.correct, correctAnswer: r.correctAnswer }
      setFeedback(fb)
      lastResultCache.set(item.problemId, { selected, feedback: fb })
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
          feedback={showFeedback}
          silent={showSilent}
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

// 题面文字缩放控制（放大/缩小/重置；作用于整块题面 zoom）。
function ZoomControls({ scale, onChange }: { scale: number; onChange: (s: number) => void }) {
  const step = 0.1
  return (
    <span className="flex shrink-0 items-center gap-0.5 rounded-lg border bg-background px-1 py-0.5">
      <button
        type="button"
        title="缩小题目文字"
        className="flex size-5 items-center justify-center rounded text-muted-foreground transition-colors hover:bg-muted hover:text-foreground disabled:opacity-40"
        disabled={scale <= 0.7}
        onClick={() => onChange(Math.round((scale - step) * 100) / 100)}
      >
        <ZoomOutIcon className="size-3.5" />
      </button>
      <span className="w-9 text-center text-[10px] tabular-nums">{Math.round(scale * 100)}%</span>
      <button
        type="button"
        title="放大题目文字"
        className="flex size-5 items-center justify-center rounded text-muted-foreground transition-colors hover:bg-muted hover:text-foreground disabled:opacity-40"
        disabled={scale >= 2}
        onClick={() => onChange(Math.round((scale + step) * 100) / 100)}
      >
        <ZoomInIcon className="size-3.5" />
      </button>
      <button
        type="button"
        title="重置题目文字大小"
        className="flex size-5 items-center justify-center rounded text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
        onClick={() => onChange(1)}
      >
        <RotateCcwIcon className="size-3" />
      </button>
    </span>
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
