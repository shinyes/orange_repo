import { useEffect, useState, type FormEvent } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  ArrowDownIcon,
  ArrowUpIcon,
  PencilIcon,
  PlusIcon,
  RotateCcwIcon,
  Trash2Icon,
} from 'lucide-react'
import { toast } from 'sonner'

import { api } from '@/lib/api'
import type { AdminCategory, AdminSubject, ProblemType } from '@/lib/types'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Checkbox } from '@/components/ui/checkbox'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
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
import { AssignmentsTab } from '@/pages/oj/AdminAssignments'

const TYPE_LABEL: Record<ProblemType, string> = {
  single_choice: '单选题',
  true_false: '判断题',
}

const ALL_TYPES: ProblemType[] = ['single_choice', 'true_false']

export function parseTags(text: string): string[] {
  return text
    .split(/[,，、\s]+/)
    .map((s) => s.trim())
    .filter(Boolean)
    // 规范化标签路径：去首尾斜杠（CIE/ → CIE）、合并连续斜杠（a//b → a/b）。
    // 分类选父标签即按前缀包含其子树，末尾斜杠无意义且会被服务端校验拒绝。
    .map((s) => s.replace(/^\/+|\/+$/g, '').replace(/\/{2,}/g, '/'))
    .filter(Boolean)
}

function moveItem<T>(arr: T[], from: number, to: number): T[] {
  const next = [...arr]
  const [it] = next.splice(from, 1)
  next.splice(to, 0, it)
  return next
}

// 系统管理页（仅管理员）：科目 / 分类 / 学生 / 布置 / 设置。
export function AdminPage() {
  return (
    <div className="mx-auto w-full max-w-3xl px-4 py-6 lg:max-w-5xl lg:px-8 lg:py-8">
      <h1 className="mb-4 text-lg font-semibold">系统管理</h1>
      <Tabs defaultValue="subjects">
        <TabsList className="w-full">
          <TabsTrigger value="subjects" className="flex-1">科目</TabsTrigger>
          <TabsTrigger value="categories" className="flex-1">分类</TabsTrigger>
          <TabsTrigger value="assignments" className="flex-1">布置</TabsTrigger>
          <TabsTrigger value="students" className="flex-1">学生</TabsTrigger>
          <TabsTrigger value="settings" className="flex-1">设置</TabsTrigger>
        </TabsList>
        <TabsContent value="subjects">
          <SubjectsTab />
        </TabsContent>
        <TabsContent value="categories">
          <CategoriesTab />
        </TabsContent>
        <TabsContent value="assignments">
          <AssignmentsTab />
        </TabsContent>
        <TabsContent value="students">
          <StudentsTab />
        </TabsContent>
        <TabsContent value="settings">
          <SettingsTab />
        </TabsContent>
      </Tabs>
    </div>
  )
}

// ---------- 科目 ----------

function SubjectsTab() {
  const qc = useQueryClient()
  const subjects = useQuery({ queryKey: ['admin-subjects'], queryFn: api.adminSubjects })
  const [name, setName] = useState('')
  const [editing, setEditing] = useState<AdminSubject | null>(null)
  const [deleting, setDeleting] = useState<AdminSubject | null>(null)

  const list = subjects.data?.subjects ?? []

  async function create(e: FormEvent) {
    e.preventDefault()
    if (!name.trim()) return
    try {
      await api.createSubject(name.trim())
      setName('')
      toast.success('科目已创建')
      void qc.invalidateQueries({ queryKey: ['admin-subjects'] })
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '创建失败')
    }
  }

  async function moveSubject(idx: number, dir: -1 | 1) {
    const target = idx + dir
    if (target < 0 || target >= list.length) return
    const ids = moveItem(list.map((s) => s.id), idx, target)
    try {
      await api.setSubjectOrder(ids)
      void qc.invalidateQueries({ queryKey: ['admin-subjects'] })
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '排序失败')
    }
  }

  return (
    <div className="space-y-3 pt-4">
      <form onSubmit={create} className="flex flex-col gap-2 sm:flex-row">
        <Input placeholder="新科目名称" value={name} onChange={(e) => setName(e.target.value)} />
        <Button type="submit" disabled={!name.trim()}>
          <PlusIcon className="size-4" /> 新增
        </Button>
      </form>
      {list.map((s, i) => (
        <div key={s.id} className="flex items-center gap-2 rounded-xl border bg-card p-3">
          <div className="flex flex-col gap-0.5">
            <button
              type="button"
              className="flex size-8 items-center justify-center rounded-md text-muted-foreground hover:text-foreground disabled:opacity-30"
              onClick={() => void moveSubject(i, -1)}
              disabled={i === 0}
              aria-label="上移"
            >
              <ArrowUpIcon className="size-4" />
            </button>
            <button
              type="button"
              className="flex size-8 items-center justify-center rounded-md text-muted-foreground hover:text-foreground disabled:opacity-30"
              onClick={() => void moveSubject(i, 1)}
              disabled={i === list.length - 1}
              aria-label="下移"
            >
              <ArrowDownIcon className="size-4" />
            </button>
          </div>
          <span className="min-w-0 flex-1 truncate font-medium">{s.name}</span>
          <span className="text-xs text-muted-foreground">{s.categories.length} 个分类</span>
          <Button variant="ghost" size="icon" onClick={() => setEditing(s)} aria-label="编辑">
            <PencilIcon className="size-4" />
          </Button>
          <Button variant="ghost" size="icon" className="text-red-600 hover:text-red-600" onClick={() => setDeleting(s)} aria-label="删除">
            <Trash2Icon className="size-4" />
          </Button>
        </div>
      ))}
      {list.length === 0 && (
        <div className="rounded-xl border border-dashed p-8 text-center text-sm text-muted-foreground">暂无科目</div>
      )}

      {/* 重命名 */}
      <RenameSubjectDialog
        subject={editing}
        onOpenChange={(v) => {
          if (!v) setEditing(null)
        }}
      />
      {/* 删除确认 */}
      <AlertDialog open={deleting !== null} onOpenChange={(v) => { if (!v) setDeleting(null) }}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>删除科目「{deleting?.name}」？</AlertDialogTitle>
            <AlertDialogDescription>科目下的全部分类及学生的相关错题记录将一并删除，此操作不可撤销。</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <AlertDialogAction
              className="bg-red-600 hover:bg-red-700"
              onClick={() => {
                if (!deleting) return
                void api.deleteSubject(deleting.id).then(() => {
                  toast.success('科目已删除')
                  void qc.invalidateQueries({ queryKey: ['admin-subjects'] })
                }).catch((err) => toast.error(err instanceof Error ? err.message : '删除失败'))
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

function RenameSubjectDialog({ subject, onOpenChange }: { subject: AdminSubject | null; onOpenChange: (v: boolean) => void }) {
  const qc = useQueryClient()
  const [name, setName] = useState('')
  useEffect(() => {
    if (subject) setName(subject.name)
  }, [subject])
  return (
    <Dialog open={subject !== null} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle>重命名科目</DialogTitle>
        </DialogHeader>
        <form
          onSubmit={(e) => {
            e.preventDefault()
            if (!subject || !name.trim()) return
            void api.renameSubject(subject.id, name.trim()).then(() => {
              toast.success('已重命名')
              onOpenChange(false)
              void qc.invalidateQueries({ queryKey: ['admin-subjects'] })
            }).catch((err) => toast.error(err instanceof Error ? err.message : '重命名失败'))
          }}
          className="space-y-4"
        >
          <Input value={name} onChange={(e) => setName(e.target.value)} required />
          <DialogFooter>
            <Button type="submit">保存</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

// ---------- 分类 ----------

function CategoriesTab() {
  const qc = useQueryClient()
  const subjects = useQuery({ queryKey: ['admin-subjects'], queryFn: api.adminSubjects })
  const [dialog, setDialog] = useState<{ subject: AdminSubject; category?: AdminCategory } | null>(null)
  const [deleting, setDeleting] = useState<{ subject: AdminSubject; category: AdminCategory } | null>(null)
  const list = subjects.data?.subjects ?? []

  async function moveCategory(subject: AdminSubject, idx: number, dir: -1 | 1) {
    const target = idx + dir
    if (target < 0 || target >= subject.categories.length) return
    const ids = moveItem(subject.categories.map((c) => c.id), idx, target)
    try {
      await api.setCategoryOrder(subject.id, ids)
      void qc.invalidateQueries({ queryKey: ['admin-subjects'] })
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '排序失败')
    }
  }

  return (
    <div className="space-y-5 pt-4">
      {list.map((s) => (
        <div key={s.id} className="rounded-xl border bg-card">
          <div className="flex items-center justify-between border-b px-4 py-2.5">
            <span className="text-sm font-semibold">{s.name}</span>
            <Button size="sm" variant="outline" onClick={() => setDialog({ subject: s })}>
              <PlusIcon className="size-4" /> 新增分类
            </Button>
          </div>
          {s.categories.length === 0 ? (
            <div className="p-5 text-center text-xs text-muted-foreground">暂无分类</div>
          ) : (
            s.categories.map((c, i) => (
              <div key={c.id} className="flex items-center gap-2 border-b px-4 py-2.5 last:border-b-0">
                <div className="flex flex-col gap-0.5">
                  <button
                    type="button"
                    className="flex size-8 items-center justify-center rounded-md text-muted-foreground hover:text-foreground disabled:opacity-30"
                    onClick={() => void moveCategory(s, i, -1)}
                    disabled={i === 0}
                    aria-label="上移"
                  >
                    <ArrowUpIcon className="size-3.5" />
                  </button>
                  <button
                    type="button"
                    className="flex size-8 items-center justify-center rounded-md text-muted-foreground hover:text-foreground disabled:opacity-30"
                    onClick={() => void moveCategory(s, i, 1)}
                    disabled={i === s.categories.length - 1}
                    aria-label="下移"
                  >
                    <ArrowDownIcon className="size-3.5" />
                  </button>
                </div>
                <div className="min-w-0 flex-1">
                  <div className="flex flex-wrap items-center gap-1.5">
                    <span className="truncate text-sm font-medium">{c.name}</span>
                    <span className="text-xs text-muted-foreground">顺序 {c.orderNo}</span>
                  </div>
                  <div className="mt-1 flex flex-wrap items-center gap-1 text-xs">
                    {c.tags.map((t) => (
                      <Badge key={t} variant="secondary" className="text-[10px]">{t}</Badge>
                    ))}
                    {c.tags.length === 0 && <span className="text-muted-foreground">全部标签</span>}
                    {c.types.map((t) => (
                      <Badge key={t} variant="outline" className="text-[10px]">{TYPE_LABEL[t]}</Badge>
                    ))}
                    <span className="ml-1 text-muted-foreground">{c.questionCount} 题</span>
                  </div>
                </div>
                <Button variant="ghost" size="icon" onClick={() => setDialog({ subject: s, category: c })} aria-label="编辑">
                  <PencilIcon className="size-4" />
                </Button>
                <Button variant="ghost" size="icon" className="text-red-600 hover:text-red-600" onClick={() => setDeleting({ subject: s, category: c })} aria-label="删除">
                  <Trash2Icon className="size-4" />
                </Button>
              </div>
            ))
          )}
        </div>
      ))}
      {list.length === 0 && (
        <div className="rounded-xl border border-dashed p-8 text-center text-sm text-muted-foreground">
          请先在「科目」页创建科目
        </div>
      )}

      {dialog && (
        <CategoryDialog
          subjectId={dialog.subject.id}
          category={dialog.category}
          onOpenChange={(v) => { if (!v) setDialog(null) }}
          onSaved={() => {
            setDialog(null)
            void qc.invalidateQueries({ queryKey: ['admin-subjects'] })
          }}
        />
      )}

      <AlertDialog open={deleting !== null} onOpenChange={(v) => { if (!v) setDeleting(null) }}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>删除分类「{deleting?.category.name}」？</AlertDialogTitle>
            <AlertDialogDescription>学生的相关错题记录将一并删除，此操作不可撤销。</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <AlertDialogAction
              className="bg-red-600 hover:bg-red-700"
              onClick={() => {
                if (!deleting) return
                void api.deleteCategory(deleting.category.id).then(() => {
                  toast.success('分类已删除')
                  void qc.invalidateQueries({ queryKey: ['admin-subjects'] })
                }).catch((err) => toast.error(err instanceof Error ? err.message : '删除失败'))
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

// 分类编辑对话框：名称 / 显示顺序 / 标签映射 / 题型映射 + 实时题目数预览。
function CategoryDialog({
  subjectId,
  category,
  onOpenChange,
  onSaved,
}: {
  subjectId: number
  category?: AdminCategory
  onOpenChange: (v: boolean) => void
  onSaved: () => void
}) {
  const [name, setName] = useState(category?.name ?? '')
  const [orderNo, setOrderNo] = useState(String(category?.orderNo ?? ''))
  const [tagsText, setTagsText] = useState((category?.tags ?? []).join('、'))
  const [types, setTypes] = useState<ProblemType[]>(category?.types ?? ALL_TYPES)
  const [count, setCount] = useState<number | null>(null)
  const [previewError, setPreviewError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  // 实时题目数预览（防抖）；标签格式错误（如末尾斜杠）时显示服务端校验信息
  useEffect(() => {
    setPreviewError(null)
    const t = setTimeout(() => {
      void api.problemsCount(parseTags(tagsText), types)
        .then((d) => setCount(d.count))
        .catch((err) => {
          setCount(null)
          setPreviewError(err instanceof Error ? err.message : '筛选条件无效')
        })
    }, 300)
    return () => clearTimeout(t)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tagsText, types])

  function toggleType(t: ProblemType) {
    setTypes((prev) => (prev.includes(t) ? prev.filter((x) => x !== t) : [...prev, t]))
  }

  async function save(e: FormEvent) {
    e.preventDefault()
    if (busy) return
    setBusy(true)
    try {
      const payload = {
        name: name.trim(),
        orderNo: orderNo ? Number(orderNo) : undefined,
        tags: parseTags(tagsText),
        types,
      }
      if (category) {
        await api.updateCategory(category.id, payload)
      } else {
        await api.createCategory({ subjectId, ...payload })
      }
      toast.success(category ? '分类已更新' : '分类已创建')
      onSaved()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '保存失败')
    } finally {
      setBusy(false)
    }
  }

  return (
    <Dialog open onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{category ? '编辑分类' : '新增分类'}</DialogTitle>
          <DialogDescription>配置分类对应的题目标签与题型（第一阶段仅单选/判断）</DialogDescription>
        </DialogHeader>
        <form onSubmit={save} className="space-y-4">
          <div className="grid grid-cols-2 gap-3">
            <div className="space-y-1.5">
              <Label htmlFor="cat-name">分类名称</Label>
              <Input id="cat-name" value={name} onChange={(e) => setName(e.target.value)} required />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="cat-order">显示顺序</Label>
              <Input
                id="cat-order"
                type="number"
                min={1}
                value={orderNo}
                onChange={(e) => setOrderNo(e.target.value)}
                placeholder="自动末尾"
              />
            </div>
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="cat-tags">题目标签（用逗号或顿号分隔，留空 = 全部标签）</Label>
            <Input id="cat-tags" value={tagsText} onChange={(e) => setTagsText(e.target.value)} placeholder="如：数学、物理/力学" />
          </div>
          <div className="space-y-1.5">
            <Label>题目类型</Label>
            <div className="flex gap-4">
              {ALL_TYPES.map((t) => (
                <label key={t} className="flex items-center gap-1.5 text-sm">
                  <Checkbox checked={types.includes(t)} onCheckedChange={() => toggleType(t)} />
                  {TYPE_LABEL[t]}
                </label>
              ))}
            </div>
            {types.length === 0 && <p className="text-xs text-red-600">至少选择一种题型</p>}
          </div>
          {previewError ? (
            <div className="text-xs text-red-600">{previewError}</div>
          ) : (
            <div className="text-xs text-muted-foreground">
              当前筛选命中题目数：<span className="font-medium">{count === null ? '…' : count}</span>
            </div>
          )}
          <DialogFooter>
            <Button type="submit" disabled={busy || types.length === 0}>
              保存
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

// ---------- 学生与管理员 ----------

function StudentsTab() {
  const qc = useQueryClient()
  const students = useQuery({ queryKey: ['admin-students'], queryFn: api.students })
  const admins = useQuery({ queryKey: ['admin-admins'], queryFn: api.admins })
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [resetting, setResetting] = useState<{ id: number; username: string; kind: 'student' | 'admin' } | null>(null)
  const [deleting, setDeleting] = useState<{ id: number; username: string } | null>(null)
  const list = students.data?.students ?? []

  async function create(e: FormEvent) {
    e.preventDefault()
    if (!username.trim() || !password) return
    try {
      await api.createStudent(username.trim(), password)
      setUsername('')
      setPassword('')
      toast.success('学生账号已创建')
      void qc.invalidateQueries({ queryKey: ['admin-students'] })
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '创建失败')
    }
  }

  function submitReset(id: number, pwd: string) {
    return resetting?.kind === 'admin' ? api.resetAdminPassword(id, pwd) : api.resetStudentPassword(id, pwd)
  }

  return (
    <div className="space-y-3 pt-4">
      {/* 管理员账号（统一账号库：与主站共享，重置后全端重新登录） */}
      <div className="rounded-xl border bg-card">
        <div className="border-b px-4 py-2.5 text-sm font-semibold">管理员账号</div>
        {(admins.data?.admins ?? []).map((a) => (
          <div key={a.id} className="flex items-center gap-2 border-b px-4 py-2.5 last:border-b-0">
            <div className="min-w-0 flex-1">
              <span className="truncate text-sm font-medium">{a.username}</span>
              <span className="ml-2 text-xs text-muted-foreground">主站与刷题服务共用</span>
            </div>
            <Button variant="ghost" size="icon" onClick={() => setResetting({ id: a.id, username: a.username, kind: 'admin' })} aria-label="重置密码">
              <RotateCcwIcon className="size-4" />
            </Button>
          </div>
        ))}
      </div>

      <form onSubmit={create} className="flex flex-col gap-2 sm:flex-row">
        <Input placeholder="用户名" value={username} onChange={(e) => setUsername(e.target.value)} />
        <Input placeholder="初始密码" value={password} onChange={(e) => setPassword(e.target.value)} type="password" />
        <Button type="submit" disabled={!username.trim() || !password}>
          <PlusIcon className="size-4" /> 新增
        </Button>
      </form>
      <div className="text-xs text-muted-foreground">学生账号</div>
      {list.map((s) => (
        <div key={s.id} className="flex items-center gap-2 rounded-xl border bg-card p-3">
          <div className="min-w-0 flex-1">
            <div className="truncate text-sm font-medium">{s.username}</div>
            <div className="text-xs text-muted-foreground">错题 {s.wrongCount} 题 · 创建于 {s.createdAt.slice(0, 10)}</div>
          </div>
          <Button variant="ghost" size="icon" onClick={() => setResetting({ id: s.id, username: s.username, kind: 'student' })} aria-label="重置密码">
            <RotateCcwIcon className="size-4" />
          </Button>
          <Button variant="ghost" size="icon" className="text-red-600 hover:text-red-600" onClick={() => setDeleting({ id: s.id, username: s.username })} aria-label="删除">
            <Trash2Icon className="size-4" />
          </Button>
        </div>
      ))}
      {list.length === 0 && (
        <div className="rounded-xl border border-dashed p-8 text-center text-sm text-muted-foreground">暂无学生账号</div>
      )}

      {/* 重置密码 */}
      <Dialog open={resetting !== null} onOpenChange={(v) => { if (!v) setResetting(null) }}>
        <DialogContent className="sm:max-w-sm">
          <DialogHeader>
            <DialogTitle>重置密码 — {resetting?.username}</DialogTitle>
            <DialogDescription>
              {resetting?.kind === 'admin'
                ? '统一账号：重置后主站与刷题服务均使用新密码，且该账号全部在线会话立即失效。'
                : '重置后该学生需用新密码登录。'}
            </DialogDescription>
          </DialogHeader>
          <ResetPasswordForm
            onSubmit={(pwd) => {
              if (!resetting) return Promise.reject(new Error('缺少账号'))
              return submitReset(resetting.id, pwd)
            }}
            onDone={() => {
              setResetting(null)
              toast.success('密码已重置')
              void qc.invalidateQueries({ queryKey: ['admin-students'] })
              void qc.invalidateQueries({ queryKey: ['admin-admins'] })
            }}
          />
        </DialogContent>
      </Dialog>
      {/* 删除确认 */}
      <AlertDialog open={deleting !== null} onOpenChange={(v) => { if (!v) setDeleting(null) }}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>删除学生「{deleting?.username}」？</AlertDialogTitle>
            <AlertDialogDescription>该学生的错题记录将一并删除，此操作不可撤销。</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <AlertDialogAction
              className="bg-red-600 hover:bg-red-700"
              onClick={() => {
                if (!deleting) return
                void api.deleteStudent(deleting.id).then(() => {
                  toast.success('学生已删除')
                  void qc.invalidateQueries({ queryKey: ['admin-students'] })
                }).catch((err) => toast.error(err instanceof Error ? err.message : '删除失败'))
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

function ResetPasswordForm({ onSubmit, onDone }: { onSubmit: (pwd: string) => Promise<void>; onDone: () => void }) {
  const [password, setPassword] = useState('')
  return (
    <form
      className="space-y-4"
      onSubmit={(e) => {
        e.preventDefault()
        if (!password) return
        void onSubmit(password).then(onDone).catch((err) => toast.error(err instanceof Error ? err.message : '重置失败'))
      }}
    >
      <Input type="password" placeholder="新密码" value={password} onChange={(e) => setPassword(e.target.value)} required />
      <DialogFooter>
        <Button type="submit" disabled={!password}>确认重置</Button>
      </DialogFooter>
    </form>
  )
}

// ---------- 设置 ----------

function SettingsTab() {
  const qc = useQueryClient()
  const settings = useQuery({ queryKey: ['admin-settings'], queryFn: api.settings })
  const [roundSize, setRoundSize] = useState('')
  useEffect(() => {
    if (settings.data) setRoundSize(String(settings.data.roundSize))
  }, [settings.data])

  async function save() {
    const n = Number(roundSize)
    if (!Number.isInteger(n) || n < 1 || n > 100) {
      toast.error('每轮题数需为 1–100 的整数')
      return
    }
    try {
      await api.putSettings(n)
      toast.success('设置已保存')
      void qc.invalidateQueries({ queryKey: ['admin-settings'] })
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '保存失败')
    }
  }

  return (
    <div className="space-y-4 pt-4">
      <div className="space-y-1.5">
        <Label htmlFor="round-size">每轮题数（1–100，抽题时从符合分类筛选的题目中随机抽取）</Label>
        <div className="flex flex-col gap-2 sm:flex-row">
          <Input id="round-size" type="number" min={1} max={100} value={roundSize} onChange={(e) => setRoundSize(e.target.value)} />
          <Button onClick={() => void save()}>保存</Button>
        </div>
      </div>
    </div>
  )
}