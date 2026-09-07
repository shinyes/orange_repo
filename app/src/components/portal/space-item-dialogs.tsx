// 门户列表页（训练/练习）共享：管理员新建项目简易弹窗 + 可见成员分配弹窗。
// 与 SpaceAdmin（管理端）功能对齐的轻量版：标题/描述/训练限次；可见成员覆盖式。
import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { toast } from 'sonner'

import { api } from '@/api'
import { Button } from '@/components/ui/button'
import {
  Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'

// ---------- 新建训练（门户管理员） ----------

export function NewTrainingDialog(props: {
  spaceId: number
  open: boolean
  onOpenChange: (v: boolean) => void
  onCreated: () => void
}) {
  const [title, setTitle] = useState('')
  const [description, setDescription] = useState('')
  const [maxAttempts, setMaxAttempts] = useState('3')
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    if (props.open) {
      setTitle(''); setDescription(''); setMaxAttempts('3')
    }
  }, [props.open])

  async function create() {
    const t = title.trim()
    if (!t) {
      toast.error('请输入训练标题')
      return
    }
    const ma = Number(maxAttempts)
    setBusy(true)
    try {
      await api.createSpaceTraining(props.spaceId, {
        title: t,
        description: description.trim() || undefined,
        maxAttempts: Number.isFinite(ma) && ma > 0 ? ma : undefined,
      })
      toast.success('训练已创建（默认无成员可见，可点眼睛按钮分配）')
      props.onCreated()
      props.onOpenChange(false)
    } catch (e) {
      toast.error(e instanceof Error ? e.message : '创建失败')
    } finally {
      setBusy(false)
    }
  }

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>新建训练</DialogTitle>
          <DialogDescription>创建后默认无成员可见，请用卡片上的眼睛按钮分配可见成员。</DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          <div className="space-y-1.5">
            <Label>训练标题</Label>
            <Input value={title} onChange={(e) => setTitle(e.target.value)} placeholder="如：C++ 基础训练" autoFocus />
          </div>
          <div className="space-y-1.5">
            <Label>描述（可选）</Label>
            <Textarea rows={2} value={description} onChange={(e) => setDescription(e.target.value)} placeholder="训练内容简介" />
          </div>
          <div className="space-y-1.5">
            <Label>客观题每人限答次数</Label>
            <Input type="number" min={0} value={maxAttempts} onChange={(e) => setMaxAttempts(e.target.value)} />
            <p className="text-[11px] text-muted-foreground">0 = 不限次数</p>
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => props.onOpenChange(false)}>取消</Button>
          <Button onClick={() => void create()} disabled={busy}>{busy ? '创建中…' : '创建'}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

// ---------- 新建练习（门户管理员） ----------

export function NewPracticeDialog(props: {
  spaceId: number
  open: boolean
  onOpenChange: (v: boolean) => void
  onCreated: () => void
}) {
  const [title, setTitle] = useState('')
  const [description, setDescription] = useState('')
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    if (props.open) { setTitle(''); setDescription('') }
  }, [props.open])

  async function create() {
    const t = title.trim()
    if (!t) {
      toast.error('请输入练习标题')
      return
    }
    setBusy(true)
    try {
      await api.createSpacePractice(props.spaceId, { title: t, description: description.trim() || undefined })
      toast.success('练习已创建（默认无成员可见，可点眼睛按钮分配）')
      props.onCreated()
      props.onOpenChange(false)
    } catch (e) {
      toast.error(e instanceof Error ? e.message : '创建失败')
    } finally {
      setBusy(false)
    }
  }

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>新建练习</DialogTitle>
          <DialogDescription>创建后默认无成员可见，请用卡片上的眼睛按钮分配可见成员。</DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          <div className="space-y-1.5">
            <Label>练习标题</Label>
            <Input value={title} onChange={(e) => setTitle(e.target.value)} placeholder="如：期中模拟卷" autoFocus />
          </div>
          <div className="space-y-1.5">
            <Label>描述（可选）</Label>
            <Textarea rows={2} value={description} onChange={(e) => setDescription(e.target.value)} placeholder="练习说明" />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => props.onOpenChange(false)}>取消</Button>
          <Button onClick={() => void create()} disabled={busy}>{busy ? '创建中…' : '创建'}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

// ---------- 可见成员分配（门户管理员） ----------

export function VisibleUsersDialog(props: {
  spaceId: number
  kind: 'training' | 'practice'
  itemId: number
  title: string
  open: boolean
  onClose: () => void
  onSaved: () => void
}) {
  const [selected, setSelected] = useState<Set<number>>(new Set())
  const [busy, setBusy] = useState(false)
  const membersQ = useQuery({
    queryKey: ['space', props.spaceId, 'members'],
    queryFn: () => api.spaceMembers(props.spaceId),
  })
  const visQ = useQuery({
    queryKey: ['space-visible', props.kind, props.itemId],
    queryFn: () => api.visibleUsers(props.kind, props.spaceId, props.itemId),
    enabled: props.open,
  })
  useEffect(() => {
    if (props.open && visQ.data) {
      setSelected(new Set(visQ.data.userIds))
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [props.open, props.itemId, visQ.data])

  const members = membersQ.data?.members ?? []

  async function save() {
    setBusy(true)
    try {
      await api.setVisibleUsers(props.kind, props.spaceId, props.itemId, [...selected])
      toast.success(selected.size === 0 ? '已设为无成员可见（仅管理员）' : `已分配 ${selected.size} 位成员可见`)
      props.onSaved()
      props.onClose()
    } catch (e) {
      toast.error(e instanceof Error ? e.message : '保存失败')
    } finally {
      setBusy(false)
    }
  }

  const toggle = (uid: number) => {
    setSelected((prev) => {
      const n = new Set(prev)
      if (n.has(uid)) n.delete(uid)
      else n.add(uid)
      return n
    })
  }

  return (
    <Dialog open={props.open} onOpenChange={(v) => !v && props.onClose()}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>可见成员 · {props.title}</DialogTitle>
          <DialogDescription>
            仅被分配的成员能在门户看到并进入该项目；默认无成员可见（管理员始终可见）。
          </DialogDescription>
        </DialogHeader>
        <div className="max-h-72 space-y-1 overflow-y-auto">
          {members.length === 0 ? (
            <p className="py-6 text-center text-xs text-muted-foreground">该空间暂无成员，请先在「空间管理」添加成员。</p>
          ) : (
            members.map((m) => (
              <label key={m.userId} className="flex cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-sm hover:bg-muted">
                <input type="checkbox" className="size-4 accent-[var(--primary)]" checked={selected.has(m.userId)} onChange={() => toggle(m.userId)} />
                <span className="min-w-0 flex-1 truncate">{m.username}</span>
              </label>
            ))
          )}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={props.onClose}>取消</Button>
          <Button onClick={() => void save()} disabled={busy}>
            {busy ? '保存中…' : '保存分配'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
