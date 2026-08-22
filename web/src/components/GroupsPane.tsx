import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import {
  ArrowDownIcon,
  ArrowUpIcon,
  DownloadIcon,
  FolderPlusIcon,
  PlusIcon,
  TrashIcon,
} from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Separator } from '@/components/ui/separator'
import { api } from '@/lib/api'
import { useAppState } from '@/lib/app-context'
import type { Chapter, Item, PracticeItem } from '@/lib/types'
import { ConfirmDialog } from './dialogs'
import { Empty } from './ProblemPane'

// 训练详情（章节化）。
export function TrainingDetail({ id }: { id: number }) {
  const qc = useQueryClient()
  const { checked, goHome } = useAppState()
  const q = useQuery({ queryKey: ['training', id], queryFn: () => api.getTraining(id) })
  const [newChapter, setNewChapter] = useState('')
  const [confirmDelete, setConfirmDelete] = useState(false)

  async function invalidate() {
    await qc.invalidateQueries({ queryKey: ['training', id] })
    await qc.invalidateQueries({ queryKey: ['trainings'] })
  }

  if (!q.data) return <div className="p-8 text-sm text-muted-foreground">加载中…</div>
  const { training, chapters } = q.data

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
      />

      {/* 章节列表 */}
      <div className="space-y-4">
        {chapters.map((ch) => (
          <ChapterCard key={ch.id} chapter={ch} onChanged={invalidate} onAddChecked={() => addCheckedTo(ch.id)} checkedCount={checked.length} />
        ))}
        {chapters.length === 0 && <Empty>还没有章节，在下方创建第一个章节</Empty>}
      </div>

      <Separator className="my-4" />
      <div className="flex gap-2">
        <Input value={newChapter} onChange={(e) => setNewChapter(e.target.value)} placeholder="新章节名称" className="max-w-xs" onKeyDown={(e) => e.key === 'Enter' && addChapter()} />
        <Button variant="outline" onClick={addChapter} disabled={!newChapter.trim()}>
          <FolderPlusIcon data-icon="inline-start" /> 添加章节
        </Button>
      </div>

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

// 练习详情（平铺 + 分值）。
export function PracticeDetail({ id }: { id: number }) {
  const qc = useQueryClient()
  const { goHome } = useAppState()
  const q = useQuery({ queryKey: ['practice', id], queryFn: () => api.getPractice(id) })
  const [confirmDelete, setConfirmDelete] = useState(false)

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
              <span className="text-xs text-muted-foreground">分</span>
            </div>
            <Button size="icon-xs" variant="ghost" disabled={i === 0} onClick={() => move(i, -1)}>
              <ArrowUpIcon />
            </Button>
            <Button size="icon-xs" variant="ghost" disabled={i === items.length - 1} onClick={() => move(i, 1)}>
              <ArrowDownIcon />
            </Button>
            <Button size="icon-xs" variant="ghost" className="text-destructive" onClick={() => removeItem(it.id)}>
              <TrashIcon />
            </Button>
          </div>
        ))}
        {items.length === 0 && <Empty>练习为空，去左侧勾选题目后通过「加入练习」添加</Empty>}
      </div>

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
}) {
  return (
    <div className="mb-4">
      <div className="mb-1 text-xs text-muted-foreground">{props.kindLabel}</div>
      <div className="flex items-start gap-2">
        <h1 className="min-w-0 flex-1 text-xl font-semibold">{props.title}</h1>
        <a href={props.exportUrl}>
          <Button variant="outline" size="sm">
            <DownloadIcon data-icon="inline-start" /> 导出 OrangeOJ ZIP
          </Button>
        </a>
        <Button variant="ghost" size="sm" className="text-destructive" onClick={props.onDelete}>
          <TrashIcon />
        </Button>
      </div>
      <div className="mt-2 flex flex-wrap items-center gap-1.5">
        <Badge variant="outline">{props.count} 题</Badge>
        {props.description && <span className="text-xs text-muted-foreground">{props.description}</span>}
        {props.tags.map((t) => (
          <Badge key={t} variant="secondary" className="text-xs">
            {t}
          </Badge>
        ))}
      </div>
    </div>
  )
}

// ---------- 章节卡片 ----------

function ChapterCard(props: { chapter: Chapter; onChanged: () => void; onAddChecked: () => void; checkedCount: number }) {
  const { ch } = { ch: props.chapter }
  const [renaming, setRenaming] = useState(false)
  const [name, setName] = useState(ch.title)

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
  async function moveItem(index: number, dir: -1 | 1) {
    const items = ch.items
    const j = index + dir
    if (j < 0 || j >= items.length) return
    const ids = items.map((x) => x.id)
    ;[ids[index], ids[j]] = [ids[j], ids[index]]
    await api.reorderChapterItems(ch.id, ids)
    props.onChanged()
  }
  async function removeChapter() {
    await api.deleteChapter(ch.id)
    props.onChanged()
  }

  return (
    <div className="rounded-xl border">
      <div className="flex items-center gap-2 border-b bg-muted/50 px-3 py-2">
        {renaming ? (
          <Input value={name} onChange={(e) => setName(e.target.value)} onBlur={saveName} onKeyDown={(e) => e.key === 'Enter' && saveName()} className="h-7 max-w-xs" autoFocus />
        ) : (
          <button type="button" className="text-sm font-medium hover:underline" onClick={() => setRenaming(true)} title="点击重命名">
            {ch.title}
          </button>
        )}
        <Badge variant="outline" className="text-[10px]">
          {ch.items.length} 题
        </Badge>
        <div className="ml-auto flex items-center gap-1">
          <Button size="xs" variant="outline" onClick={props.onAddChecked} disabled={props.checkedCount === 0}>
            <PlusIcon data-icon="inline-start" /> 加入勾选（{props.checkedCount}）
          </Button>
          <Button size="icon-xs" variant="ghost" className="text-destructive" onClick={removeChapter} title="删除章节">
            <TrashIcon />
          </Button>
        </div>
      </div>
      <div className="divide-y">
        {ch.items.map((it, i) => (
          <div key={it.id} className="flex items-center gap-2 px-3 py-1.5">
            <span className="w-6 text-center text-xs text-muted-foreground">{i + 1}</span>
            <span className="min-w-0 flex-1 truncate text-sm">{it.problemTitle || `#${it.problemId}`}</span>
            <Button size="icon-xs" variant="ghost" disabled={i === 0} onClick={() => moveItem(i, -1)}>
              <ArrowUpIcon />
            </Button>
            <Button size="icon-xs" variant="ghost" disabled={i === ch.items.length - 1} onClick={() => moveItem(i, 1)}>
              <ArrowDownIcon />
            </Button>
            <Button size="icon-xs" variant="ghost" className="text-destructive" onClick={() => removeItem(it)}>
              <TrashIcon />
            </Button>
          </div>
        ))}
        {ch.items.length === 0 && <div className="px-3 py-2 text-xs text-muted-foreground">空章节，勾选题目后点「加入勾选」</div>}
      </div>
    </div>
  )
}
