import { useState, type DragEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import {
  ArrowDownIcon,
  ArrowUpIcon,
  ChevronRightIcon,
  DownloadIcon,
  EyeIcon,
  FolderPlusIcon,
  GripVerticalIcon,
  PencilIcon,
  PlusIcon,
  TrashIcon,
} from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Separator } from '@/components/ui/separator'
import { api } from '@/lib/api'
import { useAppState } from '@/lib/app-context'
import type { Chapter, Item, PracticeItem } from '@/lib/types'
import { ConfirmDialog } from './dialogs'
import { Empty } from './ProblemPane'

type DragState =
  | { kind: 'item'; itemId: number; fromChapterId: number }
  | { kind: 'chapter'; chapterId: number }

// 训练详情（章节化，支持拖拽排序：条目可跨章节移动，章节可整体排序；拖章节时全部折叠）。
export function TrainingDetail({ id }: { id: number }) {
  const qc = useQueryClient()
  const { checked, goHome } = useAppState()
  const q = useQuery({ queryKey: ['training', id], queryFn: () => api.getTraining(id) })
  const [newChapter, setNewChapter] = useState('')
  const [confirmDelete, setConfirmDelete] = useState(false)
  const [collapsed, setCollapsed] = useState<Record<number, boolean>>({})
  const [editMode, setEditMode] = useState(true)
  const [editGroup, setEditGroup] = useState(false)

  // 拖拽状态提升到详情级：跨章节放置需要全局视野
  const [drag, setDrag] = useState<DragState | null>(null)
  const [itemIndicator, setItemIndicator] = useState<{ chapterId: number; index: number } | null>(null)
  const [chapterIndicator, setChapterIndicator] = useState<number | null>(null)

  const chapters = q.data?.chapters ?? []
  const draggingChapter = drag?.kind === 'chapter'

  async function invalidate() {
    await qc.invalidateQueries({ queryKey: ['training', id] })
    await qc.invalidateQueries({ queryKey: ['trainings'] })
  }

  function endDrag() {
    setDrag(null)
    setItemIndicator(null)
    setChapterIndicator(null)
  }

  async function commitLayout(next: Chapter[]) {
    try {
      await api.updateTrainingLayout(id, {
        chapterIds: next.map((c) => c.id),
        chapters: next.map((c) => ({ chapterId: c.id, itemIds: c.items.map((i) => i.id) })),
      })
      await invalidate()
    } catch (e) {
      toast.error(e instanceof Error ? e.message : '保存布局失败')
    } finally {
      endDrag()
    }
  }

  function layoutKey(cs: Chapter[]) {
    return JSON.stringify(cs.map((c) => [c.id, ...c.items.map((i) => i.id)]))
  }

  /** 把条目移到 (targetChapterId, targetIndex)；无实际变化返回 null。 */
  function moveItemTo(itemId: number, fromChapterId: number, targetChapterId: number, targetIndex: number): Chapter[] | null {
    const before = layoutKey(chapters)
    const next = chapters.map((c) => ({ ...c, items: [...c.items] }))
    const src = next.find((c) => c.id === fromChapterId)
    if (!src) return null
    const fromIdx = src.items.findIndex((i) => i.id === itemId)
    if (fromIdx === -1) return null
    const [moved] = src.items.splice(fromIdx, 1)
    const dst = next.find((c) => c.id === targetChapterId)
    if (!dst) return null
    let idx = targetIndex
    if (fromChapterId === targetChapterId && fromIdx < idx) idx--
    idx = Math.max(0, Math.min(idx, dst.items.length))
    dst.items.splice(idx, 0, moved)
    if (layoutKey(next) === before) return null
    return next
  }

  /** 把章节移到序列的 targetIndex 位置；无实际变化返回 null。 */
  function moveChapterTo(chapterId: number, targetIndex: number): Chapter[] | null {
    const ids = chapters.map((c) => c.id)
    const from = ids.indexOf(chapterId)
    if (from === -1) return null
    let idx = targetIndex
    if (from < idx) idx--
    if (idx === from) return null
    ids.splice(from, 1)
    ids.splice(idx, 0, chapterId)
    const byId = new Map(chapters.map((c) => [c.id, c]))
    return ids.map((cid) => byId.get(cid)!)
  }

  function handleDropItem(targetChapterId: number, targetIndex: number) {
    if (!drag || drag.kind !== 'item') return endDrag()
    const next = moveItemTo(drag.itemId, drag.fromChapterId, targetChapterId, targetIndex)
    if (next) void commitLayout(next)
    else endDrag()
  }

  function handleDropChapter() {
    if (!drag || drag.kind !== 'chapter' || chapterIndicator == null) return endDrag()
    const next = moveChapterTo(drag.chapterId, chapterIndicator)
    if (next) void commitLayout(next)
    else endDrag()
  }

  function handleItemDragOver(chapterId: number, index: number, e: DragEvent) {
    if (!drag || drag.kind !== 'item') return
    e.preventDefault()
    e.stopPropagation()
    e.dataTransfer.dropEffect = 'move'
    setItemIndicator((prev) => (prev?.chapterId === chapterId && prev.index === index ? prev : { chapterId, index }))
  }

  function handleChapterDragOver(index: number, e: DragEvent) {
    if (!drag || drag.kind !== 'chapter') return
    e.preventDefault()
    e.stopPropagation()
    e.dataTransfer.dropEffect = 'move'
    setChapterIndicator((prev) => (prev === index ? prev : index))
  }

  async function addChapter() {
    if (!newChapter.trim()) return
    try {
      await api.createChapter(id, newChapter.trim())
      setNewChapter('')
      await invalidate()
    } catch (e) {
      toast.error(e instanceof Error ? e.message : '创建章节失败')
    }
  }

  async function addCheckedTo(chapterId: number) {
    if (checked.length === 0) {
      toast.info('请先在左侧列表勾选要加入的题目')
      return
    }
    try {
      await api.addChapterItems(chapterId, checked)
      toast.success(`已加入 ${checked.length} 道题目`)
      await invalidate()
    } catch (e) {
      toast.error(e instanceof Error ? e.message : '加入失败')
    }
  }

  if (!q.data) return <div className="p-8 text-sm text-muted-foreground">加载中…</div>
  const { training } = q.data

  return (
    <div className="mx-auto max-w-4xl px-6 py-5">
      <Header
        title={training.title}
        description={training.description}
        tags={training.tags}
        count={training.problemCount}
        kindLabel="训练"
        exportUrl={api.exportTrainingUrl(id)}
        onDelete={() => setConfirmDelete(true)}
        editMode={editMode}
        onToggleMode={() => setEditMode((v) => !v)}
        onEdit={() => setEditGroup(true)}
      />

      <p className="mb-3 text-xs text-muted-foreground">
        拖住 ⋮ 手柄移动题目（可跨章节）；拖住章节手柄调整章节顺序（拖动时章节自动折叠）。
      </p>

      {/* 章节列表：章节拖放落点在容器层统一处理 */}
      <div
        className="space-y-3"
        onDragEnd={endDrag}
        onDrop={(e) => {
          e.preventDefault()
          handleDropChapter()
        }}
      >
        {chapters.map((ch, i) => (
          <div key={ch.id}>
            {draggingChapter && chapterIndicator === i && <IndicatorLine />}
            <ChapterCard
              chapter={ch}
              index={i}
              collapsed={!!collapsed[ch.id]}
              forceCollapse={draggingChapter}
              drag={drag}
              itemIndicator={itemIndicator}
              checkedCount={checked.length}
              editMode={editMode}
              onToggleCollapse={() => setCollapsed((p) => ({ ...p, [ch.id]: !p[ch.id] }))}
              onChanged={invalidate}
              onAddChecked={() => addCheckedTo(ch.id)}
              onItemDragStart={(item, chapterId, e) => {
                e.dataTransfer.effectAllowed = 'move'
                e.dataTransfer.setData('text/plain', String(item.id))
                setDrag({ kind: 'item', itemId: item.id, fromChapterId: chapterId })
              }}
              onChapterDragStart={(chapterId, e) => {
                e.dataTransfer.effectAllowed = 'move'
                e.dataTransfer.setData('text/plain', String(chapterId))
                setDrag({ kind: 'chapter', chapterId })
              }}
              onItemDragOver={handleItemDragOver}
              onItemDrop={(targetChapterId, targetIndex, ev) => {
                ev.preventDefault()
                ev.stopPropagation()
                handleDropItem(targetChapterId, targetIndex)
              }}
              onChapterDragOver={handleChapterDragOver}
              onChapterBodyDragOver={(chapterId, count, e) => handleItemDragOver(chapterId, count, e)}
            />
          </div>
        ))}
        {draggingChapter && chapterIndicator === chapters.length && <IndicatorLine />}
        {chapters.length === 0 && <Empty>还没有章节，在下方创建第一个章节</Empty>}
      </div>

      {training.description && (
        <div className="mb-4 rounded-lg border bg-muted/30 px-4 py-3">
          <div className="mb-1 text-xs font-medium text-muted-foreground">描述</div>
          <p className="whitespace-pre-wrap text-sm">{training.description}</p>
        </div>
      )}

      {editMode && (
        <>
          <Separator className="my-4" />
          <div className="flex gap-2">
            <Input value={newChapter} onChange={(e) => setNewChapter(e.target.value)} placeholder="新章节名称" className="max-w-xs" onKeyDown={(e) => e.key === 'Enter' && addChapter()} />
            <Button variant="outline" onClick={addChapter} disabled={!newChapter.trim()}>
              <FolderPlusIcon data-icon="inline-start" /> 添加章节
            </Button>
          </div>
        </>
      )}

      <EditGroupDialog
        open={editGroup}
        onOpenChange={setEditGroup}
        kind="training"
        id={id}
        initialTitle={training.title}
        initialDescription={training.description}
        initialTags={training.tags}
        onSaved={invalidate}
      />

      <ConfirmDialog
        open={confirmDelete}
        onOpenChange={setConfirmDelete}
        title={`删除训练「${training.title}」？`}
        description="只删除编组结构，不会删除题目本身。"
        onConfirm={async () => {
          await api.deleteTraining(id)
          await qc.invalidateQueries()
          goHome()
        }}
      />
    </div>
  )
}

function IndicatorLine() {
  return <div className="my-0.5 h-0.5 rounded-full bg-primary" />
}

// 练习详情（平铺 + 分值）。
export function PracticeDetail({ id }: { id: number }) {
  const qc = useQueryClient()
  const { goHome } = useAppState()
  const q = useQuery({ queryKey: ['practice', id], queryFn: () => api.getPractice(id) })
  const [confirmDelete, setConfirmDelete] = useState(false)
  const [editMode, setEditMode] = useState(true)
  const [editGroup, setEditGroup] = useState(false)

  if (!q.data) return <div className="p-8 text-sm text-muted-foreground">加载中…</div>
  const { practice, items } = q.data

  async function invalidate() {
    await qc.invalidateQueries({ queryKey: ['practice', id] })
    await qc.invalidateQueries({ queryKey: ['practices'] })
  }

  async function updateItem(item: PracticeItem, score: number) {
    await api.updatePracticeItem(item.id, score)
    await invalidate()
  }
  async function removeItem(itemId: number) {
    await api.deletePracticeItem(itemId)
    await invalidate()
  }
  async function move(index: number, dir: -1 | 1) {
    const j = index + dir
    if (j < 0 || j >= items.length) return
    const ids = items.map((x) => x.id)
    ;[ids[index], ids[j]] = [ids[j], ids[index]]
    await api.reorderPracticeItems(id, ids)
    await invalidate()
  }

  return (
    <div className="mx-auto max-w-4xl px-6 py-5">
      <Header
        title={practice.title}
        description={practice.description}
        tags={practice.tags}
        count={practice.problemCount}
        kindLabel="练习"
        exportUrl={api.exportPracticeUrl(id)}
        onDelete={() => setConfirmDelete(true)}
        editMode={editMode}
        onToggleMode={() => setEditMode((v) => !v)}
        onEdit={() => setEditGroup(true)}
      />
      <div className="space-y-1.5">
        {items.map((it, i) => (
          <div key={it.id} className="flex items-center gap-2 rounded-lg border p-2.5">
            <span className="w-6 text-center text-xs text-muted-foreground">{i + 1}</span>
            <Badge variant="outline" className="text-[10px] text-muted-foreground">
              {it.problemType === 'programming' ? '编程' : it.problemType === 'single_choice' ? '单选' : it.problemType === 'true_false' ? '判断' : '?'}
            </Badge>
            <span className="min-w-0 flex-1 truncate text-sm">{it.problemTitle || `#${it.problemId}`}</span>
            <div className="flex w-24 items-center gap-1">
              {editMode ? (
                <Input
                  type="number"
                  defaultValue={it.score}
                  min={0}
                  className="h-7 text-xs"
                  onBlur={(e) => {
                    const v = Number(e.target.value)
                    if (v !== it.score) void updateItem(it, v)
                  }}
                />
              ) : (
                <span className="text-sm tabular-nums">{it.score} 分</span>
              )}
            </div>
            {editMode && (
              <>
                <Button size="icon-xs" variant="ghost" disabled={i === 0} onClick={() => move(i, -1)}>
                  <ArrowUpIcon />
                </Button>
                <Button size="icon-xs" variant="ghost" disabled={i === items.length - 1} onClick={() => move(i, 1)}>
                  <ArrowDownIcon />
                </Button>
                <Button size="icon-xs" variant="ghost" className="text-destructive" onClick={() => removeItem(it.id)}>
                  <TrashIcon />
                </Button>
              </>
            )}
          </div>
        ))}
        {items.length === 0 && <Empty>练习为空，去左侧勾选题目后通过「加入练习」添加</Empty>}
      </div>

      {practice.description && (
        <div className="mt-4 rounded-lg border bg-muted/30 px-4 py-3">
          <div className="mb-1 text-xs font-medium text-muted-foreground">描述</div>
          <p className="whitespace-pre-wrap text-sm">{practice.description}</p>
        </div>
      )}

      <EditGroupDialog
        open={editGroup}
        onOpenChange={setEditGroup}
        kind="practice"
        id={id}
        initialTitle={practice.title}
        initialDescription={practice.description}
        initialTags={practice.tags}
        onSaved={invalidate}
      />

      <ConfirmDialog
        open={confirmDelete}
        onOpenChange={setConfirmDelete}
        title={`删除练习「${practice.title}」？`}
        description="只删除编组结构，不会删除题目本身。"
        onConfirm={async () => {
          await api.deletePractice(id)
          await qc.invalidateQueries()
          goHome()
        }}
      />
    </div>
  )
}

// ---------- 共享头部 ----------

function Header(props: {
  title: string
  description?: string
  tags: string[]
  count: number
  kindLabel: string
  exportUrl: string
  onDelete: () => void
  editMode: boolean
  onToggleMode: () => void
  onEdit: () => void
}) {
  return (
    <div className="mb-4">
      <div className="mb-1 text-xs text-muted-foreground">{props.kindLabel}</div>
      <div className="flex items-start gap-2">
        <h1 className="min-w-0 flex-1 text-xl font-semibold">{props.title}</h1>
        <Button variant="ghost" size="icon-sm" title="编辑名称/描述" onClick={props.onEdit}>
          <PencilIcon />
        </Button>
        <a href={props.exportUrl}>
          <Button variant="outline" size="sm">
            <DownloadIcon data-icon="inline-start" /> 导出 OrangeOJ ZIP
          </Button>
        </a>
        <button
          type="button"
          onClick={props.onToggleMode}
          title={props.editMode ? '切换到显示模式' : '切换到编辑模式'}
          className={`inline-flex size-8 items-center justify-center rounded-lg border border-input text-sm transition-colors ${
            props.editMode ? 'bg-primary/10 text-primary' : 'hover:bg-muted'
          }`}
        >
          <EyeIcon className="size-4" />
        </button>
        {props.editMode && (
          <Button variant="ghost" size="sm" className="text-destructive" onClick={props.onDelete}>
            <TrashIcon />
          </Button>
        )}
      </div>
      <div className="mt-2 flex flex-wrap items-center gap-1.5">
        <Badge variant="outline">{props.count} 题</Badge>
        {props.tags.map((t) => (
          <Badge key={t} variant="secondary" className="text-xs">
            {t}
          </Badge>
        ))}
      </div>
    </div>
  )
}

// ---------- 章节卡片（含拖拽） ----------

function ChapterCard(props: {
  chapter: Chapter
  index: number
  collapsed: boolean
  forceCollapse: boolean
  drag: DragState | null
  itemIndicator: { chapterId: number; index: number } | null
  checkedCount: number
  editMode: boolean
  onToggleCollapse: () => void
  onChanged: () => void
  onAddChecked: () => void
  onItemDragStart: (item: Item, chapterId: number, e: DragEvent) => void
  onChapterDragStart: (chapterId: number, e: DragEvent) => void
  onItemDragOver: (chapterId: number, index: number, e: DragEvent) => void
  onItemDrop: (chapterId: number, index: number, e: DragEvent) => void
  onChapterDragOver: (index: number, e: DragEvent) => void
  onChapterBodyDragOver: (chapterId: number, count: number, e: DragEvent) => void
}) {
  const ch = props.chapter
  const [renaming, setRenaming] = useState(false)
  const [name, setName] = useState(ch.title)
  const showBody = !props.collapsed && !props.forceCollapse
  const itemDragActive = props.drag?.kind === 'item' && props.editMode
  const ind = itemDragActive ? props.itemIndicator : null
  const isDraggedChapter = props.drag?.kind === 'chapter' && props.drag.chapterId === ch.id

  async function saveName() {
    if (name.trim() && name.trim() !== ch.title) {
      await api.updateChapter(ch.id, name.trim(), ch.orderNo)
      props.onChanged()
    }
    setRenaming(false)
  }
  async function removeItem(item: Item) {
    await api.deleteItem(item.id)
    props.onChanged()
  }
  async function removeChapter() {
    await api.deleteChapter(ch.id)
    props.onChanged()
  }

  return (
    <div className={`rounded-xl border ${isDraggedChapter ? 'opacity-60' : ''}`}>
      {/* 章节头部：编辑模式显示拖拽/删除/重命名控件 */}
      <div
        className={`flex items-center gap-2 border-b bg-muted/50 px-2 py-2 ${
          props.editMode && props.drag?.kind === 'chapter' && !isDraggedChapter ? 'hover:bg-muted' : ''
        }`}
        onDragOver={props.editMode ? (e) =>
          props.onChapterDragOver(
            e.clientY > e.currentTarget.getBoundingClientRect().top + e.currentTarget.offsetHeight / 2 ? props.index + 1 : props.index,
            e,
          ) : undefined
        }
      >
        {props.editMode && (
          <span
            draggable
            onDragStart={(e) => props.onChapterDragStart(ch.id, e)}
            title="拖动调整章节顺序"
            className="flex shrink-0 cursor-grab items-center justify-center text-muted-foreground hover:text-foreground active:cursor-grabbing"
          >
            <GripVerticalIcon className="size-4" />
          </span>
        )}
        <button
          type="button"
          className="flex size-5 shrink-0 items-center justify-center"
          onClick={props.onToggleCollapse}
          title={props.collapsed ? '展开' : '折叠'}
        >
          <ChevronRightIcon className={`size-3.5 text-muted-foreground transition-transform ${showBody ? 'rotate-90' : ''}`} />
        </button>
        {renaming ? (
          <Input value={name} onChange={(e) => setName(e.target.value)} onBlur={saveName} onKeyDown={(e) => e.key === 'Enter' && saveName()} className="h-7 max-w-xs" autoFocus />
        ) : (
          <button type="button" className="min-w-0 flex-1 truncate text-left text-sm font-medium" onClick={() => props.editMode && setRenaming(true)} title={props.editMode ? '点击重命名' : undefined}>
            {ch.title}
          </button>
        )}
        <Badge variant="outline" className="shrink-0 text-[10px]">
          {ch.items.length} 题
        </Badge>
        {props.editMode && (
          <div className="ml-auto flex shrink-0 items-center gap-1">
            <Button size="xs" variant="outline" onClick={props.onAddChecked} disabled={props.checkedCount === 0}>
              <PlusIcon data-icon="inline-start" /> 加入勾选（{props.checkedCount}）
            </Button>
            <Button size="icon-xs" variant="ghost" className="text-destructive" onClick={removeChapter} title="删除章节">
              <TrashIcon />
            </Button>
          </div>
        )}
      </div>

      {/* 条目列表：拖动章节时整体隐藏 */}
      {showBody && (
        <div
          className="divide-y"
          onDragOver={(e) => props.onChapterBodyDragOver(ch.id, ch.items.length, e)}
          onDrop={(e) => props.onItemDrop(ch.id, ch.items.length, e)}
        >
          {ind?.chapterId === ch.id && ind.index === 0 && <IndicatorLine />}
          {ch.items.map((it, i) => (
            <div key={it.id}>
              {ind?.chapterId === ch.id && ind.index === i + 1 && <IndicatorLine />}
              <div
                draggable={props.editMode}
                onDragStart={props.editMode ? (e) => props.onItemDragStart(it, ch.id, e) : undefined}
                onDragOver={props.editMode ? (e) => props.onItemDragOver(ch.id, i, e) : undefined}
                onDrop={props.editMode ? (e) => props.onItemDrop(ch.id, i, e) : undefined}
                className={`group flex items-center gap-2 px-3 py-1.5 ${
                  props.editMode ? 'cursor-grab active:cursor-grabbing' : ''
                } ${
                  props.editMode && props.drag?.kind === 'item' && props.drag.itemId === it.id ? 'opacity-40' : 'hover:bg-muted/60'
                }`}
              >
                {props.editMode && <GripVerticalIcon className="size-3.5 shrink-0 text-muted-foreground/60" />}
                <span className="w-6 text-center text-xs text-muted-foreground">{i + 1}</span>
                <span className="min-w-0 flex-1 truncate text-sm">{it.problemTitle || `#${it.problemId}`}</span>
                {props.editMode && (
                  <Button
                    size="icon-xs"
                    variant="ghost"
                    className="opacity-0 transition-opacity group-hover:opacity-100"
                    onClick={() => removeItem(it)}
                    title="从章节移除"
                  >
                    <TrashIcon />
                  </Button>
                )}
              </div>
            </div>
          ))}
          {ch.items.length === 0 && (
            <div className={`px-3 py-2 text-xs text-muted-foreground ${itemDragActive ? 'bg-primary/5' : ''}`}>
              空章节：拖入题目或勾选后点「加入勾选」
            </div>
          )}
        </div>
      )}
    </div>
  )
}

// ---------- 编辑题册名称/描述对话框 ----------

function EditGroupDialog(props: {
  open: boolean
  onOpenChange: (v: boolean) => void
  kind: 'training' | 'practice'
  id: number
  initialTitle: string
  initialDescription: string
  initialTags: string[]
  onSaved: () => void
}) {
  const isTraining = props.kind === 'training'
  const [title, setTitle] = useState(props.initialTitle)
  const [description, setDescription] = useState(props.initialDescription)

  // 打开时同步预填值
  const [lastOpen, setLastOpen] = useState(false)
  if (props.open && !lastOpen) {
    setLastOpen(true)
    setTitle(props.initialTitle)
    setDescription(props.initialDescription)
  } else if (!props.open && lastOpen) {
    setLastOpen(false)
  }

  const save = useMutation({
    mutationFn: async () => {
      if (isTraining) {
        await api.updateTraining(props.id, { title, description, tags: props.initialTags })
      } else {
        await api.updatePractice(props.id, { title, description, tags: props.initialTags })
      }
    },
    onSuccess: async () => {
      toast.success('已保存')
      await props.onSaved()
      props.onOpenChange(false)
    },
    onError: (e) => toast.error(e instanceof Error ? e.message : '保存失败'),
  })

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>编辑{isTraining ? '训练' : '练习'}</DialogTitle>
        </DialogHeader>
        <div className="space-y-3">
          <div className="space-y-1.5">
            <Label>名称</Label>
            <Input value={title} onChange={(e) => setTitle(e.target.value)} />
          </div>
          <div className="space-y-1.5">
            <Label>描述</Label>
            <textarea
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder="可选描述"
              rows={4}
              className="flex w-full rounded-md border border-input bg-transparent px-3 py-2 text-sm placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/20"
            />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => props.onOpenChange(false)}>
            取消
          </Button>
          <Button onClick={() => save.mutate()} disabled={!title.trim() || save.isPending}>
            {save.isPending ? '保存中…' : '保存'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
