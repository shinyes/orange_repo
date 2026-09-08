// 训练内编程题作答卡：页内直接答题（不跳做题页）——
// 语言选择 + Monaco 编辑器（草稿本地实时保存 + 云端 debounce 自动保存，初始代码
// 本地 → 云草稿 → 题目模板(starterPy/starterCpp) → 通用模板）+ 运行(自定义输入)/测试/提交
// + 控制台输出与判定结果；提交带 trainingId（服务端落 submissions.training_id），
// 提交 AC 后轮询带 trainingId 使训练条目标记通过（格子变绿）。
// 测评记录：工具栏 History 按钮 → Dialog 拉该训练×题提交历史（ojSubmissions(id, trainingId)）。
import { useEffect, useRef, useState } from 'react'
import { toast } from 'sonner'
import {
  ChevronLeftIcon,
  ClipboardIcon,
  FlaskConicalIcon,
  HistoryIcon,
  Loader2Icon,
  PlayIcon,
  SendIcon,
} from 'lucide-react'
import { useQuery } from '@tanstack/react-query'

import { api } from '@/api'
import type { CaseDetail, CodeLang, Submission, SubmissionPoll } from '@/api/types'
import { CodeEditor } from '@/components/CodeEditor'
import { Button } from '@/components/ui/button'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import {
  Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle,
} from '@/components/ui/dialog'
import { cn } from '@/lib/utils'
import { genericStarter, resolveStarter, saveDraftDebounced, useCloudDraft } from '@/lib/use-programming-workspace'

const DRAFT_PREFIX = 'orangeoj:draft:'

// 草稿按 训练×题 隔离（同题在不同训练各自保存）
function draftKey(problemId: number, lang: CodeLang, trainingId: number) {
  return `${DRAFT_PREFIX}t${trainingId}-${problemId}-${lang}`
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

  const [lang, setLang] = useState<CodeLang>(() => (localStorage.getItem(`${DRAFT_PREFIX}lang-${trainingId}-${problemId}`) as CodeLang) || 'python')
  // 初始 code：本地草稿 →（下方 reconcile）云草稿 → 题目模板 → 通用模板
  const [code, setCode] = useState(() => localStorage.getItem(draftKey(problemId, lang, trainingId)) ?? genericStarter(lang))
  const [consoleText, setConsoleText] = useState('控制台已就绪')
  const [consoleVariant, setConsoleVariant] = useState<'default' | 'error' | 'success'>('default')
  const [busy, setBusy] = useState<string | null>(null) // run/test/submit
    const [showCustomInput, setShowCustomInput] = useState(false)
  const [customInput, setCustomInput] = useState('')
  const [historyOpen, setHistoryOpen] = useState(false)
  const codeRef = useRef(code)
  codeRef.current = code
  const touchedRef = useRef(false)

  // 云端草稿：本地为空且用户未输入时，随 cloudLoaded/题目数据到达逐级回填（云草稿 → 题目模板 → 通用）；
  // 仅云草稿回填写本地草稿，模板本身不落本地（避免挡住后续云草稿）。
  const cloudDraft = useCloudDraft(problemId, lang, 'training', trainingId)
  useEffect(() => {
    if (touchedRef.current) return
    const local = localStorage.getItem(draftKey(problemId, lang, trainingId))
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
      localStorage.setItem(draftKey(problemId, lang, trainingId), cloudDraft.initialCode)
    }
  }, [code, lang, cloudDraft.cloudLoaded, cloudDraft.initialCode, problemQ.data, problemId, trainingId])

  // 云端草稿到达但用户已先输入（慢网竞态）：保留当前编辑并提示，避免云草稿被静默丢弃/覆盖
  const draftWarnedRef = useRef(false)
  useEffect(() => {
    if (!draftWarnedRef.current && touchedRef.current && cloudDraft.cloudLoaded
      && cloudDraft.initialCode && cloudDraft.initialCode.trim() !== '' && cloudDraft.initialCode !== code) {
      draftWarnedRef.current = true
      toast.warning('检测到云端草稿（其它页面/设备保存过）：保留当前编辑，后续保存将覆盖云端草稿')
    }
  }, [code, cloudDraft.cloudLoaded, cloudDraft.initialCode])

  // 编辑器输入：实时写本地草稿 + 云端 debounce 自动保存（失败静默，本地已缓存）。
  function handleCodeChange(next: string) {
    touchedRef.current = true
    setCode(next)
    localStorage.setItem(draftKey(problemId, lang, trainingId), next)
    saveDraftDebounced(problemId, lang, next, 'training', trainingId)
  }

  function switchLang(l: CodeLang) {
    if (l === lang) return
    touchedRef.current = false
    draftWarnedRef.current = false
    setLang(l)
    setCode(localStorage.getItem(draftKey(problemId, l, trainingId)) ?? genericStarter(l))
    localStorage.setItem(`${DRAFT_PREFIX}lang-${trainingId}-${problemId}`, l)
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
      </div>

      {/* 判定结果不单独横幅展示（避免编辑器上方遮挡）：控制台文本 + 左侧导航绿/红格已反馈 */}

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

// ---------- 测评记录（训练内；列表 + 展开详情，参考 CodingPage 交互） ----------

function SubmissionHistoryDialog({ open, onOpenChange, problemId, trainingId }: {
  open: boolean
  onOpenChange: (v: boolean) => void
  problemId: number
  trainingId: number
}) {
  const submissionsQ = useQuery({
    queryKey: ['oj-submissions', 't', problemId, trainingId],
    queryFn: () => api.ojSubmissions(problemId, trainingId),
    enabled: open,
  })
  const list = submissionsQ.data?.submissions ?? []
  const [selected, setSelected] = useState<Submission | null>(null)
  // 打开时回到列表
  const lastOpen = useRef(false)
  if (open && !lastOpen.current) {
    lastOpen.current = true
    setSelected(null)
  } else if (!open && lastOpen.current) {
    lastOpen.current = false
  }
  const selectedSub = selected ?? null

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex max-h-[85vh] flex-col sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            {selectedSub && (
              <Button size="icon-xs" variant="ghost" title="返回列表" onClick={() => setSelected(null)}>
                <ChevronLeftIcon className="size-4" />
              </Button>
            )}
            {selectedSub ? `提交 #${selectedSub.id}` : '测评记录'}
          </DialogTitle>
          <DialogDescription>
            {selectedSub ? '该次提交的代码与逐用例判定结果' : '本训练内该题的提交记录（点击条目查看详情）'}
          </DialogDescription>
        </DialogHeader>

        {selectedSub ? (
          <SubmissionDetail sub={selectedSub} />
        ) : list.length === 0 ? (
          <div className="p-8 text-center text-sm text-muted-foreground">
            暂无测评记录
            <p className="mt-1 text-xs opacity-70">点击「测试」或「运行」提交代码后，测评记录将在这里显示</p>
            {submissionsQ.isLoading && <Loader2Icon className="mx-auto mt-2 size-4 animate-spin" />}
          </div>
        ) : (
          <div className="max-h-[60vh] overflow-y-auto">
            {list.map((s) => (
              <HistoryRow key={s.id} sub={s} onClick={() => setSelected(s)} />
            ))}
          </div>
        )}

        <DialogFooter className="mt-2">
          <Button variant="outline" size="sm" onClick={() => (selectedSub ? setSelected(null) : onOpenChange(false))}>
            {selectedSub ? '返回列表' : '关闭'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

// 列表行：灰色图标 + 「提交 #id · verdict」主行 + 元信息副行（verdict 纯文本，无彩色徽章）
function HistoryRow({ sub, onClick }: { sub: Submission; onClick: () => void }) {
  const cases = sub.caseDetails ?? []
  const passCount = cases.filter((c) => c.verdict === 'AC' || c.verdict === 'OK').length
  return (
    <button
      type="button"
      onClick={onClick}
      className="flex w-full items-center gap-3 border-b px-3 py-2.5 text-left transition-colors last:border-b-0 hover:bg-accent/60"
    >
      <HistoryIcon className="size-4 shrink-0 text-muted-foreground/60" />
      <div className="min-w-0 flex-1">
        <p className="truncate text-sm font-medium">
          提交 #{sub.id} · {verdictText(sub.verdict)}
        </p>
        <p className="truncate text-xs text-muted-foreground">
          {submitTypeText(sub.submitType)} · {langText(sub.language)} · {new Date(sub.createdAt).toLocaleString()}
          {sub.timeMs > 0 || sub.memoryKiB > 0 ? ` · ${sub.timeMs}ms · ${sub.memoryKiB}KiB` : ''}
          {cases.length > 0 ? ` · 测试点 ${passCount}/${cases.length}` : ''}
        </p>
      </div>
    </button>
  )
}

// 详情视图：顶部三徽标 + 逐用例切换 + 代码/输入/输出/预期输出/错误 Tabs
function SubmissionDetail({ sub }: { sub: Submission }) {
  const cases = sub.caseDetails ?? []
  const passCount = cases.filter((c) => c.verdict === 'AC' || c.verdict === 'OK').length
  const [caseIdx, setCaseIdx] = useState(0)
  const [tab, setTab] = useState('code')
  const cur: CaseDetail | undefined = cases[caseIdx]

  const verdictVariant = (v: string) =>
    v === 'AC' || v === 'OK' ? 'bg-emerald-50 text-emerald-700' : 'bg-red-50 text-red-700'
  const badgeCount = (text: string, variant: string) => (
    <span className={cn('rounded-md border px-1.5 py-0.5 text-[11px] font-medium', variant)}>{text}</span>
  )

  const tabs: { key: string; label: string; content: string; empty: string; tone?: 'err' | 'ok' }[] = [
    { key: 'code', label: '代码', content: sub.sourceCode ?? '', empty: '无代码' },
    { key: 'input', label: '输入', content: cur?.input ?? '', empty: '（空）' },
    { key: 'output', label: '输出', content: cur?.output ?? sub.stdout ?? '', empty: '（无输出）', tone: 'err' },
    { key: 'expected', label: '预期输出', content: cur?.expectedOutput ?? '', empty: '（无预期输出）', tone: 'ok' },
    { key: 'error', label: '错误', content: cur?.error ?? sub.stderr ?? '', empty: '（无错误）', tone: 'err' },
  ]
  const active = tabs.find((t) => t.key === tab) ?? tabs[0]

  return (
    <div className="flex min-h-0 flex-col gap-3">
      <div className="flex flex-wrap items-center gap-1.5">
        {badgeCount(`测试点 ${cases.length} 个`, 'border-border bg-background text-muted-foreground')}
        {badgeCount(`通过 ${passCount} 个`, 'border-emerald-200 bg-emerald-50 text-emerald-700')}
        {badgeCount(`未通过 ${cases.length - passCount} 个`, 'border-red-200 bg-red-50 text-red-700')}
        <span className={cn('ml-auto rounded-md border px-2 py-0.5 text-xs font-semibold', verdictVariant(sub.verdict))}>
          {verdictText(sub.verdict)}
        </span>
      </div>

      {cases.length > 1 && (
        <div className="flex flex-wrap gap-1.5">
          {cases.map((c, i) => (
            <button
              key={c.caseNo}
              type="button"
              onClick={() => setCaseIdx(i)}
              className={cn(
                'rounded-md border px-2 py-1 text-xs transition-colors',
                i === caseIdx ? 'border-primary bg-primary/10 font-medium text-primary' : 'text-muted-foreground hover:bg-muted',
              )}
            >
              测试点 {c.caseNo} · {verdictText(c.verdict)}
            </button>
          ))}
        </div>
      )}

      <div className="flex flex-wrap items-center gap-1 border-b pb-1">
        {tabs.map((t) => (
          <button
            key={t.key}
            type="button"
            onClick={() => setTab(t.key)}
            className={cn(
              'rounded-md px-2 py-1 text-xs transition-colors',
              tab === t.key ? 'bg-muted font-medium text-foreground' : 'text-muted-foreground hover:text-foreground',
            )}
          >
            {t.label}
          </button>
        ))}
        {tab === 'code' && active.content && (
          <button
            type="button"
            className="ml-auto inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-primary"
            onClick={() => {
              void navigator.clipboard?.writeText(active.content).then(() => toast.success('代码已复制'))
            }}
          >
            <ClipboardIcon className="size-3.5" /> 复制
          </button>
        )}
      </div>

      <pre
        className={cn(
          'max-h-[38vh] overflow-auto whitespace-pre-wrap rounded-lg border p-3 font-mono text-xs leading-relaxed',
          !active.content && 'text-muted-foreground',
          active.tone === 'err' && active.content && 'border-red-200 bg-red-50 text-red-700',
          active.tone === 'ok' && active.content && 'border-emerald-200 bg-emerald-50 text-emerald-700',
          active.tone === undefined && 'bg-muted/40',
        )}
      >
        {active.content || active.empty}
      </pre>
    </div>
  )
}
