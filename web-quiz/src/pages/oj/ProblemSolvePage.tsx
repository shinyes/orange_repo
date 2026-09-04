import { useEffect, useRef, useState } from 'react'
import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import {
  ArrowLeftIcon,
  CheckCircle2Icon,
  CopyIcon,
  HistoryIcon,
  Loader2Icon,
  PlayIcon,
  SendIcon,
  XCircleIcon,
  FlaskConicalIcon,
} from 'lucide-react'
import { toast } from 'sonner'
import { useQuery, useQueryClient } from '@tanstack/react-query'

import { api } from '@/lib/api'
import type { CaseDetail, CodeLang, OjProblem, Submission, SubmissionPoll } from '@/lib/types'
import { Markdown, preserveLineBreaks } from '@/lib/markdown'
import { CodeBlock } from '@/lib/code-highlight'
import { CodeEditor } from '@/components/CodeEditor'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Textarea } from '@/components/ui/textarea'
import {
  Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle,
} from '@/components/ui/dialog'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { cn } from '@/lib/utils'
import { langLabel, starterCode, typeLabel, verdictCls, verdictText } from './oj-utils'

const DRAFT_KEY = 'oj-draft'

// 做题页：客观题内联作答 / 编程题 运行·测试·提交。
export function ProblemSolvePage() {
  const { problemId } = useParams()
  const [searchParams] = useSearchParams()
  const navigate = useNavigate()
  const pid = Number(problemId)
  const backTo = searchParams.get('back') ?? '/training'

  const problemQ = useQuery({
    queryKey: ['oj-problem', pid],
    queryFn: () => api.ojProblem(pid),
    enabled: pid > 0,
  })
  const problem = problemQ.data

  if (problemQ.isLoading) return <Center>题目加载中…</Center>
  if (problemQ.isError || !problem) {
    return (
      <Center>
        <p className="text-sm text-muted-foreground">题目不存在或不可见</p>
        <Button variant="outline" className="mt-3" onClick={() => navigate(backTo)}>返回</Button>
      </Center>
    )
  }
  return problem.type === 'programming' ? (
    <ProgrammingSolve key={pid} problem={problem} backTo={backTo} />
  ) : (
    <ObjectiveSolve key={pid} problem={problem} backTo={backTo} />
  )
}

function Center({ children }: { children: React.ReactNode }) {
  return <div className="flex h-full flex-col items-center justify-center px-4 text-center">{children}</div>
}

// ---------------- 客观题 ----------------

interface ObjSubmitResult {
  correct: boolean
  verdict: string
  correctAnswer?: { answerIndex?: number; answer?: boolean }
}

function ObjectiveSolve({ problem, backTo }: { problem: OjProblem; backTo: string }) {
  const navigate = useNavigate()
  const qc = useQueryClient()
  const [picked, setPicked] = useState<number | null>(null)
  const [pickedTF, setPickedTF] = useState<boolean | null>(null)
  const [result, setResult] = useState<ObjSubmitResult | null>(null)
  const [busy, setBusy] = useState(false)
  const options = (problem.bodyJson.options as string[] | undefined) ?? []

  async function submit(payload: { optionIndex?: number; answer?: boolean }) {
    if (busy || result) return
    setBusy(true)
    try {
      const r = await api.ojObjectiveSubmit(problem.id, payload.optionIndex ?? payload.answer ?? false)
      setResult({ correct: r.correct, verdict: r.verdict, correctAnswer: r.correctAnswer })
      void qc.invalidateQueries({ queryKey: ['oj-assigned'] })
      void qc.invalidateQueries({ queryKey: ['oj-training'] })
      void qc.invalidateQueries({ queryKey: ['oj-practice'] })
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '提交失败')
    } finally {
      setBusy(false)
    }
  }

  const answered = result !== null
  const rightIdx = answered && !result.correct ? (result.correctAnswer?.answerIndex ?? -1) : -1
  const rightTF = answered && !result.correct ? (result.correctAnswer?.answer ?? null) : null
  return (
    <div className="mx-auto w-full max-w-2xl px-4 py-5 lg:max-w-3xl lg:px-8 lg:py-8">
      <TopBar backTo={backTo} problem={problem} />
      <div className="mt-3 rounded-2xl border bg-card p-5">
        <div className="mb-3 flex items-center gap-2">
          <Badge>{typeLabel(problem.type)}</Badge>
          <h2 className="min-w-0 flex-1 truncate text-base font-semibold">{problem.title}</h2>
        </div>
        <Markdown text={preserveLineBreaks(problem.statementMd || '（暂无题面）')} className="markdown-body text-[17px] leading-relaxed" />

        {problem.type === 'single_choice' ? (
          <div className="mt-5 space-y-2.5">
            {options.map((opt, i) => {
              const isPick = picked === i
              const isRight = answered && i === rightIdx
              const isWrongPick = answered && !result.correct && isPick
              const isRightPick = answered && result.correct && isPick
              return (
                <button
                  key={i}
                  type="button"
                  disabled={answered || busy}
                  onClick={() => { setPicked(i); void submit({ optionIndex: i }) }}
                  className={cn(
                    'flex w-full items-start gap-2.5 rounded-xl border p-3 text-left text-sm transition-colors',
                    !answered && 'hover:border-primary/60 hover:bg-primary/5',
                    (isRightPick || isRight) && 'border-emerald-500 bg-emerald-50',
                    isWrongPick && 'border-red-500 bg-red-50',
                    answered && !(isRightPick || isRight) && !isWrongPick && 'opacity-50',
                  )}
                >
                  <span className={cn(
                    'mt-0.5 flex size-6 shrink-0 items-center justify-center rounded-full text-xs font-medium',
                    (isRightPick || isRight) ? 'bg-emerald-500 text-white' : isWrongPick ? 'bg-red-500 text-white' : 'bg-muted text-muted-foreground',
                  )}>
                    {String.fromCharCode(65 + i)}
                  </span>
                  <span className="min-w-0 flex-1"><Markdown text={preserveLineBreaks(opt)} className="markdown-body text-sm" /></span>
                  {(isRightPick || isRight) && <CheckCircle2Icon className="mt-0.5 size-4 shrink-0 text-emerald-600" />}
                  {isWrongPick && <XCircleIcon className="mt-0.5 size-4 shrink-0 text-red-600" />}
                </button>
              )
            })}
            {options.length === 0 && (
              <div className="rounded-lg border border-dashed p-4 text-center text-sm text-muted-foreground">题目选项缺失</div>
            )}
          </div>
        ) : (
          <div className="mt-5 grid grid-cols-2 gap-2.5">
            {[true, false].map((v) => {
              const isPick = pickedTF === v
              const isRight = answered && rightTF === v
              const isWrongPick = answered && !result.correct && isPick
              const isRightPick = answered && result.correct && isPick
              return (
                <button
                  key={String(v)}
                  type="button"
                  disabled={answered || busy}
                  onClick={() => { setPickedTF(v); void submit({ answer: v }) }}
                  className={cn(
                    'rounded-xl border p-4 text-base font-medium transition-colors',
                    !answered && 'hover:border-primary/60 hover:bg-primary/5',
                    (isRightPick || isRight) && 'border-emerald-500 bg-emerald-50 text-emerald-700',
                    isWrongPick && 'border-red-500 bg-red-50 text-red-700',
                    answered && !(isRightPick || isRight) && !isWrongPick && 'opacity-50',
                  )}
                >
                  {v ? '✓ 正确' : '✗ 错误'}
                </button>
              )
            })}
          </div>
        )}

        {answered && (
          <div className={cn(
            'mt-4 flex items-center gap-2 rounded-xl border p-3 text-sm',
            result.correct ? 'border-emerald-200 bg-emerald-50 text-emerald-700' : 'border-red-200 bg-red-50 text-red-700',
          )}>
            {result.correct ? <CheckCircle2Icon className="size-4" /> : <XCircleIcon className="size-4" />}
            <span>{result.correct ? '回答正确' : '回答错误'}</span>
          </div>
        )}
        <div className="mt-4 flex items-center justify-between">
          <Button variant="outline" onClick={() => navigate(backTo)}>
            <ArrowLeftIcon className="size-4" /> 返回
          </Button>
          {answered && <Button onClick={() => navigate(backTo)}>完成</Button>}
        </div>
      </div>
    </div>
  )
}

// ---------------- 编程题 ----------------

function ProgrammingSolve({ problem, backTo }: { problem: OjProblem; backTo: string }) {
  const qc = useQueryClient()
  const samples = (problem.bodyJson.samples as { input?: string; output?: string }[] | undefined) ?? []
  const [lang, setLang] = useState<CodeLang>(() => (localStorage.getItem(DRAFT_KEY + `-lang-${problem.id}`) as CodeLang) || 'python')
  const [code, setCode] = useState(() => localStorage.getItem(DRAFT_KEY + `-${problem.id}-${lang}`) ?? starterCode(lang))
  const [consoleText, setConsoleText] = useState('控制台已就绪')
  const [consoleVariant, setConsoleVariant] = useState<'default' | 'error' | 'success'>('default')
  const [busyAction, setBusyAction] = useState<string | null>(null) // run/test/submit 进行中
  const [verdictBanner, setVerdictBanner] = useState<{ verdict: string; score: number; timeMs: number } | null>(null)
  const [showCustomInput, setShowCustomInput] = useState(false)
  const [customInput, setCustomInput] = useState('')
  const [historyOpen, setHistoryOpen] = useState(false)

  useEffect(() => {
    localStorage.setItem(DRAFT_KEY + `-${problem.id}-${lang}`, code)
  }, [code, lang, problem.id])

  function switchLang(l: CodeLang) {
    if (l === lang) return
    const prev = lang
    setLang(l)
    setCode(localStorage.getItem(DRAFT_KEY + `-${problem.id}-${l}`) ?? starterCode(l))
    localStorage.setItem(DRAFT_KEY + `-lang-${problem.id}`, l)
    setConsoleText(prev ? '语言已切换，代码草稿分别保存' : '控制台已就绪')
  }

  const codeRef = useRef(code)
  codeRef.current = code

  async function poll(submissionId: number): Promise<SubmissionPoll> {
    for (let i = 0; i < 200; i++) {
      const snap = await api.ojPoll(submissionId)
      if (snap.isFinal) return snap
      await new Promise((r) => setTimeout(r, snap.pollAfterMs || 1000))
    }
    throw new Error('判题等待超时，请稍后查看测评记录')
  }

  async function action(kind: 'run' | 'test' | 'submit', inputOverride?: string) {
    if (busyAction) return
    if (!codeRef.current.trim()) {
      toast.error('代码不能为空')
      return
    }
    setBusyAction(kind)
    setVerdictBanner(null)
    const name = kind === 'run' ? '运行' : kind === 'test' ? '测试' : '提交'
    setConsoleText(`[${new Date().toLocaleTimeString()}] ${name}中…`)
    setConsoleVariant('default')
    try {
      const created = kind === 'run'
        ? await api.ojRun(problem.id, lang, codeRef.current, inputOverride ?? '')
        : kind === 'test'
          ? await api.ojTest(problem.id, lang, codeRef.current)
          : await api.ojSubmit(problem.id, lang, codeRef.current)
      const snap = await poll(created.submissionId)
      applySnap(snap, kind)
    } catch (err) {
      setConsoleText(err instanceof Error ? err.message : `${name}失败`)
      setConsoleVariant('error')
    } finally {
      setBusyAction(null)
    }
  }

  function applySnap(snap: SubmissionPoll, kind: 'run' | 'test' | 'submit') {
    const details = snap.caseDetails ?? []
    const total = details.length
    const passed = details.filter((d) => d.verdict === 'AC' || d.verdict === 'OK').length
    if (snap.verdict === 'CE') {
      setConsoleText(`编译失败\n\n${snap.stderr || ''}`)
      setConsoleVariant('error')
    } else if (kind === 'run') {
      setConsoleText(`${snap.stdout || '（无输出）'}${snap.stderr ? `\n[stderr]\n${snap.stderr}` : ''}`)
      setConsoleVariant(snap.verdict === 'OK' ? 'success' : 'error')
    } else if (snap.verdict === 'AC' || snap.verdict === 'OK') {
      setConsoleText(`测试结果：全部通过（${total} 个测试点）`)
      setConsoleVariant('success')
    } else {
      const failed = details.find((d) => d.verdict !== 'AC' && d.verdict !== 'OK')
      const parts = [`测试结果：未通过${total ? `（通过 ${passed}/${total}）` : ''}`]
      if (failed) {
        if (failed.error) parts.push(`\n${failed.error}`)
        if (failed.input !== undefined) parts.push(`\n输入：\n${failed.input}`)
        if (failed.output !== undefined) parts.push(`\n输出：\n${failed.output}`)
        if (failed.expectedOutput !== undefined) parts.push(`\n期望：\n${failed.expectedOutput}`)
      }
      setConsoleText(parts.join('\n'))
      setConsoleVariant('error')
    }
    if (kind === 'submit' || kind === 'test') {
      setVerdictBanner({ verdict: snap.verdict, score: snap.score, timeMs: snap.timeMs })
      // 判题落定后刷新任务完成态（返回列表/详情时即时）
      void qc.invalidateQueries({ queryKey: ['oj-assigned'] })
      void qc.invalidateQueries({ queryKey: ['oj-training'] })
      void qc.invalidateQueries({ queryKey: ['oj-practice'] })
    }
  }

  const problemBody = problem.bodyJson as { inputFormat?: string; outputFormat?: string }

  return (
    <div className="mx-auto flex w-full max-w-6xl flex-col gap-3 px-3 py-3 lg:h-full lg:flex-row lg:overflow-hidden lg:px-5 lg:py-4 2xl:max-w-7xl">
      {/* 左：题面 */}
      <div className="min-w-0 flex-1 overflow-y-auto rounded-2xl border bg-card p-4">
        <TopBar backTo={backTo} problem={problem} />
        <div className="mt-3 space-y-3">
          <div className="rounded-xl bg-muted/50 p-3 text-sm leading-relaxed">
            <Markdown text={preserveLineBreaks(problem.statementMd || '（暂无题面）')} className="markdown-body" />
          </div>
          {problemBody.inputFormat && (
            <Section title="输入格式"><Markdown text={problemBody.inputFormat} className="markdown-body text-sm" /></Section>
          )}
          {problemBody.outputFormat && (
            <Section title="输出格式"><Markdown text={problemBody.outputFormat} className="markdown-body text-sm" /></Section>
          )}
          {samples.length > 0 && (
            <div>
              <div className="mb-1.5 text-sm font-semibold">样例</div>
              <div className="space-y-2">
                {samples.map((s, i) => (
                  <div key={i} className="grid grid-cols-1 gap-2 sm:grid-cols-2">
                    <SampleBox label={`输入样例 ${i + 1}`} text={s.input ?? ''} onUse={() => { setCustomInput(s.input ?? ''); setShowCustomInput(true) }} />
                    <SampleBox label={`输出样例 ${i + 1}`} text={s.output ?? ''} />
                  </div>
                ))}
              </div>
            </div>
          )}
          <div className="text-xs text-muted-foreground">
            时间限制：{problem.timeLimitMs} ms · 内存限制：{problem.memoryLimitMiB} MiB
          </div>
        </div>
      </div>

      {/* 右：编辑器 */}
      <div className="flex min-h-[420px] min-w-0 flex-1 flex-col rounded-2xl border bg-card lg:h-full xl:flex-[1.15]">
        <div className="flex flex-wrap items-center gap-1.5 border-b p-2">
          <Select value={lang} onValueChange={(v) => switchLang(v as CodeLang)}>
            <SelectTrigger className="h-8 w-[130px] text-xs">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="python">Python 3</SelectItem>
              <SelectItem value="cpp">C++ (g++ 11)</SelectItem>
            </SelectContent>
          </Select>
          <Button size="sm" className="h-8 bg-emerald-600 text-xs hover:bg-emerald-700" disabled={!!busyAction} onClick={() => setShowCustomInput(true)}>
            {busyAction === 'run' ? <Loader2Icon className="size-3.5 animate-spin" /> : <PlayIcon className="size-3.5" />} 运行
          </Button>
          <Button size="sm" variant="secondary" className="h-8 text-xs" disabled={!!busyAction} onClick={() => void action('test')}>
            {busyAction === 'test' ? <Loader2Icon className="size-3.5 animate-spin" /> : <FlaskConicalIcon className="size-3.5" />} 测试
          </Button>
          <Button size="sm" className="h-8 text-xs" disabled={!!busyAction} onClick={() => void action('submit')}>
            {busyAction === 'submit' ? <Loader2Icon className="size-3.5 animate-spin" /> : <SendIcon className="size-3.5" />} 提交
          </Button>
          <div className="flex-1" />
          <Button variant="ghost" size="sm" className="h-8 text-xs" onClick={() => setHistoryOpen(true)}>
            <HistoryIcon className="size-3.5" /> 测评记录
          </Button>
        </div>

        {verdictBanner && (
          <div className={cn('flex items-center gap-2 border-b px-3 py-2 text-sm font-medium', isAcceptedVerdict(verdictBanner.verdict) ? 'border-emerald-200 bg-emerald-50 text-emerald-700' : 'border-red-200 bg-red-50 text-red-700')}>
            {isAcceptedVerdict(verdictBanner.verdict) ? <CheckCircle2Icon className="size-4" /> : <XCircleIcon className="size-4" />}
            <span>{verdictText(verdictBanner.verdict)}（得分 {verdictBanner.score}）</span>
            <span className="text-xs font-normal opacity-70">耗时 {verdictBanner.timeMs} ms</span>
          </div>
        )}

        <div className="min-h-[260px] flex-1 border-y bg-background">
          <CodeEditor language={lang} value={code} onChange={setCode} />
        </div>

        {/* 控制台 */}
        <div className="border-t p-2">
          <div className="mb-1 text-[11px] font-medium text-muted-foreground">控制台输出</div>
          <pre
            className={cn(
              'min-h-[90px] overflow-auto rounded-lg border p-2.5 font-mono text-xs whitespace-pre-wrap',
              consoleVariant === 'error' && 'border-red-200 bg-red-50 text-red-700',
              consoleVariant === 'success' && 'border-emerald-200 bg-emerald-50 text-emerald-700',
              consoleVariant === 'default' && 'bg-muted',
            )}
          >
            {consoleText}
          </pre>
        </div>
      </div>

      {/* 自定义输入对话框（运行用） */}
      <CustomInputDialog
        open={showCustomInput}
        onOpenChange={setShowCustomInput}
        value={customInput}
        onChange={setCustomInput}
        onSubmit={() => { setShowCustomInput(false); void action('run', customInput) }}
        busy={busyAction === 'run'}
      />
      <SubmissionHistoryDialog problemId={problem.id} open={historyOpen} onOpenChange={setHistoryOpen} />
    </div>
  )
}

function isAcceptedVerdict(v: string) { return v === 'AC' || v === 'OK' }

function TopBar({ backTo, problem }: { backTo: string; problem: OjProblem }) {
  const navigate = useNavigate()
  return (
    <div className="flex items-center gap-2">
      <Button variant="ghost" size="sm" className="-ml-2 text-muted-foreground" onClick={() => navigate(backTo)}>
        <ArrowLeftIcon className="size-4" /> 返回
      </Button>
      <div className="min-w-0 flex-1">
        <h1 className="truncate text-base font-semibold">{problem.title}</h1>
      </div>
      <Badge>{typeLabel(problem.type)}</Badge>
    </div>
  )
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div>
      <div className="mb-1 text-sm font-semibold">{title}</div>
      {children}
    </div>
  )
}

function SampleBox({ label, text, onUse }: { label: string; text: string; onUse?: () => void }) {
  return (
    <div className="rounded-lg border bg-muted/40">
      <div className="flex items-center justify-between px-2.5 py-1">
        <span className="text-[11px] font-semibold text-muted-foreground">{label}</span>
        <div className="flex items-center gap-0.5">
          {onUse && (
            <button type="button" className="rounded p-1 text-muted-foreground hover:bg-muted" onClick={onUse} title="填入自定义输入">
              <CopyIcon className="size-3.5" />
            </button>
          )}
        </div>
      </div>
      <pre className="overflow-x-auto border-t bg-background px-2.5 py-2 font-mono text-xs whitespace-pre-wrap">{text || '（空）'}</pre>
    </div>
  )
}

function CustomInputDialog({ open, onOpenChange, value, onChange, onSubmit, busy }: {
  open: boolean
  onOpenChange: (v: boolean) => void
  value: string
  onChange: (v: string) => void
  onSubmit: () => void
  busy: boolean
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>自定义输入</DialogTitle>
          <DialogDescription>输入将作为标准输入传给程序运行（不做答案比对）</DialogDescription>
        </DialogHeader>
        <Textarea rows={6} value={value} onChange={(e) => onChange(e.target.value)} placeholder="例如：1 2" className="font-mono text-sm" />
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>取消</Button>
          <Button onClick={onSubmit} disabled={busy}>
            {busy && <Loader2Icon className="size-4 animate-spin" />} 运行
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

// ---------------- 测评记录 ----------------

function SubmissionHistoryDialog({ problemId, open, onOpenChange }: { problemId: number; open: boolean; onOpenChange: (v: boolean) => void }) {
  const [selected, setSelected] = useState<Submission | null>(null)
  const submissionsQ = useQuery({
    queryKey: ['oj-submissions', problemId],
    queryFn: () => api.ojSubmissions(problemId),
    enabled: open,
  })
  const list = submissionsQ.data?.submissions ?? []
  const [tab, setTab] = useState('code')
  const [caseIdx, setCaseIdx] = useState(0)

  return (
    <Dialog open={open} onOpenChange={(v) => { if (!v) { setSelected(null); setTab('code'); setCaseIdx(0) } onOpenChange(v) }}>
      <DialogContent className="flex max-h-[85vh] flex-col sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>测评记录</DialogTitle>
        </DialogHeader>
        <div className="min-h-0 flex-1 overflow-hidden">
          {selected ? (
            <SelectedView
              sub={selected}
              tab={tab}
              setTab={setTab}
              caseIdx={caseIdx}
              setCaseIdx={setCaseIdx}
              onBack={() => setSelected(null)}
            />
          ) : list.length === 0 ? (
            <div className="p-8 text-center text-sm text-muted-foreground">
              暂无测评记录
              {submissionsQ.isLoading && <Loader2Icon className="mx-auto mt-2 size-4 animate-spin" />}
            </div>
          ) : (
            <div className="max-h-[60vh] overflow-y-auto">
              {list.map((s) => (
                <button
                  key={s.id}
                  type="button"
                  onClick={() => { setSelected(s); setTab('code'); setCaseIdx(0) }}
                  className="flex w-full items-center gap-3 border-b px-3 py-2.5 text-left hover:bg-accent"
                >
                  <span className={cn('rounded-md border px-1.5 py-0.5 text-xs font-medium', verdictCls(s.verdict))}>{verdictText(s.verdict)}</span>
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-xs text-muted-foreground">
                      #{s.id} · {submitTypeText(s.submitType)} · {langLabel(s.language)} · {new Date(s.createdAt).toLocaleString()}
                    </p>
                  </div>
                  <span className="shrink-0 text-[11px] text-muted-foreground">
                    {s.timeMs}ms{s.score > 0 ? ` · ${s.score} 分` : ''}
                  </span>
                </button>
              ))}
            </div>
          )}
        </div>
        <DialogFooter className="mt-2">
          <Button variant="outline" size="sm" onClick={() => onOpenChange(false)}>关闭</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function submitTypeText(t: string) {
  return t === 'run' ? '运行' : t === 'test' ? '测试' : t === 'submit' ? '提交' : '客观题'
}

function SelectedView({ sub, tab, setTab, caseIdx, setCaseIdx, onBack }: {
  sub: Submission
  tab: string
  setTab: (t: string) => void
  caseIdx: number
  setCaseIdx: (i: number) => void
  onBack: () => void
}) {
  const cases: CaseDetail[] = sub.caseDetails ?? []
  const selCase = cases[caseIdx]
  return (
    <div className="flex h-full flex-col">
      <div className="mb-2 flex flex-wrap items-center gap-2">
        <Button variant="outline" size="sm" onClick={onBack}>返回列表</Button>
        <Badge variant="outline" className={verdictCls(sub.verdict)}>{verdictText(sub.verdict)}</Badge>
        {cases.length > 0 && (
          <Badge variant="outline">
            通过 {cases.filter((c) => c.verdict === 'AC' || c.verdict === 'OK').length}/{cases.length}
          </Badge>
        )}
        <span className="ml-auto text-[11px] text-muted-foreground">
          {sub.timeMs}ms · {Math.round(sub.memoryKiB / 1024)} MiB · #{sub.id}
        </span>
      </div>
      {cases.length > 0 && (
        <Select value={String(caseIdx)} onValueChange={(v) => setCaseIdx(Number(v))}>
          <SelectTrigger className="mb-2 h-8 w-full text-xs">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {cases.map((c, i) => (
              <SelectItem key={c.caseNo} value={String(i)}>
                测试点 {c.caseNo} · {verdictText(c.verdict)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      )}
      <Tabs value={tab} onValueChange={setTab}>
        <TabsList className="w-full">
          <TabsTrigger value="code" className="flex-1 text-xs">代码</TabsTrigger>
          {cases.length > 0 && <TabsTrigger value="input" className="flex-1 text-xs">输入</TabsTrigger>}
          {cases.length > 0 && <TabsTrigger value="output" className="flex-1 text-xs">输出</TabsTrigger>}
          {cases.length > 0 && <TabsTrigger value="expected" className="flex-1 text-xs">期望</TabsTrigger>}
          <TabsTrigger value="error" className="flex-1 text-xs">错误</TabsTrigger>
        </TabsList>
      </Tabs>
      <div className="mt-2 min-h-0 flex-1 overflow-auto rounded-lg border bg-muted/30 p-3">
        {tab === 'code' && (
          sub.sourceCode
            ? <CodeBlock code={sub.sourceCode} language={sub.language} className="[&_pre]:whitespace-pre-wrap" />
            : <pre className="font-mono text-xs whitespace-pre-wrap">（无）</pre>
        )}
        {tab === 'input' && <pre className="font-mono text-xs whitespace-pre-wrap">{selCase?.input ?? sub.inputData ?? '（空）'}</pre>}
        {tab === 'output' && <pre className="font-mono text-xs whitespace-pre-wrap">{selCase?.output ?? sub.stdout ?? '（无输出）'}</pre>}
        {tab === 'expected' && <pre className="font-mono text-xs whitespace-pre-wrap">{selCase?.expectedOutput ?? '（无）'}</pre>}
        {tab === 'error' && <pre className="font-mono text-xs whitespace-pre-wrap text-red-600">{selCase?.error ?? sub.stderr ?? '（无）'}</pre>}
      </div>
    </div>
  )
}
