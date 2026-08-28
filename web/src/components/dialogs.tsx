import { useRef, useState } from 'react'
import { toast } from 'sonner'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { EyeIcon, FileCodeIcon, FolderPlusIcon, ListChecksIcon, PencilIcon, SparklesIcon, UploadIcon } from 'lucide-react'

import { Button } from '@/components/ui/button'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { api } from '@/lib/api'
import { useAppState } from '@/lib/app-context'
import type { Practice, ProblemType, Training } from '@/lib/types'
import { ProblemEditor } from './ProblemPane'
import { ProblemView } from './problem-view'

// ---------- 删除确认 ----------

export function ConfirmDialog(props: {
  open: boolean
  onOpenChange: (v: boolean) => void
  title: string
  description: string
  onConfirm: () => void
  confirmLabel?: string
}) {
  return (
    <AlertDialog open={props.open} onOpenChange={props.onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{props.title}</AlertDialogTitle>
          <AlertDialogDescription>{props.description}</AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>取消</AlertDialogCancel>
          <AlertDialogAction onClick={props.onConfirm}>{props.confirmLabel ?? '确认删除'}</AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}

// ---------- 新建题目 ----------

export function NewProblemDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (v: boolean) => void }) {
  const { openProblem } = useAppState()
  const [title, setTitle] = useState('')
  const [type, setType] = useState<ProblemType>('programming')
  const qc = useQueryClient()

  const create = useMutation({
    mutationFn: () =>
      api.createProblem({
        type,
        title,
        tags: [],
        statementMd: '',
        bodyJson: {},
        answerJson: {},
      }),
    onSuccess: async (data) => {
      await qc.invalidateQueries({ queryKey: ['problems'] })
      await qc.invalidateQueries({ queryKey: ['tags'] })
      toast.success('题目已创建')
      onOpenChange(false)
      setTitle('')
      openProblem(data.problem.id)
    },
    onError: (e) => toast.error(e.message),
  })

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle>新建题目</DialogTitle>
          <DialogDescription>创建后进入编辑页完善题面、答案与题解。</DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          <div className="space-y-1.5">
            <Label>题目标题</Label>
            <Input value={title} onChange={(e) => setTitle(e.target.value)} placeholder="例如：A+B 问题" autoFocus />
          </div>
          <div className="space-y-1.5">
            <Label>题型</Label>
            <Select
              items={[
                { value: 'programming', label: '编程题' },
                { value: 'single_choice', label: '单选题' },
                { value: 'true_false', label: '判断题' },
              ]}
              value={type}
              onValueChange={(v) => setType(v as ProblemType)}
            >
              <SelectTrigger className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="programming">编程题</SelectItem>
                <SelectItem value="single_choice">单选题</SelectItem>
                <SelectItem value="true_false">判断题</SelectItem>
              </SelectContent>
            </Select>
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            取消
          </Button>
          <Button onClick={() => create.mutate()} disabled={!title.trim() || create.isPending}>
            创建
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

// ---------- 导入 ZIP ----------

export type ImportMode = 'auto' | 'problems' | 'training' | 'practice'

// fixedMode 非空时锁定导入方式（题目栏=仅题目），隐藏模式选择区；
// allow 限定可选模式（题册栏合并入口=自动识别/训练/练习），默认全部。
export function ImportDialog({
  open,
  onOpenChange,
  fixedMode,
  allow,
}: {
  open: boolean
  onOpenChange: (v: boolean) => void
  fixedMode?: ImportMode
  allow?: ImportMode[]
}) {
  const fileRef = useRef<HTMLInputElement>(null)
  const [mode, setMode] = useState<ImportMode>(fixedMode ?? allow?.[0] ?? 'auto')
  const [busy, setBusy] = useState(false)
  const qc = useQueryClient()

  async function submit() {
    const files = fileRef.current?.files
    if (!files || files.length === 0) {
      toast.error('请选择 ZIP 文件')
      return
    }
    setBusy(true)
    let total = 0
    let success = 0
    const names: string[] = []
    try {
      for (let i = 0; i < files.length; i++) {
        const file = files[i]
        try {
          const result = await api.import(file, mode)
          total += (result.imported as unknown[])?.length ?? 0
          success++
          if (result.title) names.push(String(result.title))
        } catch (e) {
          toast.error(`「${file.name}」导入失败：${e instanceof Error ? e.message : '未知错误'}`)
        }
      }
      if (success > 0) {
        const extra =
          names.length > 0
            ? `，题册：${names.slice(0, 3).join('、')}${names.length > 3 ? ` 等 ${names.length} 个` : ''}`
            : ''
        toast.success(`已导入 ${total} 道题目${extra}`)
        await qc.invalidateQueries()
        onOpenChange(false)
      }
    } finally {
      setBusy(false)
    }
  }

  const allModes = [
    { value: 'auto', label: '自动识别', desc: '包内含章节结构→训练，否则→练习（平铺）', icon: SparklesIcon },
    { value: 'problems', label: '仅导入题目', desc: '只把 problems.json 的题目入库', icon: FileCodeIcon },
    { value: 'training', label: '导入为训练', desc: '按 trainingPlan.json 章节结构建训练', icon: FolderPlusIcon },
    { value: 'practice', label: '导入为练习', desc: '按题目顺序建立平铺练习', icon: ListChecksIcon },
  ] as const
  const modes = fixedMode ? [] : allModes.filter((m) => (allow ?? allModes.map((x) => x.value)).includes(m.value))
  const modeLabel: Record<ImportMode, string> = { auto: '自动识别', problems: '仅导入题目', training: '导入为训练', practice: '导入为练习' }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <UploadIcon className="size-4" /> 导入 OrangeOJ ZIP 包
            {fixedMode && <span className="text-sm font-normal text-muted-foreground">（{modeLabel[fixedMode]}）</span>}
          </DialogTitle>
          <DialogDescription>兼容 OrangeOJ 导出格式：problems.json + trainingPlan.json + images/。</DialogDescription>
        </DialogHeader>
        <input
          ref={fileRef}
          type="file"
          accept=".zip"
          multiple
          className="w-full cursor-pointer rounded-lg border border-input bg-transparent p-2 text-sm file:mr-3 file:cursor-pointer file:rounded-md file:border-0 file:bg-muted file:px-3 file:py-1.5 file:text-sm"
        />
        <p className="text-xs text-muted-foreground">支持一次选择多个 ZIP 包；题册名称取包内元数据标题，缺失时用文件名。</p>
        {modes.length > 1 && (
          <div className="space-y-1.5">
            <Label>导入方式</Label>
            <div className="grid gap-1.5">
              {modes.map((m) => (
                <button
                  key={m.value}
                  type="button"
                  onClick={() => setMode(m.value)}
                  className={`flex items-start gap-2.5 rounded-lg border p-2.5 text-left transition-colors ${
                    mode === m.value ? 'border-primary bg-primary/5' : 'hover:bg-muted'
                  }`}
                >
                  <m.icon className={`mt-0.5 size-4 ${mode === m.value ? 'text-primary' : 'text-muted-foreground'}`} />
                  <span>
                    <span className="block text-sm font-medium">{m.label}</span>
                    <span className="block text-xs text-muted-foreground">{m.desc}</span>
                  </span>
                </button>
              ))}
            </div>
          </div>
        )}
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            取消
          </Button>
          <Button onClick={submit} disabled={busy}>
            {busy ? '导入中…' : '开始导入'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

// ---------- 题目预览浮窗（训练/练习条目点击弹出，支持编辑模式） ----------

export function ProblemPreviewDialog(props: {
  open: boolean
  problemId: number | null
  onOpenChange: (v: boolean) => void
}) {
  const [editing, setEditing] = useState(false)
  // 每次打开时回到查看模式
  const [lastOpen, setLastOpen] = useState(false)
  if (props.open && !lastOpen) {
    setLastOpen(true)
    setEditing(false)
  } else if (!props.open && lastOpen) {
    setLastOpen(false)
  }
  const q = useQuery({
    queryKey: ['problem', props.problemId],
    queryFn: () => api.getProblem(props.problemId as number),
    enabled: props.open && props.problemId !== null,
  })
  const problem = q.data?.problem
  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            {editing ? '编辑题目' : '题目预览'}
            {problem && (
              <button
                type="button"
                onClick={() => setEditing((v) => !v)}
                title={editing ? '切换到查看模式' : '切换到编辑模式'}
                className={`inline-flex size-7 items-center justify-center rounded-lg border border-input text-sm transition-colors ${
                  editing ? 'bg-primary/10 text-primary' : 'hover:bg-muted'
                }`}
              >
                {editing ? <EyeIcon className="size-3.5" /> : <PencilIcon className="size-3.5" />}
              </button>
            )}
          </DialogTitle>
        </DialogHeader>
        {problem ? (
          editing ? (
            <ProblemEditor problem={problem} onSaved={() => setEditing(false)} />
          ) : (
            <>
              <h1 className="mb-4 text-xl font-semibold">{problem.title}</h1>
              <ProblemView problem={problem} />
            </>
          )
        ) : (
          <div className="py-8 text-center text-sm text-muted-foreground">加载中…</div>
        )}
      </DialogContent>
    </Dialog>
  )
}

// ---------- 加入训练 / 练习 ----------

export function AddToGroupDialog(props: {
  open: boolean
  onOpenChange: (v: boolean) => void
  kind: 'training' | 'practice'
  problemIds: number[]
}) {
  const isTraining = props.kind === 'training'
  const qc = useQueryClient()
  const { clearChecked } = useAppState()
  const [target, setTarget] = useState<string>('__new__')
  const [newTitle, setNewTitle] = useState('')
  const [busy, setBusy] = useState(false)

  const listQuery = useQuery({
    queryKey: ['group-list', props.kind],
    queryFn: async (): Promise<{ trainings?: Training[]; practices?: Practice[] }> =>
      isTraining ? await api.trainings() : await api.practices(),
    enabled: props.open,
  })
  const groups: { id: number; title: string }[] = isTraining
    ? (listQuery.data?.trainings ?? [])
    : (listQuery.data?.practices ?? [])

  async function submit() {
    setBusy(true)
    try {
      let groupId: number
      if (target === '__new__') {
        const title = newTitle.trim() || (isTraining ? '新训练' : '新练习')
        groupId = isTraining ? (await api.createTraining(title)).id : (await api.createPractice(title)).id
      } else {
        groupId = Number(target)
      }
      if (isTraining) {
        // 默认加入第一个章节；无章节时先建「默认章节」
        const detail = await api.getTraining(groupId)
        let chapterId = detail.chapters[0]?.id
        if (!chapterId) chapterId = (await api.createChapter(groupId, '默认章节')).id
        await api.addChapterItems(chapterId, props.problemIds)
      } else {
        await api.addPracticeItems(groupId, props.problemIds)
      }
      await qc.invalidateQueries({ queryKey: ['trainings'] })
      await qc.invalidateQueries({ queryKey: ['training'] })
      await qc.invalidateQueries({ queryKey: ['practices'] })
      await qc.invalidateQueries({ queryKey: ['practice'] })
      toast.success(`已将 ${props.problemIds.length} 道题目加入${isTraining ? '训练' : '练习'}`)
      clearChecked()
      props.onOpenChange(false)
    } catch (e) {
      toast.error(e instanceof Error ? e.message : '操作失败')
    } finally {
      setBusy(false)
    }
  }

  const items = [
    { value: '__new__', label: `＋ 新建${isTraining ? '训练' : '练习'}…` },
    ...groups.map((g) => ({ value: String(g.id), label: g.title })),
  ]
  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle>加入{isTraining ? '训练' : '练习'}</DialogTitle>
          <DialogDescription>已选择 {props.problemIds.length} 道题目。</DialogDescription>
        </DialogHeader>
        <div className="space-y-1.5">
          <Label>目标{isTraining ? '训练' : '练习'}</Label>
          <Select items={items} value={target} onValueChange={(v) => setTarget(v as string)}>
            <SelectTrigger className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {items.map((it) => (
                <SelectItem key={it.value} value={it.value}>
                  {it.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          {target === '__new__' && (
            <Input
              value={newTitle}
              onChange={(e) => setNewTitle(e.target.value)}
              placeholder={`${isTraining ? '训练' : '练习'}名称`}
              className="mt-1.5"
            />
          )}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => props.onOpenChange(false)}>
            取消
          </Button>
          <Button onClick={submit} disabled={busy || props.problemIds.length === 0}>
            {busy ? '提交中…' : '加入'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

