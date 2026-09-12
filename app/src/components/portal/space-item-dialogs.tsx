// 门户列表页（训练/练习）共享：管理员新建项目简易弹窗 + 可见成员分配弹窗。
// 与 SpaceAdmin（管理端）功能对齐的轻量版：标题/描述/训练限次；可见成员覆盖式。
import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { toast } from 'sonner'

import { api } from '@/api'
import type { QuizBrief } from '@/api/types'
import { Button } from '@/components/ui/button'
import {
  Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { PublicToggleRow } from '@/components/portal/public-toggle'

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
  const [isPublic, setIsPublic] = useState(false)
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    if (props.open) {
      setTitle(''); setDescription(''); setMaxAttempts('3'); setIsPublic(false)
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
        isPublic,
      })
      toast.success(isPublic ? '训练已创建并公开（空间内所有成员可见）' : '训练已创建（默认无成员可见，可点眼睛按钮分配）')
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
          <DialogDescription>设置标题/描述与限次；未开启公开时默认无成员可见，可在卡片上用 👁 按钮分配。</DialogDescription>
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
          <PublicToggleRow checked={isPublic} disabled={busy} onCheckedChange={setIsPublic} />
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
  const [isPublic, setIsPublic] = useState(false)
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    if (props.open) { setTitle(''); setDescription(''); setIsPublic(false) }
  }, [props.open])

  async function create() {
    const t = title.trim()
    if (!t) {
      toast.error('请输入练习标题')
      return
    }
    setBusy(true)
    try {
      await api.createSpacePractice(props.spaceId, { title: t, description: description.trim() || undefined, isPublic })
      toast.success(isPublic ? '练习已创建并公开（空间内所有成员可见）' : '练习已创建（默认无成员可见，可点眼睛按钮分配）')
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
          <DialogDescription>设置标题/描述；未开启公开时默认无成员可见，可在卡片上用 👁 按钮分配。</DialogDescription>
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
          <PublicToggleRow checked={isPublic} disabled={busy} onCheckedChange={setIsPublic} />
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => props.onOpenChange(false)}>取消</Button>
          <Button onClick={() => void create()} disabled={busy}>{busy ? '创建中…' : '创建'}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

// ---------- 新建刷题项目（门户管理员；默认规则：做过少做/错过多做/同批不重复） ----------

export function NewQuizDialog(props: {
  spaceId: number
  open: boolean
  onOpenChange: (v: boolean) => void
  onCreated: () => void
}) {
  const [title, setTitle] = useState('')
  const [tags, setTags] = useState('')
  const [roundSize, setRoundSize] = useState('')
  const [isPublic, setIsPublic] = useState(false)
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    if (props.open) { setTitle(''); setTags(''); setRoundSize(''); setIsPublic(false) }
  }, [props.open])

  async function create() {
    const t = title.trim()
    if (!t) {
      toast.error('请输入刷题项目标题')
      return
    }
    const rs = roundSize.trim() === '' ? 0 : Number(roundSize)
    if (Number.isNaN(rs) || rs < 0 || rs > 200) {
      toast.error('每轮题数须为 1~200 的整数（留空=不限制）')
      return
    }
    setBusy(true)
    try {
      const tagList = tags.split(/[,，\s]+/).map((x) => x.trim()).filter(Boolean)
      await api.createSpaceQuiz(props.spaceId, {
        title: t,
        tags: tagList.length > 0 ? tagList : undefined,
        sourceType: 'tags',
        roundSize: rs,
        isPublic,
      })
      toast.success(
        isPublic
          ? '刷题项目已创建并公开（空间内所有成员可见）'
          : rs > 0
            ? `刷题项目已创建（每轮 ${rs} 题，默认无成员可见，可点眼睛分配）`
            : '刷题项目已创建（默认无成员可见，可点眼睛分配）',
      )
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
          <DialogTitle>新建刷题项目</DialogTitle>
          <DialogDescription>
            范围内单选/判断题循环复习：做过少做、答错的下轮多做、同轮不重复。未开启公开时默认无成员可见，可在卡片上用 👁 按钮分配。
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          <div className="space-y-1.5">
            <Label>项目标题</Label>
            <Input value={title} onChange={(e) => setTitle(e.target.value)} placeholder="如：C++ 基础刷题" autoFocus />
          </div>
          <div className="space-y-1.5">
            <Label>范围标签（可选，逗号分隔）</Label>
            <Input value={tags} onChange={(e) => setTags(e.target.value)} placeholder="如：语法基础, 循环 —— 留空=域内全部客观题" />
          </div>
          <div className="space-y-1.5">
            <Label>每轮题目数量（留空=不限，整范围为一轮）</Label>
            <Input
              type="number"
              min={1}
              max={200}
              value={roundSize}
              onChange={(e) => setRoundSize(e.target.value)}
              placeholder="如：10 —— 每轮最多抽 10 题，答完开下一轮"
            />
          </div>
          <PublicToggleRow checked={isPublic} disabled={busy} onCheckedChange={setIsPublic} />
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => props.onOpenChange(false)}>取消</Button>
          <Button onClick={() => void create()} disabled={busy}>{busy ? '创建中…' : '创建'}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

// ---------- 编辑刷题项目（标题/范围标签/每轮题数；可见成员走卡片独立按钮） ----------

export function QuizEditDialog(props: {
  spaceId: number
  quiz: QuizBrief
  open: boolean
  onOpenChange: (v: boolean) => void
  onSaved: () => void
}) {
  const [title, setTitle] = useState(props.quiz.title)
  const [tags, setTags] = useState((props.quiz.tags ?? []).join(', '))
  const [roundSize, setRoundSize] = useState(String(props.quiz.roundSize ?? 0))
  const [isPublic, setIsPublic] = useState(!!props.quiz.isPublic)
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    if (props.open) {
      setTitle(props.quiz.title)
      setTags((props.quiz.tags ?? []).join(', '))
      setRoundSize(String(props.quiz.roundSize ?? 0))
      setIsPublic(!!props.quiz.isPublic)
    }
  }, [props.open, props.quiz])

  async function save() {
    const t = title.trim()
    if (!t) {
      toast.error('请输入刷题项目标题')
      return
    }
    const rs = roundSize.trim() === '' ? 0 : Number(roundSize)
    if (Number.isNaN(rs) || rs < 0 || rs > 200) {
      toast.error('每轮题数须为 1~200 的整数（0=不限制）')
      return
    }
    setBusy(true)
    try {
      const tagList = tags.split(/[,，\s]+/).map((x) => x.trim()).filter(Boolean)
      await api.updateSpaceQuiz(props.spaceId, props.quiz.id, {
        title: t,
        tags: tagList,
        roundSize: rs,
        isPublic,
      })
      toast.success(isPublic ? '已保存并公开（空间内所有成员可见）' : rs > 0 ? '已保存（每轮 ' + rs + ' 题）' : '已保存（整范围一轮）')
      props.onSaved()
      props.onOpenChange(false)
    } catch (e) {
      toast.error(e instanceof Error ? e.message : '保存失败')
    } finally {
      setBusy(false)
    }
  }

  return (
    <Dialog open={props.open} onOpenChange={(v) => !v && props.onOpenChange(false)}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>编辑刷题项目 · {props.quiz.title}</DialogTitle>
          <DialogDescription>修改标题/范围标签与每轮题数；可见成员请在卡片右侧的 👁 按钮设置。</DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          <div className="space-y-1.5">
            <Label>项目标题</Label>
            <Input value={title} onChange={(e) => setTitle(e.target.value)} />
          </div>
          <div className="space-y-1.5">
            <Label>范围标签（可选，逗号分隔）</Label>
            <Input value={tags} onChange={(e) => setTags(e.target.value)} placeholder="留空=域内全部客观题" />
          </div>
          <div className="space-y-1.5">
            <Label>每轮题目数量（0=不限，整范围为一轮）</Label>
            <Input type="number" min={0} max={200} value={roundSize} onChange={(e) => setRoundSize(e.target.value)} placeholder="如：10" />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => props.onOpenChange(false)}>取消</Button>
          <Button onClick={() => void save()} disabled={busy}>{busy ? '保存中…' : '保存'}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

// ---------- 可见性（公开开关 + 可见成员分配，门户管理员） ----------

export function VisibleUsersDialog(props: {
  spaceId: number
  kind: 'training' | 'practice' | 'quiz'
  itemId: number
  title: string
  /** 当前是否公开（公开=空间内所有成员可见，无需逐个分配） */
  isPublic?: boolean
  /** 切换公开：由调用方用完整字段调用对应更新接口（避免重置该项目的其它字段） */
  onTogglePublic?: (v: boolean) => Promise<void> | void
  open: boolean
  onClose: () => void
  onSaved: () => void
}) {
  const [selected, setSelected] = useState<Set<number>>(new Set())
  const [pub, setPub] = useState(!!props.isPublic)
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
  // 打开时同步外部最新公开状态（列表可能刚被其它操作刷新）
  useEffect(() => {
    if (props.open) setPub(!!props.isPublic)
  }, [props.open, props.itemId, props.isPublic])

  const members = membersQ.data?.members ?? []

  async function save() {
    setBusy(true)
    try {
      // 先保存公开状态（若变更），再保存成员名单
      if (props.onTogglePublic && pub !== !!props.isPublic) {
        await props.onTogglePublic(pub)
      }
      await api.setVisibleUsers(props.kind, props.spaceId, props.itemId, [...selected])
      toast.success(
        pub
          ? '已设为公开（空间内所有成员可见）'
          : selected.size === 0
            ? '已设为无成员可见（仅管理员）'
            : `已分配 ${selected.size} 位成员可见`,
      )
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
          <DialogTitle>可见性 · {props.title}</DialogTitle>
          <DialogDescription>
            公开后空间内所有成员可见；不公开则仅下列被分配的成员可见（默认无成员可见，管理员始终可见）。
          </DialogDescription>
        </DialogHeader>

        {/* 公开开关（原在编辑弹窗，现统一到本浮窗） */}
        {props.onTogglePublic && (
          <PublicToggleRow checked={pub} disabled={busy} onCheckedChange={setPub} />
        )}

        {!pub && (
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
        )}
        {pub && (
          <p className="rounded-lg border border-dashed px-3 py-2 text-xs text-muted-foreground">
            当前为公开：空间内所有成员都能看到并进入，无需逐个分配。
          </p>
        )}
        <DialogFooter>
          <Button variant="outline" onClick={props.onClose}>取消</Button>
          <Button onClick={() => void save()} disabled={busy}>
            {busy ? '保存中…' : '保存'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
