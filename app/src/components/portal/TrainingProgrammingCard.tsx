// 训练内编程题作答卡：页内直接答题（不跳做题页）——
// 语言选择 + Monaco 编辑器（草稿按题/语言本地保存）+ 运行(自定义输入)/测试(样例)/提交
// + 控制台输出与判定结果；提交 AC 后轮询带 trainingId 使训练条目标记通过（格子变绿）。
import { useEffect, useRef, useState } from 'react'
import { toast } from 'sonner'
import {
  FlaskConicalIcon,
  Loader2Icon,
  PlayIcon,
  SendIcon,
  CheckCircle2Icon,
  XCircleIcon,
} from 'lucide-react'

import { api } from '@/api'
import type { CodeLang, SubmissionPoll } from '@/api/types'
import { CodeEditor } from '@/components/CodeEditor'
import { Button } from '@/components/ui/button'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { cn } from '@/lib/utils'

const DRAFT_PREFIX = 'orangeoj:draft:'
function starterCode(lang: CodeLang): string {
  if (lang === 'python') return '# Python 3\n'
  return '// C++\n#include <bits/stdc++.h>\nusing namespace std;\n\nint main() {\n    \n    return 0;\n}\n'
}

export function TrainingProgrammingCard({ problemId, trainingId, solved, onSolved }: {
  problemId: number
  trainingId: number
  /** 该题当前是否已通过（决定 AC 后是否轮询标记） */
  solved: boolean
  onSolved: () => void
}) {
  const [lang, setLang] = useState<CodeLang>(() => (localStorage.getItem(`${DRAFT_PREFIX}lang-${problemId}`) as CodeLang) || 'python')
  const [code, setCode] = useState(() => localStorage.getItem(`${DRAFT_PREFIX}${problemId}-${lang}`) ?? starterCode(lang))
  const [consoleText, setConsoleText] = useState('控制台已就绪')
  const [consoleVariant, setConsoleVariant] = useState<'default' | 'error' | 'success'>('default')
  const [busy, setBusy] = useState<string | null>(null) // run/test/submit
  const [verdict, setVerdict] = useState<{ verdict: string; score: number; timeMs: number } | null>(null)
  const [showCustomInput, setShowCustomInput] = useState(false)
  const [customInput, setCustomInput] = useState('')
  const codeRef = useRef(code)
  codeRef.current = code

  useEffect(() => {
    localStorage.setItem(`${DRAFT_PREFIX}${problemId}-${lang}`, code)
  }, [code, lang, problemId])

  function switchLang(l: CodeLang) {
    if (l === lang) return
    const prevCode = localStorage.getItem(`${DRAFT_PREFIX}${problemId}-${l}`)
    setLang(l)
    setCode(prevCode ?? starterCode(l))
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
        const r = await api.ojSubmit(problemId, lang, codeRef.current)
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
        <CodeEditor language={lang} value={code} onChange={setCode} />
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
    </div>
  )
}

function verdictText(v: string): string {
  const map: Record<string, string> = {
    AC: '通过 AC', WA: '答案错误 WA', CE: '编译错误 CE', RE: '运行错误 RE',
    TLE: '超时 TLE', MLE: '超内存 MLE', PENDING: '等待评测', FAILED: '评测失败',
  }
  return map[v] ?? v
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
