// 训练内编程题作答卡：页内直接答题（不跳做题页）——
// 语言选择 + Monaco 编辑器（草稿本地实时保存 + 云端 debounce 自动保存，初始代码
// 本地 → 云草稿 → 题目模板(starterPy/starterCpp) → 通用模板）+ 运行(自定义输入)/测试/提交
// + 控制台输出与判定结果；提交带 trainingId（服务端落 submissions.training_id），
// 提交 AC 后轮询带 trainingId 使训练条目标记通过（格子变绿）。
// 测评记录：工具栏 History 按钮 → Dialog 拉该训练×题提交历史（ojSubmissions(id, trainingId)）。
import { useEffect, useRef, useState } from 'react'
import { toast } from 'sonner'
import {
  FlaskConicalIcon,
  HistoryIcon,
  Loader2Icon,
  PlayIcon,
  SendIcon,
  CheckCircle2Icon,
  XCircleIcon,
} from 'lucide-react'
import { useQuery } from '@tanstack/react-query'

import { api } from '@/api'
import type { CodeLang, Submission, SubmissionPoll } from '@/api/types'
import { CodeEditor } from '@/components/CodeEditor'
import { Button } from '@/components/ui/button'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import {
  Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle,
} from '@/components/ui/dialog'
import { cn } from '@/lib/utils'
import { genericStarter, resolveStarter, saveDraftDebounced, useCloudDraft } from '@/lib/use-programming-workspace'

const DRAFT_PREFIX = 'orangeoj:draft:'

function draftKey(problemId: number, lang: CodeLang) {
  return `${DRAFT_PREFIX}${problemId}-${lang}`
}

export function TrainingProgrammingCard({ problemId, trainingId, solved, onSolved }: {
  problemId: number
  trainingId: number
  /** 该题当前是否已通过（决定 AC 后是否轮询标记） */
  solved: boolean
  onSolved: () => void
}) {
  // 题目数据（父级 ProgrammingStatement 已 useQuery 同 key，缓存命中直接取到 starterPy/starterCpp）。
  const problemQ = useQuery({
    queryKey: ['oj-problem', problemId],
    queryFn: () => api.ojProblem(problemId),
    retry: 1,
  })

  const [lang, setLang] = useState<CodeLang>(() => (localStorage.getItem(`${DRAFT_PREFIX}lang-${problemId}`) as CodeLang) || 'python')
  // 初始 code：本地草稿 →（下方 reconcile）云草稿 → 题目模板 → 通用模板
  const [code, setCode] = useState(() => localStorage.getItem(draftKey(problemId, lang)) ?? genericStarter(lang))
  const [consoleText, setConsoleText] = useState('控制台已就绪')
  const [consoleVariant, setConsoleVariant] = useState<'default' | 'error' | 'success'>('default')
  const [busy, setBusy] = useState<string | null>(null) // run/test/submit
  const [verdict, setVerdict] = useState<{ verdict: string; score: number; timeMs: number } | null>(null)
  const [showCustomInput, setShowCustomInput] = useState(false)
  const [customInput, setCustomInput] = useState('')
  const [historyOpen, setHistoryOpen] = useState(false)
  const codeRef = useRef(code)
  codeRef.current = code
  const touchedRef = useRef(false)

  // 云端草稿：本地为空且用户未输入时，随 cloudLoaded/题目数据到达逐级回填（云草稿 → 题目模板 → 通用）；
  // 仅云草稿回填写本地草稿，模板本身不落本地（避免挡住后续云草稿）。
  const cloudDraft = useCloudDraft(problemId, lang)
  useEffect(() => {
    if (touchedRef.current) return
    const local = localStorage.getItem(draftKey(problemId, lang))
    if (local != null && local.trim() !== '') return
    let next: string | null = null
    if (cloudDraft.cloudLoaded && cloudDraft.initialCode && cloudDraft.initialCode.trim() !== '') {
      next = cloudDraft.initialCode
    } else if (problemQ.data) {
      next = resolveStarter(lang, problemQ.data)
    }
    if (next == null || next === code) return
    setCode(next)
    // 云草稿（用户的真实内容）持久化；模板无需持久化（随时可按题目数据重算）。
    if (cloudDraft.cloudLoaded && cloudDraft.initialCode.trim() !== '') {
      localStorage.setItem(draftKey(problemId, lang), cloudDraft.initialCode)
    }
  }, [code, lang, cloudDraft.cloudLoaded, cloudDraft.initialCode, problemQ.data, problemId])

  // 编辑器输入：实时写本地草稿 + 云端 debounce 自动保存（失败静默，本地已缓存）。
  function handleCodeChange(next: string) {
    touchedRef.current = true
    setCode(next)
    localStorage.setItem(draftKey(problemId, lang), next)
    saveDraftDebounced(problemId, lang, next)
  }

  function switchLang(l: CodeLang) {
    if (l === lang) return
    touchedRef.current = false
    setLang(l)
    setCode(localStorage.getItem(draftKey(problemId, l)) ?? genericStarter(l))
    localStorage.setItem(`${DRAFT_PREFIX}lang-${problemId}`, l)
    setConsoleText('语言已切换，草稿分别保存')
    setConsoleVariant('default')
  }

  async function poll(submissionId: number): Promise<SubmissionPoll> {
    for (let i = 0; i < 200; i++) {
      const snap = await api.ojPoll(submissionId, trainingId)
      if (snap.isFinal) return snap
      await new Promise((r) => setTimeout(r, snap.pollAfterMs || 1000))
    }
    throw new Error('判题等待超时，请查看测评记录')
  }

  async function action(kind: 'run' | 'test' | 'submit', inputOverride?: string) {
    if (busy) return
    if (!codeRef.current.trim()) {
      toast.error('代码不能为空')
      return
    }
    setBusy(kind)
    setVerdict(null)
    try {
      let submissionId: number
      if (kind === 'run') {
        const r = await api.ojRun(problemId, lang, codeRef.current, inputOverride ?? customInput)
        submissionId = r.submissionId
      } else if (kind === 'test') {
        const r = await api.ojTest(problemId, lang, codeRef.current)
        submissionId = r.submissionId
      } else {
        // 训练内提交：带 trainingId（落 submissions.training_id，测评记录/轮询均限定本训练）
        const r = await api.ojSubmit(problemId, lang, codeRef.current, trainingId)
        submissionId = r.submissionId
      }
      const snap = await poll(submissionId)
      // 展示输出
      const lines: string[] = []
      if (snap.stdout) lines.push(snap.stdout.trimEnd())
      if (snap.stderr) lines.push(snap.stderr.trimEnd())
      if (snap.verdict === 'AC') {
        setConsoleVariant('success')
        lines.push(`判定通过（得分 ${snap.score}，耗时 ${snap.timeMs} ms）`)
      } else if (kind === 'submit' || kind === 'test') {
        setConsoleVariant(snap.verdict === 'WA' ? 'error' : 'default')
        lines.push(`判定：${verdictText(snap.verdict)}${snap.score > 0 ? `（得分 ${snap.score}）` : ''} 耗时 ${snap.timeMs} ms`)
        // 用例明细摘要
        const details = Array.isArray(snap.caseDetails) ? snap.caseDetails : []
        if (details.length > 0) {
          const ok = details.filter((d) => d.verdict === 'AC').length
          lines.push(`用例：${ok}/${details.length} 通过`)
        }
      } else {
        setConsoleVariant('default')
        lines.push(`运行完成（耗时 ${snap.timeMs} ms）`)
      }
      setConsoleText(lines.join('\n') || '（无输出）')
      setVerdict({ verdict: snap.verdict, score: snap.score, timeMs: snap.timeMs })
      if (kind === 'submit' && snap.verdict === 'AC' && !solved) {
        onSolved()
        toast.success('提交通过，本题已标记完成')
      } else if (snap.verdict !== 'AC' && kind !== 'run') {
        toast.error(verdictText(snap.verdict))
      }
    } catch (err) {
      setConsoleVariant('error')
      setConsoleText(err instanceof Error ? err.message : '操作失败')
      toast.error(err instanceof Error ? err.message : '操作失败')
    } finally {
      setBusy(null)
    }
  }

  const isAccepted = verdict?.verdict === 'AC'

  return (
    <div>
      {/* 编辑器工具栏 */}
      <div className="mb-2 flex flex-wrap items-center gap-1.5">
        <Select value={lang} onValueChange={(v) => switchLang(v as CodeLang)}>
          <SelectTrigger className="h-8 w-[130px] text-xs">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="python">Python 3</SelectItem>
            <SelectItem value="cpp">C++ (g++ 11)</SelectItem>
          </SelectContent>
        </Select>
        <Button size="sm" variant="outline" className="h-8 text-xs" disabled={!!busy} onClick={() => setShowCustomInput(true)}>
          {busy === 'run' ? <Loader2Icon className="size-3.5 animate-spin" /> : <PlayIcon className="size-3.5" />} 运行
        </Button>
        <Button size="sm" variant="secondary" className="h-8 text-xs" disabled={!!busy} onClick={() => void action('test')}>
          {busy === 'test' ? <Loader2Icon className="size-3.5 animate-spin" /> : <FlaskConicalIcon className="size-3.5" />} 测试
        </Button>
        <Button size="sm" className="h-8 text-xs bg-emerald-600 hover:bg-emerald-700" disabled={!!busy} onClick={() => void action('submit')}>
          {busy === 'submit' ? <Loader2Icon className="size-3.5 animate-spin" /> : <SendIcon className="size-3.5" />} 提交
        </Button>
        <Button size="sm" variant="ghost" className="h-8 text-xs" onClick={() => setHistoryOpen(true)}>
          <HistoryIcon className="size-3.5" /> 测评记录
        </Button>
        <span className="ml-auto text-[11px] text-muted-foreground">运行/测试/提交均真实评测</span>
      </div>

      {/* 判定横幅 */}
      {verdict && (
        <div className={cn('mb-2 flex items-center gap-2 rounded-lg border px-3 py-2 text-sm font-medium', isAccepted ? 'border-emerald-200 bg-emerald-50 text-emerald-700' : 'border-red-200 bg-red-50 text-red-700')}>
          {isAccepted ? <CheckCircle2Icon className="size-4" /> : <XCircleIcon className="size-4" />}
          <span>{verdictText(verdict.verdict)}（得分 {verdict.score}）</span>
          <span className="text-xs font-normal opacity-70">耗时 {verdict.timeMs} ms</span>
        </div>
      )}

      {/* 编辑器 */}
      <div className="h-64 overflow-hidden rounded-lg border bg-background md:h-80">
        <CodeEditor language={lang} value={code} onChange={handleCodeChange} />
      </div>

      {/* 控制台 */}
      <div className="mt-2">
        <div className="mb-1 text-[11px] font-medium text-muted-foreground">控制台输出</div>
        <pre
          className={cn(
            'min-h-[70px] overflow-auto whitespace-pre-wrap rounded-lg border bg-muted/40 p-2.5 font-mono text-xs',
            consoleVariant === 'error' && 'border-red-200 bg-red-50 text-red-700',
            consoleVariant === 'success' && 'border-emerald-200 bg-emerald-50 text-emerald-700',
          )}
        >
          {consoleText}
        </pre>
      </div>

      {/* 自定义输入（运行用）弹窗 */}
      {showCustomInput && (
        <CustomInputDialog
          value={customInput}
          onChange={setCustomInput}
          onRun={() => { setShowCustomInput(false); void action('run', customInput) }}
          onClose={() => setShowCustomInput(false)}
        />
      )}

      {/* 测评记录（训练内历史：ojSubmissions(problemId, trainingId)） */}
      <SubmissionHistoryDialog
        open={historyOpen}
        onOpenChange={setHistoryOpen}
        problemId={problemId}
        trainingId={trainingId}
      />
    </div>
  )
}

function verdictText(v: string): string {
  const map: Record<string, string> = {
    AC: '通过 AC', OK: '运行成功 OK', WA: '答案错误 WA', CE: '编译错误 CE', RE: '运行错误 RE',
    TLE: '超时 TLE', MLE: '超内存 MLE', PENDING: '等待评测', FAILED: '评测失败',
  }
  return map[v] ?? v
}

// verdict 徽标配色（与做题页测评记录一致）。
function verdictCls(v: string): string {
  const map: Record<string, string> = {
    AC: 'text-emerald-600 bg-emerald-50 border-emerald-200',
    OK: 'text-emerald-600 bg-emerald-50 border-emerald-200',
    WA: 'text-red-600 bg-red-50 border-red-200',
    CE: 'text-amber-600 bg-amber-50 border-amber-200',
    RE: 'text-red-600 bg-red-50 border-red-200',
    TLE: 'text-amber-600 bg-amber-50 border-amber-200',
    MLE: 'text-amber-600 bg-amber-50 border-amber-200',
  }
  return map[v] ?? 'text-muted-foreground bg-muted border-border'
}

function langText(lang: string): string {
  return lang === 'cpp' ? 'C++' : lang === 'python' ? 'Python 3' : lang
}

function submitTypeText(t: string) {
  return t === 'run' ? '运行' : t === 'test' ? '测试' : t === 'submit' ? '提交' : '客观题'
}

// 运行自定义输入弹窗（简易内联层）。
function CustomInputDialog(props: { value: string; onChange: (v: string) => void; onRun: () => void; onClose: () => void }) {
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4" onClick={props.onClose}>
      <div className="w-full max-w-md rounded-xl border bg-background p-4 shadow-lg" onClick={(e) => e.stopPropagation()}>
        <h4 className="mb-2 text-sm font-semibold">运行 · 自定义输入</h4>
        <textarea
          value={props.value}
          onChange={(e) => props.onChange(e.target.value)}
          rows={5}
          className="w-full rounded-lg border bg-background p-2 font-mono text-xs"
          placeholder="输入程序的标准输入内容…"
        />
        <div className="mt-2 flex justify-end gap-2">
          <Button size="sm" variant="outline" onClick={props.onClose}>取消</Button>
          <Button size="sm" onClick={props.onRun}>运行</Button>
        </div>
      </div>
    </div>
  )
}

// ---------- 测评记录（训练内） ----------

function SubmissionHistoryDialog({ open, onOpenChange, problemId, trainingId }: {
  open: boolean
  onOpenChange: (v: boolean) => void
  problemId: number
  trainingId: number
}) {
  const submissionsQ = useQuery({
    queryKey: ['oj-submissions', problemId, trainingId],
    queryFn: () => api.ojSubmissions(problemId, trainingId),
    enabled: open,
  })
  const list = submissionsQ.data?.submissions ?? []
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex max-h-[85vh] flex-col sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>测评记录</DialogTitle>
          <DialogDescription>本训练内该题的提交记录（按训练维度查看）。</DialogDescription>
        </DialogHeader>
        {list.length === 0 ? (
          <div className="p-8 text-center text-sm text-muted-foreground">
            暂无记录
            {submissionsQ.isLoading && <Loader2Icon className="mx-auto mt-2 size-4 animate-spin" />}
          </div>
        ) : (
          <div className="max-h-[60vh] overflow-y-auto">
            {list.map((s) => (
              <HistoryRow key={s.id} sub={s} />
            ))}
          </div>
        )}
        <DialogFooter className="mt-2">
          <Button variant="outline" size="sm" onClick={() => onOpenChange(false)}>关闭</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function HistoryRow({ sub }: { sub: Submission }) {
  return (
    <div className="flex w-full items-center gap-3 border-b px-3 py-2.5 text-left">
      <span className={cn('rounded-md border px-1.5 py-0.5 text-xs font-medium', verdictCls(sub.verdict))}>
        {verdictText(sub.verdict)}
      </span>
      <div className="min-w-0 flex-1">
        <p className="truncate text-xs text-muted-foreground">
          #{sub.id} · {submitTypeText(sub.submitType)} · {langText(sub.language)} · {new Date(sub.createdAt).toLocaleString()}
        </p>
      </div>
      <span className="shrink-0 text-[11px] text-muted-foreground">
        {sub.timeMs}ms{sub.score > 0 ? ` · ${sub.score} 分` : ''}
      </span>
    </div>
  )
}
