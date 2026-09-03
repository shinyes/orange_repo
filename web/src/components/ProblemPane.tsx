import { useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import {
  ArrowDownIcon,
  ArrowUpIcon,
  EyeIcon,
  ImageIcon,
  PencilIcon,
  PlusIcon,
  SaveIcon,
  TrashIcon,
} from 'lucide-react'

import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Separator } from '@/components/ui/separator'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import { api } from '@/lib/api'
import { useAppState } from '@/lib/app-context'
import { Markdown } from '@/lib/markdown'
import type {
  Problem,
  ProblemPayload,
  ProblemType,
  ProgrammingBody,
  ProgrammingCase,
  Solution,
} from '@/lib/types'
import { ConfirmDialog } from './dialogs'
import { Empty, ProblemView } from './problem-view'

export function ProblemPane({ id }: { id: number }) {
  const { goHome } = useAppState()
  const qc = useQueryClient()
  const problemQuery = useQuery({ queryKey: ['problem', id], queryFn: () => api.getProblem(id) })
  const [mode, setMode] = useState<'view' | 'edit'>('view') // 默认查看模式
  const [confirmDelete, setConfirmDelete] = useState(false)

  const problem = problemQuery.data?.problem

  const del = useMutation({
    mutationFn: () => api.deleteProblem(id),
    onSuccess: async () => {
      toast.success('题目已删除')
      await qc.invalidateQueries()
      goHome()
    },
    onError: (e) => toast.error(e.message),
  })

  if (problemQuery.isLoading) return <Centered>加载中…</Centered>
  if (problemQuery.isError || !problem) return <Centered>题目加载失败</Centered>

  return (
    <div className="mx-auto max-w-4xl px-6 py-5">
      {/* 头部：标题 + 删除（编辑模式，位于模式切换按钮左侧）+ 模式切换 */}
      <div className="mb-4 flex items-start gap-2">
        <h1 className="min-w-0 flex-1 text-xl font-semibold">{problem.title}</h1>
        {mode === 'edit' && (
          <Button variant="ghost" size="sm" className="text-destructive" onClick={() => setConfirmDelete(true)}>
            <TrashIcon data-icon="inline-start" /> 删除
          </Button>
        )}
        <button
          type="button"
          onClick={() => setMode((v) => (v === 'view' ? 'edit' : 'view'))}
          title={mode === 'view' ? '切换到编辑模式' : '切换到显示模式'}
          className={`inline-flex size-8 items-center justify-center rounded-lg border border-input text-sm transition-colors ${
            mode === 'view' ? 'hover:bg-muted' : 'bg-primary/10 text-primary'
          }`}
        >
          {mode === 'view' ? <PencilIcon className="size-4" /> : <EyeIcon className="size-4" />}
        </button>
      </div>

      {mode === 'edit' ? (
        <ProblemEditor problem={problem} onSaved={() => setMode('view')} />
      ) : (
        <ProblemView problem={problem} />
      )}

      <ConfirmDialog
        open={confirmDelete}
        onOpenChange={setConfirmDelete}
        title={`删除题目「${problem.title}」？`}
        description="该题目将从题库与其所属训练/练习中移除，操作不可撤销。"
        onConfirm={() => del.mutate()}
      />
    </div>
  )
}

function Centered({ children }: { children: React.ReactNode }) {
  return <div className="flex h-full items-center justify-center text-sm text-muted-foreground">{children}</div>
}

// ---------- 编辑器 ----------

interface EditState {
  type: ProblemType
  title: string
  tags: string[]
  statementMd: string
  limits: { time: number; memory: number }
  inputFormat: string
  outputFormat: string
  samples: ProgrammingCase[]
  testCases: ProgrammingCase[]
  options: string[]
  answerIndex: number
  tfAnswer: boolean
  solutions: Solution[]
}

function fromProblem(p: Problem): EditState {
  const body = (p.bodyJson ?? {}) as ProgrammingBody
  const answer = (p.answerJson ?? {}) as Record<string, unknown>
  return {
    type: p.type,
    title: p.title,
    tags: p.tags,
    statementMd: p.statementMd,
    limits: { time: p.timeLimitMs || 1000, memory: p.memoryLimitMiB || 256 },
    inputFormat: body.inputFormat ?? '',
    outputFormat: body.outputFormat ?? '',
    samples: body.samples ?? [],
    testCases: body.testCases ?? [],
    options: ((p.bodyJson as { options?: string[] }).options as string[] | undefined) ?? [],
    answerIndex: typeof answer.answerIndex === 'number' ? answer.answerIndex : 0,
    tfAnswer: answer.answer === true,
    solutions: p.solutions ?? [],
  }
}

export function ProblemEditor({ problem, onSaved }: { problem: Problem; onSaved: () => void }) {
  const qc = useQueryClient()
  const [s, setS] = useState<EditState>(() => fromProblem(problem))
  const [tagInput, setTagInput] = useState('')
  const [previewStatement, setPreviewStatement] = useState(false)
  const statementRef = useRef<HTMLTextAreaElement>(null)
  const imageInputRef = useRef<HTMLInputElement>(null)
  // 表单 / JSON 双模式
  const [editorTab, setEditorTab] = useState<'form' | 'json'>('form')
  const [jsonText, setJsonText] = useState('')
  const patch = (p: Partial<EditState>) => setS((prev) => ({ ...prev, ...p }))

  /** 把表单状态序列化为题目 JSON（视图结构，与保存载荷一致）。 */
  function stateToJson(): Record<string, unknown> {
    let bodyJson: Record<string, unknown> = {}
    let answerJson: Record<string, unknown> = {}
    if (s.type === 'programming') {
      bodyJson = { inputFormat: s.inputFormat, outputFormat: s.outputFormat, samples: s.samples, testCases: s.testCases }
      answerJson = {}
    } else if (s.type === 'single_choice') {
      bodyJson = { options: s.options }
      answerJson = { answerIndex: s.answerIndex }
    } else {
      bodyJson = {}
      answerJson = { answer: s.tfAnswer }
    }
    return {
      type: s.type,
      title: s.title,
      tags: s.tags,
      statementMd: s.statementMd,
      bodyJson,
      answerJson,
      solutions: s.solutions,
      timeLimitMs: s.type === 'programming' ? s.limits.time : undefined,
      memoryLimitMiB: s.type === 'programming' ? s.limits.memory : undefined,
    }
  }

  /** 由表单状态构造保存载荷。 */
  function payloadFromState(): ProblemPayload {
    const j = stateToJson()
    return {
      type: j.type as ProblemType,
      title: j.title as string,
      tags: j.tags as string[],
      statementMd: j.statementMd as string,
      bodyJson: j.bodyJson as Record<string, unknown>,
      answerJson: j.answerJson as Record<string, unknown>,
      solutions: j.solutions as Solution[] | undefined,
      timeLimitMs: j.timeLimitMs as number | undefined,
      memoryLimitMiB: j.memoryLimitMiB as number | undefined,
    }
  }

  function switchTab(tab: 'form' | 'json') {
    if (tab === 'json' && editorTab !== 'json') {
      setJsonText(JSON.stringify(stateToJson(), null, 2))
    }
    setEditorTab(tab)
  }

  /** JSON 模式提交：解析校验 → 题型不可更改 → 构造载荷保存。 */
  function handleJsonSave() {
    let data: Record<string, unknown>
    try {
      data = JSON.parse(jsonText)
    } catch (e) {
      toast.error(`JSON 格式错误：${(e as Error).message}`)
      return
    }
    if (typeof data !== 'object' || data === null || Array.isArray(data)) {
      toast.error('JSON 内容应为对象')
      return
    }
    if (typeof data.type === 'string' && data.type !== s.type) {
      toast.error('题型不可修改：JSON 中的 type 与当前题型不一致')
      return
    }
    const str = (v: unknown, fb: string) => (typeof v === 'string' ? v : fb)
    const arr = (v: unknown, fb: unknown[]) => (Array.isArray(v) ? v : fb)
    const num = (v: unknown, fb: number) => (typeof v === 'number' && Number.isFinite(v) ? v : fb)
    const obj = (v: unknown, fb: Record<string, unknown>) =>
      v && typeof v === 'object' && !Array.isArray(v) ? (v as Record<string, unknown>) : fb
    save.mutate({
      type: s.type, // 恒以当前题型为准
      title: str(data.title, s.title).trim() || s.title,
      tags: arr(data.tags, s.tags).filter((x): x is string => typeof x === 'string'),
      statementMd: str(data.statementMd, s.statementMd),
      bodyJson: obj(data.bodyJson, s.type === 'programming' ? { inputFormat: s.inputFormat, outputFormat: s.outputFormat, samples: s.samples, testCases: s.testCases } : s.type === 'single_choice' ? { options: s.options } : {}),
      answerJson: obj(data.answerJson, s.type === 'single_choice' ? { answerIndex: s.answerIndex } : s.type === 'true_false' ? { answer: s.tfAnswer } : {}),
      solutions: arr(data.solutions, s.solutions) as Solution[],
      timeLimitMs: s.type === 'programming' ? num(data.timeLimitMs, s.limits.time) : undefined,
      memoryLimitMiB: s.type === 'programming' ? num(data.memoryLimitMiB, s.limits.memory) : undefined,
    })
  }

  const save = useMutation({
    mutationFn: (payload: ProblemPayload) => api.updateProblem(problem.id, payload),
    onSuccess: async () => {
      toast.success('已保存')
      await qc.invalidateQueries({ queryKey: ['problem', problem.id] })
      await qc.invalidateQueries({ queryKey: ['problems'] })
      await qc.invalidateQueries({ queryKey: ['tags'] })
      onSaved()
    },
    onError: (e) => toast.error(e.message),
  })

  async function uploadImage(file: File) {
    try {
      const { url } = await api.uploadImage(file)
      const ta = statementRef.current
      const insert = `![图片](${url})`
      if (ta) {
        const start = ta.selectionStart ?? s.statementMd.length
        const end = ta.selectionEnd ?? start
        const next = s.statementMd.slice(0, start) + insert + s.statementMd.slice(end)
        patch({ statementMd: next })
        requestAnimationFrame(() => {
          ta.focus()
          ta.selectionStart = ta.selectionEnd = start + insert.length
        })
      } else {
        patch({ statementMd: s.statementMd + '\n' + insert })
      }
      toast.success('图片已上传并插入')
    } catch (e) {
      toast.error(e instanceof Error ? e.message : '上传失败')
    }
  }

  function addTag() {
    const t = tagInput.trim().replace(/,+$/, '')
    if (t && !s.tags.includes(t)) patch({ tags: [...s.tags, t] })
    setTagInput('')
  }

  // 现存标签作为输入建议（datalist）
  const suggestions = useQuery({
    queryKey: ['tags', 'suggestions'],
    queryFn: () => api.tags(),
    staleTime: 30_000,
  })

  return (
    <div className="space-y-4">
      {/* 编辑模式切换：表单 / JSON */}
      <Tabs value={editorTab} onValueChange={(v) => switchTab(v as 'form' | 'json')}>
        <TabsList>
          <TabsTrigger value="form">表单</TabsTrigger>
          <TabsTrigger value="json">JSON</TabsTrigger>
        </TabsList>
      </Tabs>

      {editorTab === 'json' ? (
        <div className="space-y-2">
          <Textarea
            value={jsonText}
            onChange={(e) => setJsonText(e.target.value)}
            className="min-h-96 w-full font-mono text-xs"
            spellCheck={false}
          />
          <p className="text-xs text-muted-foreground">
            JSON 模式用于精细调整结构（bodyJson / answerJson / solutions 等）；题型 type 不可修改（保存时校验）。
          </p>
        </div>
      ) : (
        <>
      {/* 基础信息 */}
      <div className="space-y-1.5">
        <Label>标题</Label>
        <Input value={s.title} onChange={(e) => patch({ title: e.target.value })} />
      </div>

      {/* 标签：chip 式输入，支持斜杠层级 */}
      <div className="space-y-1.5">
        <Label>标签（支持 / 层级，如 数学/几何；回车或逗号添加）</Label>
        <div className="flex flex-wrap items-center gap-1.5">
          {s.tags.map((t) => (
            <Badge key={t} variant="secondary" className="gap-1">
              {t}
              <button type="button" className="ml-0.5 hover:text-destructive" onClick={() => patch({ tags: s.tags.filter((x) => x !== t) })}>
                ×
              </button>
            </Badge>
          ))}
          <div className="flex gap-1">
            <Input
              value={tagInput}
              list="existing-tags"
              onChange={(e) => {
                // 逗号即添加，支持连续输入多个
                if (e.target.value.includes(',')) {
                  const parts = e.target.value.split(',').map((x) => x.trim()).filter(Boolean)
                  const next = [...s.tags]
                  for (const p of parts) if (!next.includes(p)) next.push(p)
                  patch({ tags: next })
                  setTagInput('')
                } else {
                  setTagInput(e.target.value)
                }
              }}
              onKeyDown={(e) => {
                if (e.key === 'Enter') {
                  e.preventDefault()
                  addTag()
                } else if (e.key === 'Backspace' && tagInput === '' && s.tags.length > 0) {
                  patch({ tags: s.tags.slice(0, -1) })
                }
              }}
              placeholder="例如 数学/几何"
              className="h-7 w-44 text-xs"
            />
            <Button size="xs" variant="outline" onClick={addTag}>
              添加
            </Button>
            <datalist id="existing-tags">
              {(suggestions.data?.tags ?? []).map((t) => (
                <option key={t.tag} value={t.tag} />
              ))}
            </datalist>
          </div>
        </div>
      </div>

      {/* 题面 */}
      <div className="space-y-1.5">
        <div className="flex items-center">
          <Label>题面（Markdown，支持 $KaTeX$ 公式与图片）</Label>
          <div className="ml-auto flex gap-1">
            <Button size="xs" variant="ghost" onClick={() => imageInputRef.current?.click()}>
              <ImageIcon data-icon="inline-start" /> 插入图片
            </Button>
            <Button size="xs" variant="ghost" onClick={() => setPreviewStatement(!previewStatement)}>
              <EyeIcon data-icon="inline-start" /> {previewStatement ? '编辑' : '预览'}
            </Button>
          </div>
        </div>
        <input
          ref={imageInputRef}
          type="file"
          accept="image/*"
          className="hidden"
          onChange={(e) => {
            const f = e.target.files?.[0]
            if (f) void uploadImage(f)
            e.target.value = ''
          }}
        />
        {previewStatement ? (
          <div className="min-h-40 rounded-lg border p-3">
            <Markdown text={s.statementMd || '（空）'} className="markdown-body text-sm" />
          </div>
        ) : (
          <Textarea
            ref={statementRef}
            value={s.statementMd}
            onChange={(e) => patch({ statementMd: e.target.value })}
            className="min-h-40 font-mono text-xs"
          />
        )}
      </div>

      {/* 题型内容 */}
      {s.type === 'programming' && (
        <ProgrammingEditor s={s} patch={patch} />
      )}
      {s.type === 'single_choice' && <ChoiceEditor s={s} patch={patch} />}
      {s.type === 'true_false' && (
        <div className="space-y-1.5">
          <Label>正确答案</Label>
          <div className="flex gap-2">
            <Button size="sm" variant={s.tfAnswer ? 'default' : 'outline'} onClick={() => patch({ tfAnswer: true })}>
              ✓ 正确
            </Button>
            <Button size="sm" variant={!s.tfAnswer ? 'default' : 'outline'} onClick={() => patch({ tfAnswer: false })}>
              ✗ 错误
            </Button>
          </div>
        </div>
      )}

      <Separator />

      {/* 题解 */}
      <SolutionsEditor solutions={s.solutions} onChange={(solutions) => patch({ solutions })} />
        </>
      )}

      {/* 保存（表单 / JSON 共用底条） */}
      <div className="sticky bottom-0 flex justify-end gap-2 border-t bg-background py-3">
        {editorTab === 'json' ? (
          <>
            <Button variant="outline" onClick={() => setJsonText(JSON.stringify(stateToJson(), null, 2))}>
              重置
            </Button>
            <Button onClick={handleJsonSave} disabled={save.isPending}>
              <SaveIcon data-icon="inline-start" /> {save.isPending ? '保存中…' : '保存题目'}
            </Button>
          </>
        ) : (
          <>
            <Button variant="outline" onClick={() => setS(fromProblem(problem))}>
              重置
            </Button>
            <Button onClick={() => save.mutate(payloadFromState())} disabled={save.isPending || !s.title.trim()}>
              <SaveIcon data-icon="inline-start" /> {save.isPending ? '保存中…' : '保存题目'}
            </Button>
          </>
        )}
      </div>
    </div>
  )
}

// ---------- 编程题编辑 ----------

function ProgrammingEditor({ s, patch }: { s: EditState; patch: (p: Partial<EditState>) => void }) {
  return (
    <div className="space-y-3">
      <div className="grid gap-3 sm:grid-cols-2">
        <div className="space-y-1.5">
          <Label>输入格式</Label>
          <Textarea value={s.inputFormat} onChange={(e) => patch({ inputFormat: e.target.value })} className="min-h-16" />
        </div>
        <div className="space-y-1.5">
          <Label>输出格式</Label>
          <Textarea value={s.outputFormat} onChange={(e) => patch({ outputFormat: e.target.value })} className="min-h-16" />
        </div>
      </div>
      <CasesEditor label="样例" cases={s.samples} onChange={(samples) => patch({ samples })} />
      <CasesEditor label="测试点" cases={s.testCases} onChange={(testCases) => patch({ testCases })} />
      <div className="flex gap-3">
        <div className="w-40 space-y-1.5">
          <Label>时限（ms）</Label>
          <Input type="number" min={100} value={s.limits.time} onChange={(e) => patch({ limits: { ...s.limits, time: Number(e.target.value) || 1000 } })} />
        </div>
        <div className="w-40 space-y-1.5">
          <Label>内存（MiB）</Label>
          <Input type="number" min={16} value={s.limits.memory} onChange={(e) => patch({ limits: { ...s.limits, memory: Number(e.target.value) || 256 } })} />
        </div>
      </div>
    </div>
  )
}

function CasesEditor({ label, cases, onChange }: { label: string; cases: ProgrammingCase[]; onChange: (c: ProgrammingCase[]) => void }) {
  return (
    <div className="space-y-1.5">
      <div className="flex items-center">
        <Label>{label}</Label>
        <Button size="xs" variant="ghost" className="ml-auto" onClick={() => onChange([...cases, { input: '', output: '' }])}>
          <PlusIcon data-icon="inline-start" /> 添加
        </Button>
      </div>
      <div className="space-y-2">
        {cases.map((c, i) => (
          <div key={i} className="grid gap-2 rounded-lg border p-2 sm:grid-cols-2">
            <div className="flex items-start gap-1">
              <span className="pt-2 text-xs text-muted-foreground">入</span>
              <Textarea value={c.input} onChange={(e) => onChange(cases.map((x, j) => (j === i ? { ...x, input: e.target.value } : x)))} className="min-h-16 font-mono text-xs" placeholder="输入" />
            </div>
            <div className="flex items-start gap-1">
              <span className="pt-2 text-xs text-muted-foreground">出</span>
              <Textarea value={c.output} onChange={(e) => onChange(cases.map((x, j) => (j === i ? { ...x, output: e.target.value } : x)))} className="min-h-16 font-mono text-xs" placeholder="期望输出" />
            </div>
            <Button size="icon-xs" variant="ghost" className="col-span-full w-fit text-destructive" onClick={() => onChange(cases.filter((_, j) => j !== i))}>
              <TrashIcon />
            </Button>
          </div>
        ))}
        {cases.length === 0 && <Empty>暂无{label}</Empty>}
      </div>
    </div>
  )
}

// ---------- 单选题编辑 ----------

function ChoiceEditor({ s, patch }: { s: EditState; patch: (p: Partial<EditState>) => void }) {
  return (
    <div className="space-y-3">
      <div className="space-y-1.5">
        <div className="flex items-center">
          <Label>选项（点击选择正确答案）</Label>
          <Button size="xs" variant="ghost" className="ml-auto" onClick={() => patch({ options: [...s.options, ''] })}>
            <PlusIcon data-icon="inline-start" /> 添加选项
          </Button>
        </div>
        <div className="space-y-1.5">
          {s.options.map((opt, i) => (
            <div key={i} className="flex items-start gap-2">
              <button
                type="button"
                onClick={() => patch({ answerIndex: i })}
                className={`mt-1.5 flex size-6 shrink-0 items-center justify-center rounded-full text-xs ${
                  i === s.answerIndex ? 'bg-primary text-primary-foreground' : 'bg-muted text-muted-foreground'
                }`}
                title="设为正确答案"
              >
                {String.fromCharCode(65 + i)}
              </button>
              <Textarea
                value={opt}
                onChange={(e) => patch({ options: s.options.map((x, j) => (j === i ? e.target.value : x)) })}
                placeholder={`选项 ${String.fromCharCode(65 + i)}（支持多行，回车换行）`}
                rows={1}
                className="min-h-9 flex-1 resize-y py-1.5 text-sm leading-relaxed"
              />
              <Button size="icon-xs" variant="ghost" className="mt-1 text-destructive" onClick={() => {
                const options = s.options.filter((_, j) => j !== i)
                patch({ options, answerIndex: Math.min(s.answerIndex, Math.max(0, options.length - 1)) })
              }}>
                <TrashIcon />
              </Button>
            </div>
          ))}
          {s.options.length === 0 && <Empty>请添加至少两个选项</Empty>}
        </div>
      </div>
      <div className="text-xs text-muted-foreground">当前正确答案：{String.fromCharCode(65 + s.answerIndex)}</div>
    </div>
  )
}

// ---------- 题解编辑 ----------

const LANGS = ['cpp', 'python', 'go', 'turtle']

function SolutionsEditor({ solutions, onChange }: { solutions: Solution[]; onChange: (s: Solution[]) => void }) {
  const list = solutions ?? []
  function update(i: number, p: Partial<Solution>) {
    onChange(list.map((x, j) => (j === i ? { ...x, ...p } : x)))
  }
  function move(i: number, dir: -1 | 1) {
    const j = i + dir
    if (j < 0 || j >= list.length) return
    const next = [...list]
    ;[next[i], next[j]] = [next[j], next[i]]
    onChange(next)
  }
  return (
    <div className="space-y-1.5">
      <div className="flex items-center">
        <Label>题解（可多个）</Label>
        <Button size="xs" variant="ghost" className="ml-auto" onClick={() => onChange([...list, { language: 'cpp', code: '', markdown: '' }])}>
          <PlusIcon data-icon="inline-start" /> 添加题解
        </Button>
      </div>
      <div className="space-y-3">
        {list.map((sol, i) => (
          <SolCard key={i} index={i} total={list.length} sol={sol} onChange={(p) => update(i, p)} onRemove={() => onChange(list.filter((_, j) => j !== i))} onMove={(d) => move(i, d)} />
        ))}
        {list.length === 0 && <Empty>暂无题解</Empty>}
      </div>
    </div>
  )
}

function SolCard(props: {
  index: number
  total: number
  sol: Solution
  onChange: (p: Partial<Solution>) => void
  onRemove: () => void
  onMove: (dir: -1 | 1) => void
}) {
  const [preview, setPreview] = useState(false)
  return (
    <div className="rounded-xl border p-3">
      <div className="mb-2 flex items-center gap-2">
        <Select
          items={[...LANGS.map((l) => ({ value: l, label: l })), { value: props.sol.language, label: props.sol.language }].filter(
            (x, i, arr) => arr.findIndex((y) => y.value === x.value) === i,
          )}
          value={props.sol.language}
          onValueChange={(v) => props.onChange({ language: v as string })}
        >
          <SelectTrigger size="sm">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {[...LANGS, ...(!LANGS.includes(props.sol.language) ? [props.sol.language] : [])].map((l) => (
              <SelectItem key={l} value={l}>
                {l}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <div className="ml-auto flex items-center gap-0.5">
          <Button size="icon-xs" variant="ghost" disabled={props.index === 0} onClick={() => props.onMove(-1)}>
            <ArrowUpIcon />
          </Button>
          <Button size="icon-xs" variant="ghost" disabled={props.index === props.total - 1} onClick={() => props.onMove(1)}>
            <ArrowDownIcon />
          </Button>
          <Button size="icon-xs" variant="ghost" className="text-destructive" onClick={props.onRemove}>
            <TrashIcon />
          </Button>
        </div>
      </div>
      <div className="space-y-2">
        <div className="flex items-center">
          <span className="text-xs text-muted-foreground">解读 Markdown</span>
          <Button size="xs" variant="ghost" className="ml-auto" onClick={() => setPreview(!preview)}>
            <EyeIcon data-icon="inline-start" /> {preview ? '编辑' : '预览'}
          </Button>
        </div>
        {preview ? (
          <div className="min-h-16 rounded-lg border p-2.5">
            <Markdown text={props.sol.markdown || '（空）'} className="markdown-body text-sm" />
          </div>
        ) : (
          <Textarea value={props.sol.markdown} onChange={(e) => props.onChange({ markdown: e.target.value })} className="min-h-16 text-xs" placeholder="解题思路…" />
        )}
        <Textarea
          value={props.sol.code}
          onChange={(e) => props.onChange({ code: e.target.value })}
          className="min-h-32 font-mono text-xs"
          placeholder="参考代码…"
        />
      </div>
    </div>
  )
}
