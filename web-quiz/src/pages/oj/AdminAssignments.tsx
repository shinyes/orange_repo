import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  BarChart3Icon,
  FolderKanbanIcon,
  ClipboardListIcon,
  Globe2Icon,
  PencilIcon,
  PlusIcon,
  Trash2Icon,
  UsersIcon,
  EyeIcon,
  EyeOffIcon,
} from 'lucide-react'
import { toast } from 'sonner'

import { api } from '@/lib/api'
import type { AdminAssignment, AdminStudent, AssignmentStats } from '@/lib/types'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Label } from '@/components/ui/label'
import { Checkbox } from '@/components/ui/checkbox'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle,
} from '@/components/ui/dialog'
import {
  AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription,
  AlertDialogFooter, AlertDialogHeader, AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { cn } from '@/lib/utils'

// 布置管理（管理员）：从主库训练/练习布置给学生，支持发布/撤回、定向/全体、统计。
export function AssignmentsTab() {
  const qc = useQueryClient()
  const list = useQuery({ queryKey: ['admin-assignments'], queryFn: () => api.adminAssignments() })
  const [kind, setKind] = useState<'training' | 'practice'>('training')
  const [creating, setCreating] = useState(false)
  const [statsFor, setStatsFor] = useState<AdminAssignment | null>(null)
  const [editing, setEditing] = useState<AdminAssignment | null>(null)
  const [deleting, setDeleting] = useState<AdminAssignment | null>(null)
  const assigns = list.data?.assignments ?? []
  const visible = assigns.filter((a) => a.kind === kind)

  async function togglePublish(a: AdminAssignment) {
    try {
      await api.updateAssignment(a.id, { published: !a.published })
      toast.success(a.published ? '已撤回（学生端隐藏）' : '已发布')
      void qc.invalidateQueries({ queryKey: ['admin-assignments'] })
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '操作失败')
    }
  }

  return (
    <div className="space-y-3 pt-4">
      <div className="flex items-center gap-2">
        <Tabs value={kind} onValueChange={(v) => setKind(v as 'training' | 'practice')} className="flex-1">
          <TabsList className="w-full">
            <TabsTrigger value="training" className="flex-1">训练</TabsTrigger>
            <TabsTrigger value="practice" className="flex-1">练习</TabsTrigger>
          </TabsList>
        </Tabs>
        <Button onClick={() => setCreating(true)} className="shrink-0">
          <PlusIcon className="size-4" /> 布置{kind === 'training' ? '训练' : '练习'}
        </Button>
      </div>
      <p className="text-xs text-muted-foreground">
        从主站题库中的训练/练习选择布置；训练按章节、练习按题单对学生呈现，编程题支持评测。
      </p>

      <div className="space-y-2.5">
        {visible.map((a) => (
          <AssignmentCard
            key={a.id}
            a={a}
            onTogglePublish={() => void togglePublish(a)}
            onStats={() => setStatsFor(a)}
            onEdit={() => setEditing(a)}
            onDelete={() => setDeleting(a)}
          />
        ))}
        {visible.length === 0 && (
          <div className="rounded-xl border border-dashed p-8 text-center text-sm text-muted-foreground">
            暂无{kind === 'training' ? '训练' : '练习'}布置
          </div>
        )}
      </div>

      {creating && (
        <CreateAssignmentDialog kind={kind} onDone={() => { setCreating(false); void qc.invalidateQueries({ queryKey: ['admin-assignments'] }) }} onClose={() => setCreating(false)} />
      )}
      {editing && (
        <EditStudentsDialog a={editing} onDone={() => { setEditing(null); void qc.invalidateQueries({ queryKey: ['admin-assignments'] }) }} onClose={() => setEditing(null)} />
      )}
      {statsFor && <StatsDialog a={statsFor} onClose={() => setStatsFor(null)} />}
      <AlertDialog open={deleting !== null} onOpenChange={(v) => { if (!v) setDeleting(null) }}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>删除布置「{deleting?.title}」？</AlertDialogTitle>
            <AlertDialogDescription>学生将立即无法看到该任务；历史提交记录保留。此操作不可撤销。</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <AlertDialogAction
              className="bg-red-600 hover:bg-red-700"
              onClick={async () => {
                if (!deleting) return
                try {
                  await api.deleteAssignment(deleting.id)
                  toast.success('布置已删除')
                  void qc.invalidateQueries({ queryKey: ['admin-assignments'] })
                } catch (err) {
                  toast.error(err instanceof Error ? err.message : '删除失败')
                }
                setDeleting(null)
              }}
            >
              删除
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}

function AssignmentCard({ a, onTogglePublish, onStats, onEdit, onDelete }: {
  a: AdminAssignment
  onTogglePublish: () => void
  onStats: () => void
  onEdit: () => void
  onDelete: () => void
}) {
  return (
    <div className={cn('rounded-xl border bg-card p-3.5', !a.published && 'opacity-70')}>
      <div className="flex items-start gap-2">
        {a.kind === 'training' ? <FolderKanbanIcon className="mt-0.5 size-4 shrink-0 text-primary" /> : <ClipboardListIcon className="mt-0.5 size-4 shrink-0 text-primary" />}
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-1.5">
            <span className="truncate text-sm font-medium">{a.title}</span>
            <Badge variant={a.published ? 'default' : 'secondary'} className="text-[10px]">
              {a.published ? '已发布' : '已撤回'}
            </Badge>
            {a.assignedAll ? (
              <Badge variant="outline" className="text-[10px]"><Globe2Icon className="mr-0.5 size-3" />全体学生</Badge>
            ) : (
              <Badge variant="outline" className="text-[10px]"><UsersIcon className="mr-0.5 size-3" />定向 {a.studentCount} 人</Badge>
            )}
          </div>
          <div className="mt-1 text-xs text-muted-foreground">
            {a.problemCount} 题 · 创建于 {a.createdAt.slice(0, 10)}
          </div>
        </div>
        <div className="flex shrink-0 gap-0.5">
          <Button variant="ghost" size="icon" className="size-8" onClick={onStats} title="每题统计">
            <BarChart3Icon className="size-4" />
          </Button>
          <Button variant="ghost" size="icon" className="size-8" onClick={onTogglePublish} title={a.published ? '撤回' : '发布'}>
            {a.published ? <EyeOffIcon className="size-4" /> : <EyeIcon className="size-4" />}
          </Button>
          <Button variant="ghost" size="icon" className="size-8" onClick={onEdit} title="编辑学生">
            <PencilIcon className="size-4" />
          </Button>
          <Button variant="ghost" size="icon" className="size-8 text-red-600 hover:text-red-600" onClick={onDelete} title="删除">
            <Trash2Icon className="size-4" />
          </Button>
        </div>
      </div>
    </div>
  )
}

// ---------------- 创建布置 ----------------

function CreateAssignmentDialog({ kind, onDone, onClose }: { kind: 'training' | 'practice'; onDone: () => void; onClose: () => void }) {
  const trainings = useQuery({ queryKey: ['repo-trainings'], queryFn: api.repoTrainings })
  const practices = useQuery({ queryKey: ['repo-practices'], queryFn: api.repoPractices })
  const students = useQuery({ queryKey: ['admin-students'], queryFn: api.students })
  const repoList = kind === 'training' ? (trainings.data?.trainings ?? []) : (practices.data?.practices ?? [])

  const [repoId, setRepoId] = useState<number | 0>(0)
  const [assignedAll, setAssignedAll] = useState(true)
  const [selected, setSelected] = useState<number[]>([])
  const [publish, setPublish] = useState(true)
  const [busy, setBusy] = useState(false)

  const selectedRepo = repoList.find((r) => r.id === repoId)

  function toggleStudent(id: number) {
    setSelected((s) => (s.includes(id) ? s.filter((x) => x !== id) : [...s, id]))
  }

  async function create() {
    if (!repoId || busy) return
    if (!assignedAll && selected.length === 0) {
      toast.error('请至少选择一名定向学生，或改为全体')
      return
    }
    setBusy(true)
    try {
      await api.createAssignment({ kind, repoId, assignedAll, published: publish, studentIds: selected })
      toast.success('布置成功')
      onDone()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '布置失败')
    } finally {
      setBusy(false)
    }
  }

  return (
    <Dialog open onOpenChange={(v) => { if (!v) onClose() }}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>布置{kind === 'training' ? '训练' : '练习'}</DialogTitle>
          <DialogDescription>从主站题库中选择；学生端即时可见（未发布则先隐藏）</DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <div className="space-y-1.5">
            <Label>选择主库{kind === 'training' ? '训练' : '练习'}</Label>
            <div className="max-h-48 space-y-1 overflow-y-auto rounded-lg border p-1.5">
              {repoList.map((r) => (
                <button
                  key={r.id}
                  type="button"
                  onClick={() => setRepoId(r.id)}
                  className={cn(
                    'flex w-full items-center justify-between rounded-lg px-2.5 py-2 text-left text-sm',
                    repoId === r.id ? 'bg-primary/10 text-primary' : 'hover:bg-muted',
                  )}
                >
                  <span className="min-w-0 flex-1 truncate">{r.title}</span>
                  <span className="ml-2 shrink-0 text-xs text-muted-foreground">
                    {kind === 'training' ? `${(r as { chapterCount: number }).chapterCount} 章 · ` : ''}{r.problemCount} 题
                  </span>
                </button>
              ))}
              {repoList.length === 0 && <p className="p-3 text-center text-xs text-muted-foreground">主库暂无{kind === 'training' ? '训练' : '练习'}，请先到主站题库创建</p>}
            </div>
          </div>

          <div className="space-y-1.5">
            <div className="flex items-center gap-4">
              <label className="flex items-center gap-2 text-sm">
                <Checkbox checked={assignedAll} onCheckedChange={(v) => setAssignedAll(Boolean(v))} />
                全体学生
              </label>
              {!assignedAll && <span className="text-xs text-muted-foreground">已选 {selected.length} 人</span>}
            </div>
            {!assignedAll && (
              <div className="max-h-40 space-y-1 overflow-y-auto rounded-lg border p-1.5">
                {(students.data?.students ?? []).map((s: AdminStudent) => (
                  <label key={s.id} className="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-sm hover:bg-muted">
                    <Checkbox checked={selected.includes(s.id)} onCheckedChange={() => toggleStudent(s.id)} />
                    <span className="min-w-0 flex-1 truncate">{s.username}</span>
                  </label>
                ))}
              </div>
            )}
          </div>

          <label className="flex items-center gap-2 text-sm">
            <Checkbox checked={publish} onCheckedChange={(v) => setPublish(Boolean(v))} />
            立即发布（学生端可见）
          </label>

          {selectedRepo && (
            <div className="rounded-lg bg-muted/40 p-2.5 text-xs text-muted-foreground">
              将布置「{selectedRepo.title}」{kind === 'training' && `（${(selectedRepo as { chapterCount: number }).chapterCount} 章）`}
              ，共 {selectedRepo.problemCount} 题
            </div>
          )}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={onClose}>取消</Button>
          <Button onClick={() => void create()} disabled={!repoId || busy}>
            {busy ? '布置中…' : '布置'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

// ---------------- 编辑学生（定向/全体切换） ----------------

function EditStudentsDialog({ a, onDone, onClose }: { a: AdminAssignment; onDone: () => void; onClose: () => void }) {
  const qc = useQueryClient()
  const students = useQuery({ queryKey: ['admin-students'], queryFn: api.students })
  const current = useQuery({
    queryKey: ['assignment-students', a.id],
    queryFn: () => api.assignmentStudents(a.id),
  })
  const [assignedAll, setAssignedAll] = useState(a.assignedAll)
  const [selected, setSelected] = useState<number[]>([])
  const [busy, setBusy] = useState(false)
  // 打开时载入当前定向列表
  const [loaded, setLoaded] = useState(false)
  if (!loaded && current.data) {
    setSelected(current.data.students.map((s) => s.userId))
    setAssignedAll(current.data.assignedAll)
    setLoaded(true)
  }

  function toggleStudent(id: number) {
    setSelected((s) => (s.includes(id) ? s.filter((x) => x !== id) : [...s, id]))
  }

  async function save() {
    if (busy) return
    if (!assignedAll && selected.length === 0) {
      toast.error('请至少选择一名学生')
      return
    }
    setBusy(true)
    try {
      await api.setAssignmentStudents(a.id, { assignedAll, studentIds: selected })
      toast.success('已保存')
      void qc.invalidateQueries({ queryKey: ['admin-assignments'] })
      onDone()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '保存失败')
    } finally {
      setBusy(false)
    }
  }

  return (
    <Dialog open onOpenChange={(v) => { if (!v) onClose() }}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>编辑学生 — {a.title}</DialogTitle>
        </DialogHeader>
        <div className="space-y-3">
          <label className="flex items-center gap-2 text-sm">
            <Checkbox
              checked={assignedAll}
              onCheckedChange={(v) => setAssignedAll(Boolean(v))}
            />
            全体学生
          </label>
          {!assignedAll && (
            <div className="max-h-52 space-y-1 overflow-y-auto rounded-lg border p-1.5">
              {(students.data?.students ?? []).map((s: AdminStudent) => (
                <label key={s.id} className="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 text-sm hover:bg-muted">
                  <Checkbox checked={selected.includes(s.id)} onCheckedChange={() => toggleStudent(s.id)} />
                  <span className="min-w-0 flex-1 truncate">{s.username}</span>
                </label>
              ))}
            </div>
          )}
          {!assignedAll && selected.length === 0 && (
            <p className="text-xs text-red-500">尚未选择任何学生</p>
          )}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>取消</Button>
          <Button onClick={() => void save()} disabled={busy}>保存</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

// ---------------- 每题统计 ----------------

function StatsDialog({ a, onClose }: { a: AdminAssignment; onClose: () => void }) {
  const stats = useQuery({ queryKey: ['assignment-stats', a.id], queryFn: () => api.assignmentStats(a.id) })
  const data: AssignmentStats | undefined = stats.data
  return (
    <Dialog open onOpenChange={(v) => { if (!v) onClose() }}>
      <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>
            <span className="mr-2 flex items-center gap-1.5 text-base">
              <BarChart3Icon className="size-4" /> {a.title}
            </span>
          </DialogTitle>
          <DialogDescription>
            {a.assignedAll ? '全体学生' : `${a.studentCount} 名定向学生`} · 通过 = 曾 AC（客观题答对/编程题全点通过）
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-1.5">
          {(data?.problems ?? []).map((p, i) => (
            <div key={p.problemId} className="flex items-center gap-2 rounded-lg border px-3 py-2">
              <span className="flex size-6 shrink-0 items-center justify-center rounded-md bg-muted text-xs font-medium text-muted-foreground">{i + 1}</span>
              <div className="min-w-0 flex-1">
                <p className="truncate text-sm">{p.title}</p>
                <p className="text-[11px] text-muted-foreground">
                  {p.type === 'programming' ? '编程题' : p.type === 'single_choice' ? '单选题' : '判断题'}
                </p>
              </div>
              <div className="shrink-0 text-right text-xs">
                <div className="text-emerald-600">通过 {p.accepted}</div>
                <div className="text-muted-foreground">提交 {p.submissions}</div>
              </div>
            </div>
          ))}
          {stats.isLoading && <p className="p-4 text-center text-sm text-muted-foreground">统计加载中…</p>}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>关闭</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
