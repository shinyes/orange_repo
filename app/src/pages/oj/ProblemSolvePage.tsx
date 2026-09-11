import { useEffect, useRef, useState } from 'react'
import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import {
  ArrowLeftIcon,
  CheckCircle2Icon,
  CopyIcon,
  HistoryIcon,
  Loader2Icon,
  PlayIcon,
  RotateCcwIcon,
  SendIcon,
  XCircleIcon,
  FlaskConicalIcon,
} from 'lucide-react'
import { toast } from 'sonner'
import { useQuery } from '@tanstack/react-query'

import { api } from '@/api'
import type { CaseDetail, CodeLang, OjProblem, Submission, SubmissionPoll } from '@/api/types'
import { Markdown, preserveLineBreaks } from '@/lib/markdown'
import { CodeBlock } from '@/lib/code-highlight'
import { CodeEditor } from '@/components/CodeEditor'
import { SplitPane } from '@/components/portal/SplitPane'
import { AdminEditProblemButton } from '@/components/portal/admin-edit-problem'
import { ViewSolutionButton } from '@/components/portal/view-solution-button'
import { ZoomControls } from '@/components/portal/zoom-controls'
import { EditorCollapseButton } from '@/components/portal/editor-collapse-button'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Textarea } from '@/components/ui/textarea'
import {
  Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle,
} from '@/components/ui/dialog'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { cn } from '@/lib/utils'
import { langLabel, verdictCls, verdictText } from './oj-utils'
import { resolveStarter, saveDraftDebounced, useCloudDraft } from '@/lib/use-programming-workspace'
import { CONSOLE_DEFAULT_H, useConsoleResize } from '@/hooks/use-console-resize'

const DRAFT_KEY = 'oj-draft'

// 做题页：客观题内联作答 / 编程题 运行·测试·提交。
export function ProblemSolvePage() {
  const { problemId } = useParams()
  const [searchParams] = useSearchParams()
  const navigate = useNavigate()
  const pid = Number(problemId)
  const backTo = searchParams.get('back') ?? '/'
  const review = searchParams.get('review') === '1'
  const practiceId = Number(searchParams.get('practiceId') || 0) || undefined

  const problemQ = useQuery({
    queryKey: ['oj-problem', pid],
    queryFn: () => api.ojProblem(pid),
    enabled: pid > 0,
  })
  const problem = problemQ.data

  if (!Number.isFinite(pid) || pid <= 0) {
    return (
      <Center>
        <p className="text-sm text-muted-foreground">题目不存在或不可见</p>
        <Button variant="outline" className="mt-3" onClick={() => navigate(backTo)}>返回</Button>
      </Center>
    )
  }
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
    <ProgrammingSolve key={pid} problem={problem} backTo={backTo} review={review} practiceId={practiceId} />
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
  const [picked, setPicked] = useState<number | null>(null)
  const [pickedTF, setPickedTF] = useState<boolean | null>(null)
  const [result, setResult] = useState<ObjSubmitResult | null>(null)
  const [busy, setBusy] = useState(false)
  const [statementScale, setStatementScale] = useState(1)
  const options = (problem.bodyJson.options as string[] | undefined) ?? []

  async function submit(payload: { optionIndex?: number; answer?: boolean }) {
    if (busy || result) return
    setBusy(true)
    try {
      const r = await api.ojObjectiveSubmit(problem.id, payload.optionIndex ?? payload.answer ?? false)
      setResult({ correct: r.correct, verdict: r.verdict, correctAnswer: r.correctAnswer })
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
      <TopBar backTo={backTo} problem={problem} scale={statementScale} onScale={setStatementScale} />
      <div className="mt-3 rounded-2xl border bg-card p-5">
        <div style={{ zoom: statementScale }}>
          <Markdown text={preserveLineBreaks(problem.statementMd || '（暂无题面）')} className="markdown-body text-[17px] leading-relaxed" />
        </div>

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

function ProgrammingSolve({ problem, backTo, review, practiceId }: { problem: OjProblem; backTo: string; review?: boolean; practiceId?: number }) {
  const [statementScale, setStatementScale] = useState(1)
  // 折叠「代码编辑器 + 控制台」：题面占满（仅隐藏不卸载，草稿与 Monaco 状态保留）
  const [editorCollapsed, setEditorCollapsed] = useState(false)
  const samples = (problem.bodyJson.samples as { input?: string; output?: string }[] | undefined) ?? []
  // 草稿上下文：练习进入=按练习隔离（云端 ctx practice+id）；做题页直达=全局
  const ctxKind = practiceId ? 'practice' : ''
  const ctxId = practiceId
  const ctxTag = practiceId ? `p${practiceId}` : 'g'
  // 本地草稿 key（含上下文，防跨练习/全局互串）
  const draftLocal = (pid: number, l: CodeLang) => `${DRAFT_KEY}-${ctxTag}-${pid}-${l}`
  const [lang, setLang] = useState<CodeLang>(() => (localStorage.getItem(`${DRAFT_KEY}-${ctxTag}-lang-${problem.id}`) as CodeLang) || 'python')
  // 初始 code：本地草稿 →（异步）云草稿 → 题目模板（starterPy/starterCpp）→ 通用模板。
  const [code, setCode] = useState(() => localStorage.getItem(draftLocal(problem.id, lang)) ?? resolveStarter(lang, problem))
  const [consoleText, setConsoleText] = useState('控制台已就绪')
  const [consoleVariant, setConsoleVariant] = useState<'default' | 'error' | 'success'>('default')
  // 控制台高度（默认≈5 行；上下拖拽调整 / 重置）——与训练内嵌卡一致
  const { consoleH, startDrag: startConsoleDrag, reset: resetConsoleH } = useConsoleResize(CONSOLE_DEFAULT_H)
  const [busyAction, setBusyAction] = useState<string | null>(null) // run/test/submit 进行中
  const [verdictBanner, setVerdictBanner] = useState<{ verdict: string; score: number; timeMs: number } | null>(null)
  const [showCustomInput, setShowCustomInput] = useState(false)
  const [customInput, setCustomInput] = useState('')
  const [historyOpen, setHistoryOpen] = useState(false)

  // 云端草稿（按 题×语言×上下文 GET）：加载完成后回填——本地已有草稿则保留本地。
  const cloudDraft = useCloudDraft(problem.id, lang, ctxKind, ctxId)
  // 本会话内当前语言是否已被用户手动编辑（一旦输入，云端草稿不再覆盖；切语言时重置）。
  const touchedRef = useRef(false)
  function enterLang() {
    touchedRef.current = false
    draftWarnedRef.current = false
  }

  // 云端草稿到达后回填：仅当用户未输入且本地无草稿时，用云端内容覆盖题目/通用模板并写入本地草稿。
  useEffect(() => {
    if (!cloudDraft.cloudLoaded || touchedRef.current) return
    const local = localStorage.getItem(draftLocal(problem.id, lang))
    if (local != null && local.trim() !== '') return
    if (!cloudDraft.initialCode || cloudDraft.initialCode.trim() === '') return // 云端无草稿：停留当前模板
    setCode(cloudDraft.initialCode)
    localStorage.setItem(draftLocal(problem.id, lang), cloudDraft.initialCode)
  }, [cloudDraft.cloudLoaded, cloudDraft.initialCode, lang, problem.id, ctxTag])

  // 云端草稿到达但用户已先输入（慢网竞态）：保留当前编辑并提示，避免云草稿被静默丢弃
  const draftWarnedRef = useRef(false)
  useEffect(() => {
    if (!review && !draftWarnedRef.current && touchedRef.current && cloudDraft.cloudLoaded
      && cloudDraft.initialCode && cloudDraft.initialCode.trim() !== '' && cloudDraft.initialCode !== code) {
      draftWarnedRef.current = true
      toast.warning('检测到云端草稿（其它页面/设备保存过）：保留当前编辑，后续保存将覆盖云端草稿')
    }
  }, [code, cloudDraft.cloudLoaded, cloudDraft.initialCode])

  // 编辑器输入：实时写本地草稿 + 云端 debounce 自动保存（静默失败，本地已缓存）。
  function handleCodeChange(next: string) {
    if (review) return // 回顾只读，不写草稿
    touchedRef.current = true
    setCode(next)
    localStorage.setItem(draftLocal(problem.id, lang), next)
    saveDraftDebounced(problem.id, lang, next, ctxKind, ctxId)
  }

  function switchLang(l: CodeLang) {
    if (l === lang) return
    setLang(l)
    setCode(localStorage.getItem(draftLocal(problem.id, l)) ?? resolveStarter(l, problem))
    enterLang()
    localStorage.setItem(`${DRAFT_KEY}-${ctxTag}-lang-${problem.id}`, l)
    setConsoleText('语言已切换，代码草稿分别保存')
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
          : await api.ojSubmit(problem.id, lang, codeRef.current, undefined, practiceId)
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
    }
  }

  const problemBody = problem.bodyJson as { inputFormat?: string; outputFormat?: string }

  return (
    <div className="mx-auto flex h-full w-full max-w-7xl flex-col px-3 py-3 2xl:max-w-[96rem]">
    <div className="min-h-0 flex-1">
    <>
    <SplitPane
      collapsed={editorCollapsed}
      left={
        <div className="min-w-0 h-full overflow-y-auto rounded-2xl border bg-card p-4 [scrollbar-gutter:stable]">
          <TopBar
            backTo={backTo}
            problem={problem}
            scale={statementScale}
            onScale={setStatementScale}
            editorCollapsed={editorCollapsed}
            onToggleEditor={() => setEditorCollapsed((v) => !v)}
          />
          <div style={{ zoom: statementScale }} className="mt-3 space-y-3">
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
      }
      right={
        <div className="flex h-full min-h-[420px] min-w-0 flex-col rounded-2xl border bg-card">
          <div className="flex flex-wrap items-center gap-1.5 border-b p-2">
            {review && (
              <span className="mr-1 inline-flex items-center rounded-md border border-sky-300 bg-sky-50 px-2 py-0.5 text-[11px] font-semibold text-sky-700">
                回顾模式（只读，不可作答）
              </span>
            )}
            <Select value={lang} onValueChange={(v) => switchLang(v as CodeLang)} disabled={review}>
              <SelectTrigger className="h-8 w-[130px] text-xs">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="python">Python 3</SelectItem>
                <SelectItem value="cpp">C++ (g++ 11)</SelectItem>
              </SelectContent>
            </Select>
            <Button size="sm" className="h-8 bg-emerald-600 text-xs hover:bg-emerald-700" disabled={!!busyAction || review} title={review ? '回顾模式不可运行' : undefined} onClick={() => setShowCustomInput(true)}>
              {busyAction === 'run' ? <Loader2Icon className="size-3.5 animate-spin" /> : <PlayIcon className="size-3.5" />} 运行
            </Button>
            <Button size="sm" variant="secondary" className="h-8 text-xs" disabled={!!busyAction || review} title={review ? '回顾模式不可测试' : undefined} onClick={() => void action('test')}>
              {busyAction === 'test' ? <Loader2Icon className="size-3.5 animate-spin" /> : <FlaskConicalIcon className="size-3.5" />} 测试
            </Button>
            <Button size="sm" className="h-8 text-xs" disabled={!!busyAction || review} title={review ? '回顾模式不可提交' : undefined} onClick={() => void action('submit')}>
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

          <div className="min-h-[200px] flex-1 border-y bg-background">
            <CodeEditor language={lang} value={code} onChange={handleCodeChange} readOnly={review} />
          </div>

          {/* 拖拽句柄：调整 编辑器/控制台 占比 */}
          <div
            role="separator"
            aria-orientation="horizontal"
            title="拖动调整控制台高度（双击重置）"
            onPointerDown={startConsoleDrag}
            onDoubleClick={resetConsoleH}
            className="group -mx-1 flex h-3 shrink-0 cursor-ns-resize touch-none items-center justify-center"
          >
            <div className="h-1 w-12 rounded-full bg-muted-foreground/25 transition-colors group-hover:bg-primary/50 group-active:bg-primary/70" />
          </div>

          {/* 控制台（默认≈5 行；高度可拖拽，标题行右侧可清空/重置） */}
          <div className="shrink-0 border-t p-2">
            <div className="mb-1 flex items-center gap-2 text-[11px] font-medium text-muted-foreground">
              <span>控制台输出</span>
              <span className="ml-auto flex items-center gap-1">
                {consoleText !== '控制台已就绪' && (
                  <button
                    type="button"
                    className="underline-offset-2 hover:underline"
                    onClick={() => { setConsoleText('控制台已就绪'); setConsoleVariant('default') }}
                  >
                    清空
                  </button>
                )}
                <button
                  type="button"
                  title="重置为默认占比（约 5 行）"
                  className="inline-flex size-5 items-center justify-center rounded text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
                  onClick={resetConsoleH}
                >
                  <RotateCcwIcon className="size-3" />
                </button>
              </span>
            </div>
            <pre
              style={{ height: consoleH }}
              className={cn(
                'overflow-auto whitespace-pre-wrap rounded-lg border p-2 font-mono text-xs leading-relaxed',
                consoleVariant === 'error' && 'border-red-200 bg-red-50 text-red-700',
                consoleVariant === 'success' && 'border-emerald-200 bg-emerald-50 text-emerald-700',
                consoleVariant === 'default' && 'bg-muted/40',
              )}
            >
              {consoleText}
            </pre>
          </div>
        </div>
      }
          />
    {/* 自定义输入对话框（运行用） */}
    <CustomInputDialog
      open={showCustomInput}
      onOpenChange={setShowCustomInput}
      value={customInput}
      onChange={setCustomInput}
      onSubmit={() => { setShowCustomInput(false); void action('run', customInput) }}
      busy={busyAction === 'run'}
    />
    <SubmissionHistoryDialog problemId={problem.id} practiceId={practiceId} open={historyOpen} onOpenChange={setHistoryOpen} />
    </>
    </div>
    </div>
  )
}
function isAcceptedVerdict(v: string) { return v === 'AC' || v === 'OK' }

function TopBar({ backTo, problem, scale, onScale, editorCollapsed, onToggleEditor }: {
  backTo: string
  problem: OjProblem
  scale: number
  onScale: (s: number) => void
  /** 仅编程题用：折叠代码编辑器与控制台 */
  editorCollapsed?: boolean
  onToggleEditor?: () => void
}) {
  const navigate = useNavigate()
  return (
    <div className="flex items-center gap-2">
      <Button variant="ghost" size="sm" className="-ml-2 text-muted-foreground" onClick={() => navigate(backTo)}>
        <ArrowLeftIcon className="size-4" /> 返回
      </Button>
      <div className="min-w-0 flex-1">
        <h1 className="truncate text-base font-semibold">{problem.title}</h1>
      </div>
      {/* 管理员：查看题解（仅编程题）+ 编辑题目 + 文字缩放；编程题再加「折叠编辑器与控制台」 */}
      <div className="flex shrink-0 items-center gap-1.5">
        {problem.type === 'programming' && <ViewSolutionButton problemId={problem.id} />}
        <AdminEditProblemButton problemId={problem.id} />
        <ZoomControls scale={scale} onChange={onScale} />
        {problem.type === 'programming' && onToggleEditor && (
          <EditorCollapseButton collapsed={!!editorCollapsed} onToggle={onToggleEditor} />
        )}
      </div>
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

function SubmissionHistoryDialog({ problemId, practiceId, open, onOpenChange }: { problemId: number; practiceId?: number; open: boolean; onOpenChange: (v: boolean) => void }) {
  const [selected, setSelected] = useState<Submission | null>(null)
  const submissionsQ = useQuery({
    queryKey: ['oj-submissions', practiceId ? 'p' : 'g', problemId, practiceId ?? 0],
    queryFn: () => api.ojSubmissions(problemId, undefined, practiceId),
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
