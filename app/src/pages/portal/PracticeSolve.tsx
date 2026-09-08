import { createContext, useContext, useRef, useState, type ReactNode } from 'react'
import { Link, useParams } from 'react-router-dom'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  HistoryIcon, LayoutGridIcon, Loader2Icon, SaveIcon, SendIcon,
} from 'lucide-react'
import { toast } from 'sonner'

import { api } from '@/api'
import type {
  ObjectiveAnswer, PracticeDetail, PracticeResultItem,
} from '@/api/types'
import { OPTION_LABELS } from '@/components/portal/objective'
import { Button } from '@/components/ui/button'
import {
  AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription,
  AlertDialogFooter, AlertDialogHeader, AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import {
  Dialog, DialogContent, DialogHeader, DialogTitle,
} from '@/components/ui/dialog'
import { Markdown, preserveLineBreaks } from '@/lib/markdown'
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
    <PracticeProvider key={pid} sid={sid} pid={pid} data={data}>
      <SpacePageShell
        spaceId={sid}
        backTo={`/s/${sid}/practice`}
        backLabel="返回练习列表"
        headerExtra={<HeaderActionBar />}
      >
        <div className="h-full bg-[#eef2f7]">
          <PracticePaper sid={sid} pid={pid} data={data} />
        </div>
      </SpacePageShell>
    </PracticeProvider>
  )
}

// ---------- 练习整卷共享状态（外壳顶栏按钮 + 卷面/导航/对话框共用） ----------

type PracticeResult = {
  submissionId: number
  results: PracticeResultItem[]
  objectiveCorrect: number
  objectiveTotal: number
}

type PracticeCtxValue = {
  answers: Record<number, ObjectiveAnswer>
  toggleAnswer: (problemId: number, a: ObjectiveAnswer) => void
  answeredCount: number
  submitting: boolean
  result: PracticeResult | null
  submitPaper: () => void
  canSubmit: boolean
  dismissResult: () => void
  confirmOpen: boolean
  setConfirmOpen: (v: boolean) => void
  navOpen: boolean
  setNavOpen: (v: boolean) => void
  historyAllOpen: boolean
  setHistoryAllOpen: (v: boolean) => void
  openConfirm: () => void
  openHistory: () => void
  openNav: () => void
  /** 手动保存当前作答到本地草稿（顶栏「保存」） */
  saveNow: () => void
}

const PracticeCtx = createContext<PracticeCtxValue | null>(null)

function usePracticeCtx(): PracticeCtxValue {
  const v = useContext(PracticeCtx)
  if (!v) throw new Error('usePracticeCtx must be used within PracticeProvider')
  return v
}

function practiceDraftKey(sid: number, pid: number) {
  return `oj-practice-answers:${sid}:${pid}`
}

function loadPracticeDraft(sid: number, pid: number): Record<number, ObjectiveAnswer> {
  try {
    const raw = localStorage.getItem(practiceDraftKey(sid, pid))
    if (!raw) return {}
    const obj = JSON.parse(raw) as Record<string, number | boolean>
    const out: Record<number, ObjectiveAnswer> = {}
    for (const [k, v] of Object.entries(obj)) {
      if (typeof v === 'number' || typeof v === 'boolean') out[Number(k)] = v
    }
    return out
  } catch {
    return {}
  }
}

function PracticeProvider({ sid, pid, data, children }: {
  sid: number
  pid: number
  data: PracticeDetail
  children: ReactNode
}) {
  const { items: rawItems } = data
  const items = rawItems ?? []
  const objectiveItems = items.filter(
    (i) => i.problemType === 'single_choice' || i.problemType === 'true_false',
  )
  const qc = useQueryClient()

  // 每客观题已选题（单选=number，判断=boolean）；本地草稿持久化（刷新/重进不丢）
  const [answers, setAnswers] = useState<Record<number, ObjectiveAnswer>>(() =>
    loadPracticeDraft(sid, pid),
  )
  // answers 同步镜像（事件内即时读最新，供 toggleAnswer 快照式计算）
  const answersRef = useRef(answers)
  answersRef.current = answers
  const [confirmOpen, setConfirmOpen] = useState(false)
  const [navOpen, setNavOpen] = useState(false)
  const [historyAllOpen, setHistoryAllOpen] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  // submitting 同步 ref（toggleAnswer 事件内即时判定，避免 state 闭包延迟）
  const submittingRef = useRef(false)
  const [result, setResult] = useState<PracticeResult | null>(null)

  function persist(next: Record<number, ObjectiveAnswer>) {
    try {
      if (Object.keys(next).length === 0) localStorage.removeItem(practiceDraftKey(sid, pid))
      else localStorage.setItem(practiceDraftKey(sid, pid), JSON.stringify(next))
    } catch {
      // localStorage 不可用（隐私模式等）时忽略，仅内存作答
    }
  }

  function toggleAnswer(problemId: number, a: ObjectiveAnswer) {
    // 交卷进行中锁定作答（防与草稿清空/payload 快照竞态）
    if (submittingRef.current) return
    // 基于当前快照计算 next 并持久化（保持 updater 纯函数；单用户交互下无并发覆盖风险）
    const prev = answersRef.current
    const next = { ...prev }
    if (next[problemId] === a) delete next[problemId]
    else next[problemId] = a
    answersRef.current = next
    setAnswers(next)
    persist(next)
  }

  async function doSubmit() {
    setSubmitting(true)
    submittingRef.current = true
    try {
      const payload = objectiveItems.map((it) => ({
        problemId: it.problemId,
        answer: answers[it.problemId],
        uuid: it.problemUuid,
      })).filter((a) => a.answer !== undefined)
      const r = await api.portalPracticeSubmit(sid, pid, payload)
      setResult(r)
      // 交卷后清空本地草稿（已提交内容入历史）
      localStorage.removeItem(practiceDraftKey(sid, pid))
      void qc.invalidateQueries({ queryKey: ['portal-practice-submissions', sid, pid] })
      toast.success(`交卷成功：答对 ${r.objectiveCorrect}/${r.objectiveTotal}`)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '交卷失败')
    } finally {
      setSubmitting(false)
      submittingRef.current = false
      setConfirmOpen(false)
    }
  }

  const answeredCount = Object.keys(answers).length
  const canSubmit = !submitting && answeredCount > 0 && !result

  const value: PracticeCtxValue = {
    answers,
    toggleAnswer,
    answeredCount,
    submitting,
    result,
    submitPaper: () => void doSubmit(),
    canSubmit,
    dismissResult: () => setResult(null),
    confirmOpen,
    setConfirmOpen,
    navOpen,
    setNavOpen,
    historyAllOpen,
    setHistoryAllOpen,
    openConfirm: () => setConfirmOpen(true),
    openHistory: () => setHistoryAllOpen(true),
    openNav: () => setNavOpen(true),
    saveNow: () => {
      if (result) {
        toast.info('已交卷，作答已提交；如需重做请进入新一轮')
        return
      }
      persist(answers)
      toast.success(`已保存 ${answeredCount} 道作答到本地（刷新不丢失）`)
    },
  }

  return <PracticeCtx.Provider value={value}>{children}</PracticeCtx.Provider>
}

// ---------- 外壳顶栏右侧操作按钮（全部提交记录 / 保存 / 提交；移动端含「导航」） ----------

function HeaderActionBar() {
  const { submitting, canSubmit, openConfirm, openHistory, openNav, saveNow } = usePracticeCtx()
  return (
    <div className="flex shrink-0 items-center gap-1.5">
      <Button
        variant="outline"
        size="sm"
        className="md:hidden"
        onClick={openNav}
      >
        <LayoutGridIcon className="size-3.5" />
        导航
      </Button>
      <Button
        variant="outline"
        size="sm"
        className="text-muted-foreground"
        onClick={openHistory}
      >
        <HistoryIcon className="size-3.5" />
        <span className="hidden md:inline">全部提交记录</span>
      </Button>
      <Button
        variant="secondary"
        size="sm"
        className="text-muted-foreground"
        title="将当前作答保存到本地草稿（刷新不丢失；修改会自动保存）"
        onClick={saveNow}
      >
        <SaveIcon className="size-3.5" />
        <span className="hidden sm:inline">保存</span>
      </Button>
      <Button
        size="sm"
        className="min-w-[4.75rem] bg-orange-500 text-white hover:bg-orange-600 focus-visible:ring-orange-500/30"
        disabled={!canSubmit}
        onClick={openConfirm}
      >
        {submitting ? <Loader2Icon className="size-3.5 animate-spin" /> : <SendIcon className="size-3.5" />}
        {submitting ? '提交中' : '提交'}
      </Button>
    </div>
  )
}

const CN_NUM = ['一', '二', '三', '四', '五', '六', '七', '八']

function PracticePaper({ sid, pid, data }: {
  sid: number
  pid: number
  data: PracticeDetail
}) {
  const { items: rawItems } = data
  const items = rawItems ?? []
  const objectiveItems = items.filter((i) => i.problemType === 'single_choice' || i.problemType === 'true_false')
  const {
    answers, toggleAnswer, answeredCount, result, submitPaper, dismissResult,
    confirmOpen, setConfirmOpen, navOpen, setNavOpen,
    historyAllOpen, setHistoryAllOpen,
  } = usePracticeCtx()

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

  const progCount = items.length - objectiveItems.length

  return (
    <div className="flex h-full min-h-0 flex-col">
      {/* 内容行：左栏固定 + 右栏唯一内容滚动区 */}
      <div className="mx-auto flex min-h-0 w-full max-w-6xl flex-1">
        {/* 左栏：我的提交记录 + 题号导航（自身随内容滚动，不随右栏滚动） */}
        <aside className="hidden w-56 shrink-0 overflow-y-auto p-2.5 md:block">
          <div className="space-y-3">
            <div className="rounded-xl border bg-card p-3 shadow-sm">
              <HistoryCard sid={sid} pid={pid} />
            </div>
            <div className="rounded-xl border bg-card p-3 shadow-sm">
              <PaperNav
                sections={navSections}
                verdictOf={verdictOf}
                answeredCount={answeredCount}
                objectiveTotal={objectiveItems.length}
                onJump={jumpTo}
              />
            </div>
          </div>
        </aside>

        {/* 右栏：卷面（唯一内容滚动区） */}
        <div className="min-h-0 flex-1 overflow-y-auto">
          <div className="w-full px-3 pt-3 pb-4">
            {/* 交卷结果（横向通栏） */}
            {result && (
              <ResultPanel
                result={result}
                items={objectiveItems}
                onDismiss={dismissResult}
              />
            )}

            {/* 题目分组节卡 */}
            <div className={cn('space-y-3', result && 'mt-3')}>
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
                  <section key={g.key} className="overflow-hidden rounded-xl border bg-card shadow-sm">
                    <div className="border-b px-4 pt-3 pb-2 text-xs font-bold text-muted-foreground">
                      {sectionTitles[gi]}
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
          </div>
        </div>
      </div>

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

      {/* 全部提交记录 */}
      <PracticeHistoryDialog sid={sid} pid={pid} open={historyAllOpen} onClose={() => setHistoryAllOpen(false)} />

      {/* 交卷确认 */}
      <AlertDialog open={confirmOpen} onOpenChange={setConfirmOpen}>
        <AlertDialogContent size="default">
          <AlertDialogHeader>
            <AlertDialogTitle>确认提交？</AlertDialogTitle>
            <AlertDialogDescription>
              已作答客观题 {answeredCount}/{objectiveItems.length} 题（{objectiveItems.length - answeredCount > 0 ? `尚有 ${objectiveItems.length - answeredCount} 题未作答；` : ''}{progCount > 0 ? '编程题请到做题页提交，不计入本次卷面；' : ''}提交后立即评分并记录，可再次提交重做）。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel onClick={() => setConfirmOpen(false)}>再检查一下</AlertDialogCancel>
            <AlertDialogAction onClick={() => void submitPaper()}>确认提交</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
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
    <div className="space-y-3.5">
      <div className="flex items-center justify-between gap-2">
        <span className="text-xs font-bold text-muted-foreground">题目导航</span>
        <span className="text-[11px] text-muted-foreground tabular-nums">已答 {answeredCount}/{objectiveTotal}</span>
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
                          ? 'border-red-300 bg-red-50 text-red-600'
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
  const answered = verdict === 'correct' || verdict === 'wrong'
  const wrong = verdict === 'wrong'
  // 标准形式正确项：单选=索引，判断=boolean（单选/判断统一比较）
  const correctVal: ObjectiveAnswer | null = answered && resultItem ? objectiveAnswerOf(resultItem) : null
  const readOnly = answered

  return (
    <div id={`pq-${item.problemId}`} className="scroll-mt-36 p-4">
      <div className="flex items-start gap-2">
        <span className="min-w-[1.6rem] text-right text-[16px] font-bold leading-[26px] tabular-nums text-foreground">{no}.</span>
        <div className="min-w-0 flex-1">
          {verdict !== 'idle' && verdict !== 'answered' && (
            <div className="mb-1.5"><VerdictChip verdict={verdict} /></div>
          )}
          {contentQ.isLoading && <p className="py-4 text-center text-xs text-muted-foreground">题目加载中…</p>}
          {contentQ.isError && (
            <p className="py-4 text-center text-xs text-muted-foreground">
              题目内容不可见（{contentQ.error instanceof Error ? contentQ.error.message : '加载失败'}）
            </p>
          )}
          {contentQ.data && (
            <>
              <Markdown
                text={preserveLineBreaks(contentQ.data.statementMd || '（暂无题面）')}
                className="markdown-body text-[16px] md-line"
              />
              <div className="mt-2">
                {contentQ.data.type === 'single_choice'
                  ? (contentQ.data.bodyJson.options ?? []).map((opt, i) => (
                    <PracticeRadioOption
                      key={i}
                      type="choice"
                      label={OPTION_LABELS[i] ?? String(i + 1)}
                      text={opt}
                      disabled={readOnly}
                      selected={selected === i}
                      correct={answered && correctVal === i}
                      wrongPick={wrong && selected === i && correctVal !== i}
                      dimmed={answered && correctVal !== i && selected !== i}
                      onSelect={() => onToggle(i)}
                    />
                  ))
                  : ([true, false] as const).map((v) => (
                    <PracticeRadioOption
                      key={String(v)}
                      type="judge"
                      label=""
                      text={v ? '正确' : '错误'}
                      disabled={readOnly}
                      selected={selected === v}
                      correct={answered && correctVal === v}
                      wrongPick={wrong && selected === v && correctVal !== v}
                      dimmed={answered && correctVal !== v && selected !== v}
                      onSelect={() => onToggle(v)}
                    />
                  ))}
              </div>
              {wrong && resultItem && (
                <p className="mt-2.5 rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-xs text-red-600">
                  本题回答错误，正确答案：{correctAnswerText(resultItem)}
                </p>
              )}
            </>
          )}
        </div>
      </div>
    </div>
  )
}

// ---------- 客观题 radio 选项（练习页就地作答；单选=字母行，判断=正确/错误行） ----------

function PracticeRadioOption({ type, label, text, disabled, selected, correct, wrongPick, dimmed, onSelect }: {
  type: 'choice' | 'judge'
  label: string
  text: string
  disabled?: boolean
  selected?: boolean
  correct?: boolean
  wrongPick?: boolean
  dimmed?: boolean
  onSelect: () => void
}) {
  const ringCls = cn(
    'mt-[3px] flex size-4 shrink-0 items-center justify-center rounded-full border-2 transition-colors',
    correct
      ? 'border-emerald-500 bg-emerald-50'
      : wrongPick
        ? 'border-red-500 bg-red-50'
        : selected
          ? 'border-orange-500 bg-orange-50'
          : 'border-muted-foreground/40 group-hover:border-orange-400/70',
  )
  return (
    <button
      type="button"
      disabled={disabled}
      onClick={onSelect}
      aria-checked={!!selected}
      role="radio"
      className={cn(
        'group flex w-full items-start gap-2.5 rounded-md py-1 text-left text-sm transition-colors disabled:pointer-events-none',
        disabled ? 'cursor-default' : 'hover:bg-orange-50/50',
        selected && !disabled && 'bg-orange-50/30',
        dimmed && 'opacity-50',
      )}
    >
      <span className={ringCls}>
        {(selected || correct || wrongPick) && (
          <span
            className={cn(
              'size-2 rounded-full',
              correct ? 'bg-emerald-500' : wrongPick ? 'bg-red-500' : 'bg-orange-500',
            )}
          />
        )}
      </span>
      {type === 'choice' && (
        <span className={cn('flex w-[1.5em] flex-none items-center justify-center text-sm font-semibold leading-normal', txtCls(correct, wrongPick, selected))}>
          {label}.
        </span>
      )}
      <span className="min-w-0 flex-1">
        {type === 'judge' ? (
          <span className={cn('font-medium leading-normal', txtCls(correct, wrongPick, selected))}>{text}</span>
        ) : (
          <Markdown text={preserveLineBreaks(text)} className="markdown-body text-sm leading-normal md-clean" />
        )}
      </span>
    </button>
  )
}

function txtCls(correct?: boolean, wrongPick?: boolean, selected?: boolean): string {
  if (correct) return 'text-emerald-700'
  if (wrongPick) return 'text-red-600'
  if (selected) return 'text-orange-600'
  return 'text-foreground'
}

// 交卷结果里的正确项 → 单选=索引 number / 判断=boolean（整卷统一为标准形式）
function objectiveAnswerOf(r: PracticeResultItem): ObjectiveAnswer | null {
  const ca = r.correctAnswer
  if (r.type === 'true_false') {
    if (typeof ca === 'boolean') return ca
    if (ca && typeof ca === 'object' && 'answer' in ca && typeof ca.answer === 'boolean') return ca.answer
    return null
  }
  if (typeof ca === 'number') return ca
  if (ca && typeof ca === 'object' && 'answerIndex' in ca && typeof ca.answerIndex === 'number') return ca.answerIndex
  return null
}

function VerdictChip({ verdict }: { verdict: Extract<Verdict, 'correct' | 'wrong' | 'missing'> }) {
  return (
    <span
      className={cn(
        'inline-flex items-center rounded-md border px-1.5 py-0.5 text-[11px] font-semibold',
        verdict === 'correct' ? 'border-emerald-300 bg-emerald-50 text-emerald-700'
          : verdict === 'wrong' ? 'border-red-300 bg-red-50 text-red-600'
            : 'border-border bg-muted text-muted-foreground',
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
    <div id={`pq-${item.problemId}`} className="scroll-mt-36 p-4">
      <div className="flex flex-wrap items-center justify-between gap-x-3 gap-y-2">
        <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
          <span className="text-sm font-bold tabular-nums text-foreground">{no}.</span>
          <span className="text-sm font-semibold">{item.problemTitle || `题目 #${item.problemId}`}</span>
        </div>
        <Link
          to={`/problem/${item.problemId}?practiceId=${pid}&back=${encodeURIComponent(`/s/${sid}/practice/${pid}`)}`}
          className="inline-flex shrink-0 items-center gap-1.5 rounded-lg bg-orange-500 px-3 py-1.5 text-xs font-semibold text-white transition-colors hover:bg-orange-600"
        >
          进入编程
        </Link>
      </div>
    </div>
  )
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
        'flex flex-wrap items-center justify-between gap-x-4 gap-y-2 rounded-xl border px-4 py-3 shadow-sm',
        perfect
          ? 'border-emerald-200 bg-emerald-50/60'
          : wrong.length > 0
            ? 'border-red-200 bg-red-50/50'
            : 'border-amber-200 bg-amber-50/50',
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
          {wrong.length > 0 && <ResultStat label={`答错 ${wrong.length}`} className="border-red-300 bg-red-50 text-red-600" />}
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

// 左栏「我的提交记录」卡（最近记录；超过 3 条后区域内滚动）
function HistoryCard({ sid, pid }: { sid: number; pid: number }) {
  const q = useQuery({
    queryKey: ['portal-practice-submissions', sid, pid],
    queryFn: () => api.portalPracticeSubmissions(sid, pid),
  })
  const list = q.data?.submissions ?? []
  return (
    <div>
      <div className="flex items-center justify-between gap-2">
        <span className="text-xs font-bold text-muted-foreground">我的提交记录</span>
        {list.length > 0 && <span className="text-[10px] tabular-nums text-muted-foreground/80">共 {list.length} 次</span>}
      </div>
      {q.isLoading && <p className="py-2.5 text-center text-[11px] text-muted-foreground">加载中…</p>}
      {!q.isLoading && list.length === 0 && (
        <p className="py-2.5 text-[11px] text-muted-foreground">暂无提交记录，交卷后显示在此</p>
      )}
      {list.length > 0 && (
        <>
          <div className={cn('mt-1', list.length > 3 && 'max-h-[96px] overflow-y-auto pr-0.5')}>
            {list.map((s) => (
              <Link
                key={s.id}
                to={`/s/${sid}/practice/${pid}/record/${s.id}`}
                title="点击查看该次答题卡"
                className="flex h-8 items-center gap-2 rounded-md py-1.5 pr-1 text-xs transition-colors hover:bg-orange-50/60"
              >
                <span className="flex size-5 shrink-0 items-center justify-center rounded-full bg-orange-50 text-[10px] font-bold tabular-nums text-orange-600">
                  {s.objectiveCorrect}
                </span>
                <span className="min-w-0 truncate text-muted-foreground">答对 {s.objectiveCorrect} 题</span>
                <span className="ml-auto shrink-0 tabular-nums text-muted-foreground/80">{formatTime(s.createdAt)}</span>
              </Link>
            ))}
          </div>
        </>
      )}
    </div>
  )
}

// 全部提交记录（Dialog 全量列表；左栏快捷卡 + 顶栏「全部提交记录」共用）
function PracticeHistoryDialog({ sid, pid, open, onClose }: { sid: number; pid: number; open: boolean; onClose: () => void }) {
  const q = useQuery({
    queryKey: ['portal-practice-submissions', sid, pid],
    queryFn: () => api.portalPracticeSubmissions(sid, pid),
  })
  const list = q.data?.submissions ?? []
  return (
    <Dialog open={open} onOpenChange={(v) => !v && onClose()}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>全部提交记录</DialogTitle>
        </DialogHeader>
        <div className="max-h-[60vh] overflow-y-auto">
          {q.isLoading && <p className="py-6 text-center text-xs text-muted-foreground">加载中…</p>}
          {!q.isLoading && list.length === 0 && (
            <p className="py-6 text-center text-xs text-muted-foreground">暂无提交记录</p>
          )}
          {list.map((s) => (
            <Link
              key={s.id}
              to={`/s/${sid}/practice/${pid}/record/${s.id}`}
              className="flex items-center gap-3 border-b px-2 py-2.5 text-xs transition-colors last:border-b-0 hover:bg-orange-50/50"
            >
              <span className="tabular-nums text-muted-foreground">#{s.id}</span>
              <span className="font-medium text-emerald-600">答对 {s.objectiveCorrect} 题</span>
              <span className="ml-auto tabular-nums text-muted-foreground">{formatTime(s.createdAt)}</span>
              <span className="text-[10px] text-orange-600">查看答题卡 →</span>
            </Link>
          ))}
        </div>
      </DialogContent>
    </Dialog>
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
