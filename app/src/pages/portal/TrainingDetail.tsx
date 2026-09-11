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
} from 'lucide-react'
import { toast } from 'sonner'

import { api } from '@/api'
import type { CorrectAnswer, ObjectiveAnswer, TrainingItemView } from '@/api/types'
import { usePortalSession } from '@/pages/portal/portal-context'
import { TrainingProgrammingCard } from '@/components/portal/TrainingProgrammingCard'
import { ZoomControls } from '@/components/portal/zoom-controls'
import { EditorCollapseButton } from '@/components/portal/editor-collapse-button'
import { AdminEditProblemButton } from '@/components/portal/admin-edit-problem'
import { ViewSolutionButton } from '@/components/portal/view-solution-button'
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
  // 折叠「代码编辑器 + 控制台」：题面占满（编程题专用；仅隐藏不卸载）
  const [editorCollapsed, setEditorCollapsed] = useState(false)

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
                              // 绿=已通过（任何题型；通过后再次答错/提交错误保持绿，solved 不回退）
                              it.solved
                                ? 'border-green-400 bg-green-50 text-green-700 hover:bg-green-100'
                                // 红=客观题达上限仍未通过
                                : !it.solved && it.locked && obj
                                  ? 'border-red-300 bg-red-50 text-red-600 hover:bg-red-100'
                                  : 'border-border bg-background hover:border-primary hover:bg-slate-50',
                              isCurrent && 'ring-2 ring-primary ring-offset-1',
                            )}
                          >
                            {/* 编号按章节重新从 1 开始 */}
                            {j + 1}
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
                <div className="flex h-full min-h-0 flex-col">
                  {/* 题目头固定（编辑/缩放按钮不随正文滚动条移动） */}
                  <div className="mb-2 flex shrink-0 flex-wrap items-center gap-x-3 gap-y-1 px-4 pt-4 text-xs text-muted-foreground lg:px-8">
                    <span className="rounded bg-muted px-1.5 py-0.5 font-medium">{item.chapterTitle}</span>
                    <span>第 {activeIdx + 1} / {all.length} 题</span>
                    <span>{training.maxAttempts > 0 ? `限答 ${training.maxAttempts} 次` : '不限次'}</span>
                    {!item.solved && item.locked && (
                      <span className="inline-flex items-center gap-1 text-red-600">已达上限，可回顾</span>
                    )}
                    <span className="ml-auto flex items-center gap-1">
                      <AdminEditProblemButton problemId={item.problemId} />
                      <ZoomControls scale={statementScale} onChange={setStatementScale} />
                    </span>
                  </div>
                  {/* 正文独立滚动（滚动条只在正文区出现，头行不受宽度变化影响） */}
                  <div className="min-h-0 flex-1 overflow-y-auto [scrollbar-gutter:stable]">
                    <PageContainer className="py-2">
                      <div style={{ zoom: statementScale }}>
                        <ObjectiveCard
                          key={`i${item.id}`}
                          sid={sid} tid={tid} item={item} maxAttempts={training.maxAttempts}
                          onAnswered={invalidate}
                        />
                      </div>
                    </PageContainer>
                  </div>
                </div>
              ) : (
                <SplitPane
                  collapsed={editorCollapsed}
                  left={
                    <div className="h-full min-h-0 w-full overflow-y-auto px-4 py-4 [scrollbar-gutter:stable] lg:px-5">
                      {/* 无独立头栏：编辑/缩放/折叠/已通过 均集成在题目卡内部标题行；
                          缩放仅作用于卡内正文（标题行与按钮大小/位置固定——与练习做题页一致） */}
                      <ProgrammingStatement
                        problemId={item.problemId}
                        itemSolved={itemSolved}
                        scale={statementScale}
                        onScale={setStatementScale}
                        editorCollapsed={editorCollapsed}
                        onToggleEditor={() => setEditorCollapsed((v) => !v)}
                      />
                    </div>
                  }
                  right={
                    <div className="flex h-full min-h-0 flex-col bg-background px-4 py-4 lg:px-5">
                      <div className="min-h-0 flex-1">
                        <TrainingProgrammingCard
                          key={`i${item.id}`}
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
                            it.solved ? 'bg-emerald-500 text-white'
                              : !it.solved && it.locked && obj ? 'bg-red-500 text-white'
                                : !obj ? 'border border-dashed bg-muted/40 text-muted-foreground'
                                  : 'bg-muted text-muted-foreground hover:bg-muted/70',
                          )}
                        >
                          {/* 编号按章节重新从 1 开始 */}
                          {j + 1}
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

function ProgrammingStatement({ problemId, itemSolved, scale, onScale, editorCollapsed, onToggleEditor }: {
  problemId: number
  itemSolved: boolean
  scale: number
  onScale: (s: number) => void
  editorCollapsed: boolean
  onToggleEditor: () => void
}) {
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
          {itemSolved && (
            <span className="inline-flex shrink-0 items-center gap-1 text-xs text-emerald-600">
              <CircleCheckBigIcon className="size-4" /> 已通过
            </span>
          )}
        </h1>
        {/* 管理员：查看题解 + 编辑题目 + 文字缩放 + 折叠编辑器（题解在编辑左侧，折叠在最右） */}
        <span className="flex shrink-0 items-center gap-1">
          <ViewSolutionButton problemId={problemId} />
          <AdminEditProblemButton problemId={problemId} />
          <ZoomControls scale={scale} onChange={onScale} />
          <EditorCollapseButton collapsed={editorCollapsed} onToggle={onToggleEditor} />
        </span>
      </div>
      <div style={{ zoom: scale }} className="space-y-3">
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

// 本会话内判分结果记忆（切题/重进回顾仍显示红绿；key 含 空间×训练×题，
// 避免同题跨训练/跨空间串用旧判定——且仅回顾态（已通过/达限）读取，防止阻塞继续作答）
const lastResultCache = new Map<string, { selected: ObjectiveAnswer; feedback: ObjFeedback }>()

type ObjFeedback = { correct: boolean; correctAnswer?: CorrectAnswer; wrongHint?: string }

function ObjectiveCard({ sid, tid, item, maxAttempts, onAnswered }: {
  sid: number
  tid: number
  item: TrainingItemView
  maxAttempts: number
  onAnswered: () => void
}) {
  const readOnly = item.solved || (item.locked && maxAttempts > 0 && item.attempts >= maxAttempts)
  const { user: curUser } = usePortalSession()
  const cacheKey = `${curUser?.id ?? 0}:${sid}:${tid}:${item.problemId}`
  // 缓存恢复规则：回顾态（已通过/达限）或 缓存为答错（未锁定）时恢复判定展示——
  // 答错未达限时切走再回来仍可见错题反馈并可直接再答；已答对的未锁定态不恢复
  // （避免复用"正确"判定造成无法继续作答/重复提交的卡死窗口）。
  const cachedRaw = lastResultCache.get(cacheKey)
  const cached = readOnly || (cachedRaw && !cachedRaw.feedback.correct) ? cachedRaw : undefined
  const [selected, setSelected] = useState<ObjectiveAnswer | null>(cached?.selected ?? null)
  const [feedback, setFeedback] = useState<ObjFeedback | null>(cached?.feedback ?? null)
  const [busy, setBusy] = useState(false)

  const contentQ = useQuery({
    queryKey: ['oj-problem', item.problemId],
    queryFn: () => api.ojProblem(item.problemId),
    retry: 1,
  })

  // 回顾态：无本地判分但服务端带正确答案（此前答过）→ 静默标出正确项
  const reviewFeedback = readOnly && !feedback && item.correctAnswer
    ? ({ correct: false, correctAnswer: item.correctAnswer } as ObjFeedback)
    : null
  const showFeedback = feedback ?? reviewFeedback
  const showSilent = reviewFeedback != null

  async function submit() {
    if (busy || feedback || readOnly || selected == null) return
    setBusy(true)
    try {
      const r = await api.portalTrainingAnswer(sid, tid, item.problemId, selected)
      // 未达尝试上限时服务端不下发正确答案 → 只提示对错与剩余次数
      const wrongHint = !r.correct && !r.locked && maxAttempts > 0
        ? `还可再答 ${(r as { remaining?: number }).remaining ?? Math.max(0, maxAttempts - r.attempts)} 次`
        : undefined
      const fb: ObjFeedback = { correct: r.correct, correctAnswer: r.correctAnswer, wrongHint }
      setFeedback(fb)
      lastResultCache.set(cacheKey, { selected, feedback: fb })
      if (r.correct) toast.success('回答正确')
      else if (r.locked) toast.error('回答错误，次数已用尽，正确答案已显示')
      else toast.error('回答错误，请重试')
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
          wrongHint={feedback?.wrongHint}
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
