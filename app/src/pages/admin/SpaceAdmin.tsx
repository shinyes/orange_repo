// 空间管理（/admin/spaces；domain_admin / global_admin 选中域后，全宽页）：
// 空间 CRUD + 展开详情（成员管理 / 内容管理：空间训练·练习·刷题项目）。
// 迁移适配：返回按钮从 view.kind 状态机（goHome）改为 URL 导航回 /admin/problems。
import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import {
  ArrowLeftIcon,
  BookOpenIcon,
  ChevronRightIcon,
  ClipboardListIcon,
  FileStackIcon,
  LayoutGridIcon,
  LoaderCircleIcon,
  PencilIcon,
  PlusIcon,
  SearchIcon,
  Trash2Icon,
  UsersIcon,
  XIcon,
} from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { api } from '@/api'
import { useDomain } from '@/pages/admin/domain-context'
import type { Space } from '@/api/types'
import { ConfirmDialog } from './dialogs'

export function SpaceAdmin() {
  const { domainId } = useDomain()
  const navigate = useNavigate()
  const spacesQ = useQuery({
    queryKey: ['admin', 'spaces', domainId],
    queryFn: api.spaces,
    enabled: domainId != null,
  })
  const spaces = spacesQ.data?.spaces ?? []

  // 当前域名（global_admin 全部 / domain_admin 仅其域——见后端 handleListDomains）
  const domainsQ = useQuery({
    queryKey: ['admin', 'domains'],
    queryFn: () => api.domains(),
    enabled: domainId != null,
  })
  const domainName = domainsQ.data?.domains.find((d) => d.id === domainId)?.name

  const [creating, setCreating] = useState(false)
  const [renaming, setRenaming] = useState<Space | null>(null)
  const [deleting, setDeleting] = useState<Space | null>(null)
  const [expanded, setExpanded] = useState<number | null>(null)

  return (
    <div className="h-full overflow-y-auto">
    <div className="mx-auto max-w-5xl px-6 py-6">
      <div className="mb-5 flex items-center gap-2">
        <Button variant="ghost" size="icon-sm" title="返回题目管理" onClick={() => navigate('/admin/problems')}>
          <ArrowLeftIcon />
        </Button>
        <h1 className="text-xl font-semibold">空间管理</h1>
        <Badge variant="secondary" className="text-xs">{domainName ? `${domainName} #${domainId}` : `域 #${domainId ?? '—'}`}</Badge>
        <div className="ml-auto">
          {domainId != null && (
            <Button size="sm" onClick={() => setCreating(true)}>
              <PlusIcon data-icon="inline-start" /> 新建空间
            </Button>
          )}
        </div>
      </div>

      {domainId == null ? (
        <div className="flex flex-col items-center justify-center gap-2 py-16 text-center">
          <LayoutGridIcon className="size-8 text-muted-foreground/60" />
          <p className="text-sm font-medium">请先选择域</p>
          <p className="text-xs text-muted-foreground">作为系统管理员，请先在顶栏选择要管理的域，再管理该域的空间。</p>
        </div>
      ) : spacesQ.isLoading ? (
        <div className="flex items-center justify-center gap-2 py-10 text-sm text-muted-foreground">
          <LoaderCircleIcon className="size-4 animate-spin" /> 加载中…
        </div>
      ) : spaces.length === 0 ? (
        <div className="py-10 text-center text-sm text-muted-foreground">该域还没有空间，点击右上角「新建空间」创建。</div>
      ) : (
        <div className="space-y-2">
          {spaces.map((sp) => (
            <SpaceCard
              key={sp.id}
              space={sp}
              expanded={expanded === sp.id}
              onToggle={() => setExpanded(expanded === sp.id ? null : sp.id)}
              onRename={() => setRenaming(sp)}
              onDelete={() => setDeleting(sp)}
            />
          ))}
        </div>
      )}

      <CreateSpaceDialog open={creating} onOpenChange={setCreating} onCreated={() => void spacesQ.refetch()} />
      <RenameSpaceDialog space={renaming} onOpenChange={(v) => !v && setRenaming(null)} onDone={() => void spacesQ.refetch()} />
      {deleting && (
        <ConfirmDialog
          open
          onOpenChange={(v) => !v && setDeleting(null)}
          title={`删除空间「${deleting.name}」？`}
          description="空间内的训练/练习/刷题项目与作答数据将一并删除，操作不可撤销。"
          onConfirm={async () => {
            try {
              await api.deleteSpace(deleting.id)
              toast.success('空间已删除')
              if (expanded === deleting.id) setExpanded(null)
              await spacesQ.refetch()
              setDeleting(null)
            } catch (e) {
              toast.error(e instanceof Error ? e.message : '删除失败')
            }
          }}
        />
      )}
    </div>
    </div>
  )
}

// ---------- 空间行 + 展开详情 ----------

function SpaceCard(props: {
  space: Space
  expanded: boolean
  onToggle: () => void
  onRename: () => void
  onDelete: () => void
}) {
  const { space, expanded } = props
  return (
    <div className="overflow-hidden rounded-xl border">
      <div className="flex items-center gap-2 px-4 py-2.5 hover:bg-muted/30">
        <button type="button" className="flex flex-1 items-center gap-2 text-left" onClick={props.onToggle}>
          <ChevronRightIcon className={`size-4 text-muted-foreground transition-transform ${expanded ? 'rotate-90' : ''}`} />
          <LayoutGridIcon className="size-4 shrink-0 text-primary/70" />
          <span className="min-w-0 flex-1 truncate text-sm font-medium">{space.name}</span>
          <Badge variant="secondary" className="text-[10px]">#{space.id}</Badge>
        </button>
        <div className="flex items-center gap-1">
          <Button size="xs" variant="ghost" onClick={props.onRename}>
            <PencilIcon data-icon="inline-start" /> 改名
          </Button>
          <Button size="xs" variant="ghost" className="text-destructive" onClick={props.onDelete}>
            <Trash2Icon data-icon="inline-start" /> 删除
          </Button>
        </div>
      </div>
      {expanded && <SpaceDetail spaceId={space.id} />}
    </div>
  )
}

function SpaceDetail({ spaceId }: { spaceId: number }) {
  const [tab, setTab] = useState<'members' | 'trainings' | 'practices' | 'quizzes'>('members')
  const tabs = [
    { key: 'members', label: '成员', icon: UsersIcon },
    { key: 'trainings', label: '训练', icon: BookOpenIcon },
    { key: 'practices', label: '练习', icon: ClipboardListIcon },
    { key: 'quizzes', label: '刷题项目', icon: FileStackIcon },
  ] as const

  return (
    <div className="border-t bg-muted/20 px-4 py-3">
      <div className="mb-3 flex gap-1 border-b pb-2">
        {tabs.map((t) => (
          <button
            key={t.key}
            type="button"
            onClick={() => setTab(t.key)}
            className={`inline-flex items-center gap-1.5 rounded-md px-2.5 py-1 text-xs transition-colors ${
              tab === t.key ? 'bg-background font-medium text-foreground shadow-sm' : 'text-muted-foreground hover:bg-muted'
            }`}
          >
            <t.icon className="size-3.5" /> {t.label}
          </button>
        ))}
      </div>
      {tab === 'members' && <MembersPanel spaceId={spaceId} />}
      {tab === 'trainings' && <TrainingsPanel spaceId={spaceId} />}
      {tab === 'practices' && <PracticesPanel spaceId={spaceId} />}
      {tab === 'quizzes' && <QuizzesPanel spaceId={spaceId} />}
    </div>
  )
}

// ---------- 成员管理 ----------

function MembersPanel({ spaceId }: { spaceId: number }) {
  const qc = useQueryClient()
  const membersQ = useQuery({
    queryKey: ['admin', 'spaces', spaceId, 'members'],
    queryFn: () => api.spaceMembers(spaceId),
  })
  const usersQ = useQuery({ queryKey: ['admin', 'users'], queryFn: api.users })
  const members = membersQ.data?.members ?? []
  const allUsers = usersQ.data?.users ?? []

  const [pickerOpen, setPickerOpen] = useState(false)
  const [selected, setSelected] = useState<Set<number>>(new Set())
  const [busy, setBusy] = useState(false)
  // 新建成员账号
  const [newUserOpen, setNewUserOpen] = useState(false)

  function openPicker() {
    setSelected(new Set(members.map((m) => m.userId)))
    setPickerOpen(true)
  }

  async function saveMembers() {
    setBusy(true)
    try {
      await api.setSpaceMembers(spaceId, [...selected])
      toast.success('成员已更新')
      setPickerOpen(false)
      await qc.invalidateQueries({ queryKey: ['admin', 'spaces', spaceId, 'members'] })
    } catch (e) {
      toast.error(e instanceof Error ? e.message : '保存失败')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div>
      <div className="mb-2 flex items-center gap-2">
        <span className="text-sm font-medium">空间成员（{members.length}）</span>
        <div className="ml-auto flex gap-1.5">
          <Button size="xs" variant="outline" onClick={() => setNewUserOpen(true)}>
            <PlusIcon data-icon="inline-start" /> 新建成员账号
          </Button>
          <Button size="xs" onClick={openPicker}>
            <UsersIcon data-icon="inline-start" /> 选择成员
          </Button>
        </div>
      </div>

      {membersQ.isLoading ? (
        <div className="flex items-center gap-2 py-4 text-xs text-muted-foreground">
          <LoaderCircleIcon className="size-3.5 animate-spin" /> 加载中…
        </div>
      ) : members.length === 0 ? (
        <div className="rounded-lg border border-dashed px-3 py-3 text-center text-xs text-muted-foreground">
          暂无成员。点「选择成员」从账号池拉人；或先「新建成员账号」。
        </div>
      ) : (
        <div className="flex flex-wrap gap-1.5">
          {members.map((m) => (
            <Badge key={m.userId} variant="secondary" className="gap-1 py-1 text-xs">
              {m.username}
              <span className="text-[10px] text-muted-foreground">#{m.userId}</span>
            </Badge>
          ))}
        </div>
      )}

      {/* 选择成员（覆盖式） */}
      <Dialog open={pickerOpen} onOpenChange={(v) => !v && setPickerOpen(false)}>
        <DialogContent className="max-h-[80vh] overflow-y-auto sm:max-w-md">
          <DialogHeader>
            <DialogTitle>选择空间成员</DialogTitle>
            <DialogDescription>勾选要加入的成员（保存将覆盖现有成员）。可在下方创建账号。</DialogDescription>
          </DialogHeader>
          <div className="max-h-72 space-y-1 overflow-y-auto">
            {allUsers.length === 0 && <p className="text-xs text-muted-foreground">暂无可用成员账号。</p>}
            {allUsers.map((u) => (
              <label
                key={u.id}
                className={`flex cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-sm hover:bg-muted ${
                  selected.has(u.id) ? 'bg-primary/5' : ''
                }`}
              >
                <input
                  type="checkbox"
                  className="size-3.5 accent-[var(--primary)]"
                  checked={selected.has(u.id)}
                  onChange={() =>
                    setSelected((prev) => {
                      const next = new Set(prev)
                      if (next.has(u.id)) next.delete(u.id)
                      else next.add(u.id)
                      return next
                    })
                  }
                />
                <span className="flex-1 truncate">{u.username}</span>
                <span className="text-[10px] text-muted-foreground">#{u.id}</span>
              </label>
            ))}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setPickerOpen(false)}>取消</Button>
            <Button onClick={() => void saveMembers()} disabled={busy}>
              {busy ? '保存中…' : `保存（${selected.size} 人）`}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <CreateMemberDialog open={newUserOpen} onOpenChange={setNewUserOpen} onCreated={() => void usersQ.refetch()} />
    </div>
  )
}

function CreateMemberDialog(props: { open: boolean; onOpenChange: (v: boolean) => void; onCreated: () => void }) {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [busy, setBusy] = useState(false)
  async function submit() {
    if (!username.trim() || !password) {
      toast.error('用户名与密码必填')
      return
    }
    setBusy(true)
    try {
      await api.createUser(username.trim(), password)
      toast.success(`成员账号「${username.trim()}」已创建`)
      props.onCreated()
      props.onOpenChange(false)
      setUsername('')
      setPassword('')
    } catch (e) {
      toast.error(e instanceof Error ? e.message : '创建失败')
    } finally {
      setBusy(false)
    }
  }
  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className="sm:max-w-xs">
        <DialogHeader>
          <DialogTitle>新建成员账号</DialogTitle>
          <DialogDescription>创建后可将其拉入任意空间做题。</DialogDescription>
        </DialogHeader>
        <div className="space-y-2">
          <Input value={username} onChange={(e) => setUsername(e.target.value)} placeholder="用户名" autoFocus />
          <Input type="password" value={password} onChange={(e) => setPassword(e.target.value)} placeholder="初始密码" />
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => props.onOpenChange(false)}>取消</Button>
          <Button onClick={() => void submit()} disabled={busy || !username.trim() || !password}>
            {busy ? '创建中…' : '创建'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

// ---------- 空间内容：训练 ----------

function TrainingsPanel({ spaceId }: { spaceId: number }) {
  const qc = useQueryClient()
  const listQ = useQuery({
    queryKey: ['space', spaceId, 'trainings'],
    queryFn: () => api.spaceTrainings(spaceId),
  })
  const trainings = listQ.data?.trainings ?? []
  const [creating, setCreating] = useState(false)
  const [openId, setOpenId] = useState<number | null>(null)
  const invalidate = () => {
    void qc.invalidateQueries({ queryKey: ['space', spaceId, 'trainings'] })
  }
  return (
    <div>
      <div className="mb-2 flex items-center gap-2">
        <span className="text-sm font-medium">空间训练（{trainings.length}）</span>
        <div className="ml-auto">
          <Button size="xs" onClick={() => setCreating(true)}>
            <PlusIcon data-icon="inline-start" /> 新建训练
          </Button>
        </div>
      </div>
      {trainings.length === 0 ? (
        <div className="rounded-lg border border-dashed px-3 py-3 text-center text-xs text-muted-foreground">暂无训练</div>
      ) : (
        <div className="space-y-1.5">
          {trainings.map((t) => (
            <div key={t.id} className="rounded-lg border bg-background">
              <div className="flex items-center gap-2 px-3 py-2">
                <button
                  type="button"
                  className="flex min-w-0 flex-1 items-center gap-2 text-left"
                  onClick={() => setOpenId(openId === t.id ? null : t.id)}
                >
                  <ChevronRightIcon className={`size-4 shrink-0 text-muted-foreground transition-transform ${openId === t.id ? 'rotate-90' : ''}`} />
                  <BookOpenIcon className="size-4 shrink-0 text-primary/70" />
                  <span className="min-w-0 flex-1 truncate text-sm">{t.title}</span>
                  <Badge variant="outline" className="shrink-0 text-[10px]">{t.problemCount} 题</Badge>
                  {t.maxAttempts > 0 && <Badge variant="secondary" className="shrink-0 text-[10px]">限 {t.maxAttempts} 次</Badge>}
                </button>
                <Button
                  size="icon-xs"
                  variant="ghost"
                  className="text-destructive"
                  title="删除训练"
                  onClick={() => {
                    if (!confirm(`删除训练「${t.title}」？空间内章节/条目将一并删除。`)) return
                    void api.deleteSpaceTraining(spaceId, t.id).then(() => {
                      toast.success('已删除')
                      if (openId === t.id) setOpenId(null)
                      invalidate()
                    }).catch((e) => toast.error(e instanceof Error ? e.message : '删除失败'))
                  }}
                >
                  <Trash2Icon />
                </Button>
              </div>
              {openId === t.id && <TrainingDetailBody spaceId={spaceId} trainingId={t.id} onChanged={invalidate} />}
            </div>
          ))}
        </div>
      )}

      <CreateSpaceTrainingDialog
        spaceId={spaceId}
        open={creating}
        onOpenChange={setCreating}
        onCreated={() => invalidate()}
      />
    </div>
  )
}

function TrainingDetailBody({ spaceId, trainingId, onChanged }: { spaceId: number; trainingId: number; onChanged: () => void }) {
  const qc = useQueryClient()
  const q = useQuery({
    queryKey: ['space', spaceId, 'trainings', trainingId],
    queryFn: () => api.getSpaceTraining(spaceId, trainingId),
  })
  const [chapterTitle, setChapterTitle] = useState('')
  // 从仓库题库选题加入章节
  const [addingTo, setAddingTo] = useState<number | null>(null) // chapterId；非 null 时打开选题弹窗
  const [pickerOpen, setPickerOpen] = useState(false)
  const [pickerChapter, setPickerChapter] = useState<number | null>(null)

  if (q.isLoading) {
    return <div className="flex items-center gap-2 px-4 py-3 text-xs text-muted-foreground"><LoaderCircleIcon className="size-3.5 animate-spin" /> 加载中…</div>
  }
  const chapters = q.data?.chapters ?? []
  const invalidate = () => {
    void qc.invalidateQueries({ queryKey: ['space', spaceId, 'trainings', trainingId] })
    onChanged()
  }

  async function addChapter() {
    if (!chapterTitle.trim()) return
    try {
      await api.createSpaceChapter(spaceId, trainingId, chapterTitle.trim())
      setChapterTitle('')
      invalidate()
    } catch (e) {
      toast.error(e instanceof Error ? e.message : '创建章节失败')
    }
  }

  function openPicker(chapterId: number) {
    setAddingTo(chapterId)
    setPickerChapter(chapterId)
    setPickerOpen(true)
  }

  return (
    <div className="space-y-2 border-t bg-muted/30 px-4 py-3">
      <div className="mb-1 flex items-center gap-2">
        <span className="text-xs font-medium text-muted-foreground">章节与条目</span>
        <div className="ml-auto flex gap-1.5">
          <Input
            value={chapterTitle}
            onChange={(e) => setChapterTitle(e.target.value)}
            placeholder="新章节名称"
            className="h-7 w-40 text-xs"
            onKeyDown={(e) => e.key === 'Enter' && void addChapter()}
          />
          <Button size="xs" variant="outline" onClick={() => void addChapter()} disabled={!chapterTitle.trim()}>
            添加章节
          </Button>
        </div>
      </div>
      {chapters.length === 0 ? (
        <p className="text-xs text-muted-foreground">还没有章节。</p>
      ) : (
        chapters.map((ch) => (
          <div key={ch.id} className="rounded-lg border bg-background px-3 py-2">
            <div className="mb-1 flex items-center gap-2">
              <span className="min-w-0 flex-1 truncate text-sm font-medium">{ch.title}</span>
              <Badge variant="outline" className="text-[10px]">{ch.items.length} 题</Badge>
              <Button size="xs" variant="outline" onClick={() => openPicker(ch.id)}>
                <PlusIcon data-icon="inline-start" /> 从题库加题
              </Button>
            </div>
            <ul className="divide-y">
              {ch.items.map((it) => (
                <li key={it.id} className="flex items-center gap-2 py-1 text-sm">
                  <span className="w-5 text-center text-[10px] text-muted-foreground">{it.orderNo}</span>
                  <span className="min-w-0 flex-1 truncate">{it.problemTitle || `#${it.problemId}`}</span>
                  {it.problemType && <Badge variant="secondary" className="text-[10px]">{typeLabel(it.problemType)}</Badge>}
                  <button
                    type="button"
                    title="从章节移除"
                    className="text-muted-foreground hover:text-destructive"
                    onClick={() => {
                      void api.deleteSpaceItem(it.id).then(() => {
                        toast.success('已移除')
                        invalidate()
                      }).catch((e) => toast.error(e instanceof Error ? e.message : '移除失败'))
                    }}
                  >
                    <XIcon className="size-3.5" />
                  </button>
                </li>
              ))}
              {ch.items.length === 0 && <li className="py-1 text-xs text-muted-foreground">空章节</li>}
            </ul>
          </div>
        ))
      )}

      {/* 从仓库题库选题加入章节 */}
      {pickerChapter != null && (
        <ProblemPickerDialog
          open={pickerOpen}
          onOpenChange={(v) => {
            setPickerOpen(v)
            if (!v) {
              setAddingTo(null)
              setPickerChapter(null)
            }
          }}
          title={`向章节加题${addingTo != null ? `（${chapters.find((c) => c.id === addingTo)?.title ?? ''}）` : ''}`}
          existingIds={chapters.find((c) => c.id === pickerChapter)?.items.map((i) => i.problemId) ?? []}
          onSubmit={async (problemIds) => {
            try {
              await api.addSpaceChapterItems(spaceId, pickerChapter, problemIds)
              toast.success(`已加入 ${problemIds.length} 道题目`)
              invalidate()
              setPickerOpen(false)
              setAddingTo(null)
              setPickerChapter(null)
            } catch (e) {
              toast.error(e instanceof Error ? e.message : '加入失败')
            }
          }}
        />
      )}
    </div>
  )
}

// ---------- 空间内容：练习 ----------

function PracticesPanel({ spaceId }: { spaceId: number }) {
  const qc = useQueryClient()
  const listQ = useQuery({
    queryKey: ['space', spaceId, 'practices'],
    queryFn: () => api.spacePractices(spaceId),
  })
  const practices = listQ.data?.practices ?? []
  const [creating, setCreating] = useState(false)
  const [openId, setOpenId] = useState<number | null>(null)
  const invalidate = () => {
    void qc.invalidateQueries({ queryKey: ['space', spaceId, 'practices'] })
  }
  return (
    <div>
      <div className="mb-2 flex items-center gap-2">
        <span className="text-sm font-medium">空间练习（{practices.length}）</span>
        <div className="ml-auto">
          <Button size="xs" onClick={() => setCreating(true)}>
            <PlusIcon data-icon="inline-start" /> 新建练习
          </Button>
        </div>
      </div>
      {practices.length === 0 ? (
        <div className="rounded-lg border border-dashed px-3 py-3 text-center text-xs text-muted-foreground">暂无练习</div>
      ) : (
        <div className="space-y-1.5">
          {practices.map((p) => (
            <div key={p.id} className="rounded-lg border bg-background">
              <div className="flex items-center gap-2 px-3 py-2">
                <button
                  type="button"
                  className="flex min-w-0 flex-1 items-center gap-2 text-left"
                  onClick={() => setOpenId(openId === p.id ? null : p.id)}
                >
                  <ChevronRightIcon className={`size-4 shrink-0 text-muted-foreground transition-transform ${openId === p.id ? 'rotate-90' : ''}`} />
                  <ClipboardListIcon className="size-4 shrink-0 text-primary/70" />
                  <span className="min-w-0 flex-1 truncate text-sm">{p.title}</span>
                  <Badge variant="outline" className="shrink-0 text-[10px]">{p.problemCount} 题</Badge>
                </button>
                <Button
                  size="icon-xs"
                  variant="ghost"
                  className="text-destructive"
                  title="删除练习"
                  onClick={() => {
                    if (!confirm(`删除练习「${p.title}」？`)) return
                    void api.deleteSpacePractice(spaceId, p.id).then(() => {
                      toast.success('已删除')
                      if (openId === p.id) setOpenId(null)
                      invalidate()
                    }).catch((e) => toast.error(e instanceof Error ? e.message : '删除失败'))
                  }}
                >
                  <Trash2Icon />
                </Button>
              </div>
              {openId === p.id && <PracticeDetailBody spaceId={spaceId} practiceId={p.id} onChanged={invalidate} />}
            </div>
          ))}
        </div>
      )}
      <CreateSpacePracticeDialog
        spaceId={spaceId}
        open={creating}
        onOpenChange={setCreating}
        onCreated={() => invalidate()}
      />
    </div>
  )
}

function PracticeDetailBody({ spaceId, practiceId, onChanged }: { spaceId: number; practiceId: number; onChanged: () => void }) {
  const qc = useQueryClient()
  const q = useQuery({
    queryKey: ['space', spaceId, 'practices', practiceId],
    queryFn: () => api.getSpacePractice(spaceId, practiceId),
  })
  const [pickerOpen, setPickerOpen] = useState(false)
  const invalidate = () => {
    void qc.invalidateQueries({ queryKey: ['space', spaceId, 'practices', practiceId] })
    onChanged()
  }
  if (q.isLoading) {
    return <div className="flex items-center gap-2 px-4 py-3 text-xs text-muted-foreground"><LoaderCircleIcon className="size-3.5 animate-spin" /> 加载中…</div>
  }
  const items = q.data?.items ?? []

  return (
    <div className="space-y-2 border-t bg-muted/30 px-4 py-3">
      <div className="flex gap-1.5">
        <Button size="xs" variant="outline" onClick={() => setPickerOpen(true)}>
          <PlusIcon data-icon="inline-start" /> 从题库加题
        </Button>
      </div>
      {items.length === 0 ? (
        <p className="text-xs text-muted-foreground">还没有题目。</p>
      ) : (
        <ul className="divide-y rounded-lg border bg-background px-2">
          {items.map((it) => (
            <li key={it.id} className="flex items-center gap-2 py-1.5 text-sm">
              <span className="w-5 text-center text-[10px] text-muted-foreground">{it.orderNo}</span>
              <span className="min-w-0 flex-1 truncate">{it.problemTitle || `#${it.problemId}`}</span>
              {it.problemType && <Badge variant="secondary" className="text-[10px]">{typeLabel(it.problemType)}</Badge>}
              <button
                type="button"
                title="移除"
                className="text-muted-foreground hover:text-destructive"
                onClick={() => {
                  void api.deleteSpaceItem(it.id).then(() => {
                    toast.success('已移除')
                    invalidate()
                  }).catch((e) => toast.error(e instanceof Error ? e.message : '移除失败'))
                }}
              >
                <XIcon className="size-3.5" />
              </button>
            </li>
          ))}
        </ul>
      )}

      <ProblemPickerDialog
        open={pickerOpen}
        onOpenChange={setPickerOpen}
        title="向练习加题"
        existingIds={items.map((i) => i.problemId)}
        onSubmit={async (problemIds) => {
          try {
            await api.addSpacePracticeItems(spaceId, practiceId, problemIds)
            toast.success(`已加入 ${problemIds.length} 道题目`)
            setPickerOpen(false)
            invalidate()
          } catch (e) {
            toast.error(e instanceof Error ? e.message : '加入失败')
          }
        }}
      />
    </div>
  )
}

// ---------- 空间内容：刷题项目 ----------

function QuizzesPanel({ spaceId }: { spaceId: number }) {
  const qc = useQueryClient()
  const listQ = useQuery({
    queryKey: ['space', spaceId, 'quizzes'],
    queryFn: () => api.spaceQuizzes(spaceId),
  })
  const quizzes = listQ.data?.quizzes ?? []
  const [creating, setCreating] = useState(false)
  const invalidate = () => {
    void qc.invalidateQueries({ queryKey: ['space', spaceId, 'quizzes'] })
  }
  return (
    <div>
      <div className="mb-2 flex items-center gap-2">
        <span className="text-sm font-medium">刷题项目（{quizzes.length}）</span>
        <div className="ml-auto">
          <Button size="xs" onClick={() => setCreating(true)}>
            <PlusIcon data-icon="inline-start" /> 新建刷题项目
          </Button>
        </div>
      </div>
      {quizzes.length === 0 ? (
        <div className="rounded-lg border border-dashed px-3 py-3 text-center text-xs text-muted-foreground">暂无刷题项目</div>
      ) : (
        <div className="space-y-1.5">
          {quizzes.map((qz) => (
            <div key={qz.id} className="flex items-center gap-2 rounded-lg border bg-background px-3 py-2">
              <FileStackIcon className="size-4 shrink-0 text-primary/70" />
              <span className="min-w-0 flex-1 truncate text-sm">{qz.title}</span>
              <Badge variant="secondary" className="shrink-0 text-[10px]">
                {qz.sourceType === 'tags' ? '按标签' : '按仓库'}
              </Badge>
              <Button
                size="icon-xs"
                variant="ghost"
                className="text-destructive"
                title="删除"
                onClick={() => {
                  if (!confirm(`删除刷题项目「${qz.title}」？`)) return
                  void api.deleteSpaceQuiz(spaceId, qz.id).then(() => {
                    toast.success('已删除')
                    invalidate()
                  }).catch((e) => toast.error(e instanceof Error ? e.message : '删除失败'))
                }}
              >
                <Trash2Icon />
              </Button>
            </div>
          ))}
        </div>
      )}
      <CreateSpaceQuizDialog spaceId={spaceId} open={creating} onOpenChange={setCreating} onCreated={() => invalidate()} />
    </div>
  )
}

// ---------- 从仓库模板选择（新建训练/练习/刷题共用） ----------

function RepoPicker(props: {
  repoKind: 'training' | 'practice'
  onRepoKind: (k: 'training' | 'practice') => void
  value: string
  onChange: (v: string) => void
}) {
  const isTraining = props.repoKind === 'training'
  const trainingsQ = useQuery({ queryKey: ['trainings'], queryFn: api.trainings, enabled: isTraining })
  const practicesQ = useQuery({ queryKey: ['practices'], queryFn: api.practices, enabled: !isTraining })
  const list = isTraining ? (trainingsQ.data?.trainings ?? []) : (practicesQ.data?.practices ?? [])

  return (
    <div className="space-y-2">
      <div className="flex gap-1.5">
        <Button size="xs" variant={isTraining ? 'default' : 'outline'} onClick={() => props.onRepoKind('training')}>
          仓库训练
        </Button>
        <Button size="xs" variant={!isTraining ? 'default' : 'outline'} onClick={() => props.onRepoKind('practice')}>
          仓库练习
        </Button>
      </div>
      <p className="text-xs text-muted-foreground">模板须属于当前域；拷贝时仅复制章节结构并引用同域题目。</p>
      {list.length === 0 ? (
        <p className="text-xs text-muted-foreground">当前域暂无{isTraining ? '训练' : '练习'}模板。</p>
      ) : (
        <div className="max-h-44 space-y-1 overflow-y-auto">
          {list.map((t) => (
            <label
              key={t.id}
              className={`flex cursor-pointer items-center gap-2 rounded-md px-2 py-1 text-sm hover:bg-muted ${
                props.value === String(t.id) ? 'bg-primary/5' : ''
              }`}
            >
              <input
                type="radio"
                name="repopicker"
                className="size-3.5 accent-[var(--primary)]"
                checked={props.value === String(t.id)}
                onChange={() => props.onChange(String(t.id))}
              />
              <span className="min-w-0 flex-1 truncate">{t.title}</span>
              <span className="text-[10px] text-muted-foreground">{t.problemCount} 题</span>
            </label>
          ))}
        </div>
      )}
    </div>
  )
}

function CreateSpaceTrainingDialog(props: { spaceId: number; open: boolean; onOpenChange: (v: boolean) => void; onCreated: () => void }) {
  const [title, setTitle] = useState('')
  const [maxAttempts, setMaxAttempts] = useState('')
  const [fromRepo, setFromRepo] = useState(false)
  const [repoKind, setRepoKind] = useState<'training' | 'practice'>('training')
  const [repoId, setRepoId] = useState('')
  const [busy, setBusy] = useState(false)

  async function submit() {
    if (!title.trim()) {
      toast.error('请输入训练标题')
      return
    }
    setBusy(true)
    try {
      const body: { title: string; maxAttempts?: number; fromRepo?: { kind: 'training' | 'practice'; id: number } } = {
        title: title.trim(),
      }
      const ma = Number(maxAttempts)
      if (maxAttempts.trim() !== '' && Number.isFinite(ma) && ma > 0) body.maxAttempts = ma
      if (fromRepo && repoId) body.fromRepo = { kind: repoKind, id: Number(repoId) }
      await api.createSpaceTraining(props.spaceId, body)
      toast.success('训练已创建')
      props.onCreated()
      props.onOpenChange(false)
      setTitle('')
      setMaxAttempts('')
      setFromRepo(false)
      setRepoId('')
    } catch (e) {
      toast.error(e instanceof Error ? e.message : '创建失败')
    } finally {
      setBusy(false)
    }
  }

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-md">
        <DialogHeader>
          <DialogTitle>新建空间训练</DialogTitle>
          <DialogDescription>可自建（随后加章节/题目），或从当前域仓库模板拷贝结构。</DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          <div className="space-y-1.5">
            <Label>训练标题</Label>
            <Input value={title} onChange={(e) => setTitle(e.target.value)} placeholder="例如：第三章复习训练" autoFocus />
          </div>
          <div className="space-y-1.5">
            <Label>作答次数上限（空=默认 3；0 不限暂不可用）</Label>
            <Input value={maxAttempts} onChange={(e) => setMaxAttempts(e.target.value)} placeholder="留空使用默认 3" inputMode="numeric" />
          </div>
          <div className="space-y-1.5">
            <Label className="flex items-center gap-2">
              <input
                type="checkbox"
                className="size-3.5 accent-[var(--primary)]"
                checked={fromRepo}
                onChange={(e) => setFromRepo(e.target.checked)}
              />
              从仓库模板拷贝结构
            </Label>
            {fromRepo && (
              <RepoPicker repoKind={repoKind} onRepoKind={setRepoKind} value={repoId} onChange={setRepoId} />
            )}
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => props.onOpenChange(false)}>取消</Button>
          <Button onClick={() => void submit()} disabled={busy || !title.trim() || (fromRepo && !repoId)}>
            {busy ? '创建中…' : '创建'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function CreateSpacePracticeDialog(props: { spaceId: number; open: boolean; onOpenChange: (v: boolean) => void; onCreated: () => void }) {
  const [title, setTitle] = useState('')
  const [fromRepo, setFromRepo] = useState(false)
  const [repoKind, setRepoKind] = useState<'training' | 'practice'>('practice')
  const [repoId, setRepoId] = useState('')
  const [busy, setBusy] = useState(false)

  async function submit() {
    if (!title.trim()) {
      toast.error('请输入练习标题')
      return
    }
    setBusy(true)
    try {
      const body: { title: string; fromRepo?: { kind: 'training' | 'practice'; id: number } } = { title: title.trim() }
      if (fromRepo && repoId) body.fromRepo = { kind: repoKind, id: Number(repoId) }
      await api.createSpacePractice(props.spaceId, body)
      toast.success('练习已创建')
      props.onCreated()
      props.onOpenChange(false)
      setTitle('')
      setFromRepo(false)
      setRepoId('')
    } catch (e) {
      toast.error(e instanceof Error ? e.message : '创建失败')
    } finally {
      setBusy(false)
    }
  }

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-md">
        <DialogHeader>
          <DialogTitle>新建空间练习</DialogTitle>
          <DialogDescription>整卷作答后统一交卷；可自建（随后按 ID 加题），或从当前域仓库模板拷贝。</DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          <div className="space-y-1.5">
            <Label>练习标题</Label>
            <Input value={title} onChange={(e) => setTitle(e.target.value)} placeholder="例如：期中整卷" autoFocus />
          </div>
          <div className="space-y-1.5">
            <Label className="flex items-center gap-2">
              <input
                type="checkbox"
                className="size-3.5 accent-[var(--primary)]"
                checked={fromRepo}
                onChange={(e) => setFromRepo(e.target.checked)}
              />
              从仓库模板拷贝题目
            </Label>
            {fromRepo && (
              <RepoPicker repoKind={repoKind} onRepoKind={setRepoKind} value={repoId} onChange={setRepoId} />
            )}
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => props.onOpenChange(false)}>取消</Button>
          <Button onClick={() => void submit()} disabled={busy || !title.trim() || (fromRepo && !repoId)}>
            {busy ? '创建中…' : '创建'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function CreateSpaceQuizDialog(props: { spaceId: number; open: boolean; onOpenChange: (v: boolean) => void; onCreated: () => void }) {
  const [title, setTitle] = useState('')
  const [sourceType, setSourceType] = useState<'tags' | 'repo'>('repo')
  const [repoKind, setRepoKind] = useState<'training' | 'practice'>('training')
  const [repoId, setRepoId] = useState('')
  const [busy, setBusy] = useState(false)

  async function submit() {
    if (!title.trim()) {
      toast.error('请输入项目标题')
      return
    }
    if (sourceType === 'repo' && !repoId) {
      toast.error('请选择仓库模板')
      return
    }
    setBusy(true)
    try {
      await api.createSpaceQuiz(props.spaceId, {
        title: title.trim(),
        sourceType,
        ...(sourceType === 'repo' ? { repoKind, repoId: Number(repoId) } : {}),
      })
      toast.success('刷题项目已创建')
      props.onCreated()
      props.onOpenChange(false)
      setTitle('')
      setSourceType('repo')
      setRepoId('')
    } catch (e) {
      toast.error(e instanceof Error ? e.message : '创建失败')
    } finally {
      setBusy(false)
    }
  }

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-md">
        <DialogHeader>
          <DialogTitle>新建刷题项目</DialogTitle>
        </DialogHeader>
        <div className="space-y-3">
          <div className="space-y-1.5">
            <Label>项目标题</Label>
            <Input value={title} onChange={(e) => setTitle(e.target.value)} placeholder="例如：每日一练" autoFocus />
          </div>
          <div className="space-y-1.5">
            <Label>题目来源</Label>
            <Select
              items={[
                { value: 'repo', label: '仓库模板' },
                { value: 'tags', label: '按标签筛选' },
              ]}
              value={sourceType}
              onValueChange={(v) => setSourceType(v as 'tags' | 'repo')}
            >
              <SelectTrigger className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="repo">仓库模板</SelectItem>
                <SelectItem value="tags">按标签筛选</SelectItem>
              </SelectContent>
            </Select>
          </div>
          {sourceType === 'repo' && (
            <RepoPicker repoKind={repoKind} onRepoKind={setRepoKind} value={repoId} onChange={setRepoId} />
          )}
          {sourceType === 'tags' && (
            <p className="text-xs text-muted-foreground">按标签的刷题项目在前台门户中配置标签；此处先创建标题即可。</p>
          )}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => props.onOpenChange(false)}>取消</Button>
          <Button onClick={() => void submit()} disabled={busy || !title.trim()}>
            {busy ? '创建中…' : '创建'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

// ---------- 空间 CRUD 对话框 ----------

function CreateSpaceDialog(props: { open: boolean; onOpenChange: (v: boolean) => void; onCreated: () => void }) {
  const [name, setName] = useState('')
  const [busy, setBusy] = useState(false)
  async function submit() {
    if (!name.trim()) {
      toast.error('请输入空间名称')
      return
    }
    setBusy(true)
    try {
      await api.createSpace(name.trim())
      toast.success(`空间「${name.trim()}」已创建`)
      props.onCreated()
      props.onOpenChange(false)
      setName('')
    } catch (e) {
      toast.error(e instanceof Error ? e.message : '创建失败')
    } finally {
      setBusy(false)
    }
  }
  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className="sm:max-w-xs">
        <DialogHeader>
          <DialogTitle>新建空间</DialogTitle>
          <DialogDescription>空间 = 做题组织单位（训练/练习/作答隔离）。</DialogDescription>
        </DialogHeader>
        <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="例如：高一（3）班" autoFocus onKeyDown={(e) => e.key === 'Enter' && void submit()} />
        <DialogFooter>
          <Button variant="outline" onClick={() => props.onOpenChange(false)}>取消</Button>
          <Button onClick={() => void submit()} disabled={busy || !name.trim()}>
            {busy ? '创建中…' : '创建'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function RenameSpaceDialog(props: { space: Space | null; onOpenChange: (v: boolean) => void; onDone: () => void }) {
  const [name, setName] = useState('')
  const [busy, setBusy] = useState(false)
  const sp = props.space
  const [lastId, setLastId] = useState<number | null>(null)
  if (sp && lastId !== sp.id) {
    setLastId(sp.id)
    setName(sp.name)
  } else if (!sp && lastId !== null) {
    setLastId(null)
  }
  async function save() {
    if (!sp || !name.trim()) return
    setBusy(true)
    try {
      await api.renameSpace(sp.id, name.trim())
      toast.success('空间已重命名')
      props.onDone()
      props.onOpenChange(false)
    } catch (e) {
      toast.error(e instanceof Error ? e.message : '重命名失败')
    } finally {
      setBusy(false)
    }
  }
  return (
    <Dialog open={sp !== null} onOpenChange={props.onOpenChange}>
      <DialogContent className="sm:max-w-xs">
        <DialogHeader>
          <DialogTitle>重命名空间</DialogTitle>
        </DialogHeader>
        <Input value={name} onChange={(e) => setName(e.target.value)} autoFocus onKeyDown={(e) => e.key === 'Enter' && void save()} />
        <DialogFooter>
          <Button variant="outline" onClick={() => props.onOpenChange(false)}>取消</Button>
          <Button onClick={() => void save()} disabled={!name.trim() || busy}>
            {busy ? '保存中…' : '保存'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function typeLabel(t: string): string {
  if (t === 'programming') return '编程'
  if (t === 'single_choice') return '单选'
  if (t === 'true_false') return '判断'
  return t || '?'
}

// ---------- 从仓库题库选题（空间训练/练习加题） ----------
// 题目来自当前域仓库（api.problems 走 dq 自动带 domainId），已在本空间目标中的题目置灰。

function ProblemPickerDialog(props: {
  open: boolean
  onOpenChange: (v: boolean) => void
  title: string
  /** 已在此空间目标（章节/练习）里的题目 ID，置灰不可重复加入。 */
  existingIds?: number[]
  /** 提交所选题目 ID。调用方负责调 API 并关闭。 */
  onSubmit: (problemIds: number[]) => void
}) {
  const [query, setQuery] = useState('')
  const [selected, setSelected] = useState<Set<number>>(new Set())
  const [submitting, setSubmitting] = useState(false)
  const problemsQ = useQuery({
    queryKey: ['space-problem-picker'],
    queryFn: () => api.problems({ q: '', tags: [], type: '' }),
    enabled: props.open,
  })
  const all = problemsQ.data?.problems ?? []
  const existing = new Set(props.existingIds ?? [])
  const q = query.trim().toLowerCase()
  const visible = q ? all.filter((p) => p.title.toLowerCase().includes(q) || String(p.id).includes(q)) : all

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{props.title}</DialogTitle>
          <DialogDescription>从当前域仓库题库中选择题目加入。已加入的题目置灰。</DialogDescription>
        </DialogHeader>

        <div className="relative">
          <SearchIcon className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="搜索标题 / ID…"
            className="pl-8"
          />
        </div>

        <div className="max-h-72 space-y-1 overflow-y-auto">
          {problemsQ.isLoading ? (
            <div className="flex items-center justify-center gap-2 py-6 text-xs text-muted-foreground">
              <LoaderCircleIcon className="size-4 animate-spin" /> 加载题目…
            </div>
          ) : visible.length === 0 ? (
            <p className="py-6 text-center text-xs text-muted-foreground">没有匹配的题目</p>
          ) : (
            visible.map((p) => {
              const isExisting = existing.has(p.id)
              const isSelected = selected.has(p.id)
              return (
                <label
                  key={p.id}
                  className={`flex cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-sm hover:bg-muted ${
                    isExisting ? 'cursor-not-allowed opacity-50' : isSelected ? 'bg-primary/5' : ''
                  }`}
                >
                  <input
                    type="checkbox"
                    className="size-3.5 accent-[var(--primary)]"
                    disabled={isExisting}
                    checked={isSelected || isExisting}
                    onChange={() =>
                      setSelected((prev) => {
                        const next = new Set(prev)
                        if (next.has(p.id)) next.delete(p.id)
                        else next.add(p.id)
                        return next
                      })
                    }
                  />
                  <Badge variant="outline" className="shrink-0 px-1.5 text-[10px] text-muted-foreground">
                    {typeLabel(p.type)}
                  </Badge>
                  <span className="min-w-0 flex-1 truncate">{p.title}</span>
                  <span className="shrink-0 text-[10px] text-muted-foreground">#{p.id}</span>
                  {isExisting && <span className="shrink-0 text-[10px] text-muted-foreground">已加入</span>}
                </label>
              )
            })
          )}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => props.onOpenChange(false)}>取消</Button>
          <Button
            disabled={selected.size === 0 || submitting}
            onClick={async () => {
              setSubmitting(true)
              try {
                await props.onSubmit([...selected])
              } finally {
                setSubmitting(false)
                setSelected(new Set())
              }
            }}
          >
            {submitting ? '加入中…' : `加入所选（${selected.size}）`}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
