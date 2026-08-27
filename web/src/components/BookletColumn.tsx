import { useMemo, useState, type DragEvent } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import {
  CheckIcon,
  ChevronRightIcon,
  FileCodeIcon,
  FolderIcon,
  FolderPlusIcon,
  ListChecksIcon,
  MoreVerticalIcon,
  PencilIcon,
  PlusIcon,
  SearchIcon,
  TrashIcon,
  XIcon,
} from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { ScrollArea } from '@/components/ui/scroll-area'
import { api } from '@/lib/api'
import { useAppState } from '@/lib/app-context'
import { useMenuAnchorHold } from '@/lib/use-menu-anchor-hold'
import type { BookletDirectory } from '@/lib/types'
import { ConfirmDialog } from './dialogs'

type BookletItem = { id: number; type: 'training' | 'practice'; title: string; count: number; folderId: number | null }

interface FolderNode {
  dir: BookletDirectory
  children: FolderNode[]
}

// 拖拽负载：题册行 或 目录行
type DragState = { kind: 'booklet'; item: BookletItem } | { kind: 'folder'; node: FolderNode } | null

// 放置目标：目录行（三区语义：上下边缘=同级排序，中间=改为子目录）或根区域
type DropZone = { kind: 'folder'; id: number; mode: 'before' | 'after' | 'child' } | { kind: 'root' } | null

type FolderDropMode = 'before' | 'after' | 'child'

// 题册列：目录树（可嵌套、可拖拽改层级/顺序）+ 训练/练习混排，可筛选与拖拽移动，点击打开详情。
export function BookletColumn() {
  const { view, openTraining, openPractice } = useAppState()
  const qc = useQueryClient()
  const [search, setSearch] = useState('')
  const [expanded, setExpanded] = useState<Record<number, boolean>>({})
  const [activeFolderId, setActiveFolderId] = useState<number | null>(null)
  const [creating, setCreating] = useState<'training' | 'practice' | 'directory' | null>(null)
  const [renamingId, setRenamingId] = useState<number | null>(null)
  const [deletingDir, setDeletingDir] = useState<FolderNode | null>(null)
  const [drag, setDrag] = useState<DragState>(null)
  const [dropZone, setDropZone] = useState<DropZone>(null)

  const trainingsQ = useQuery({ queryKey: ['trainings'], queryFn: api.trainings })
  const practicesQ = useQuery({ queryKey: ['practices'], queryFn: api.practices })
  const dirsQ = useQuery({ queryKey: ['booklet-directories'], queryFn: api.bookletDirectories })
  const dirs = dirsQ.data?.directories ?? []

  const items = useMemo(() => {
    const all: BookletItem[] = []
    for (const t of trainingsQ.data?.trainings ?? [])
      all.push({ id: t.id, type: 'training', title: t.title, count: t.problemCount, folderId: t.folderId ?? null })
    for (const p of practicesQ.data?.practices ?? [])
      all.push({ id: p.id, type: 'practice', title: p.title, count: p.problemCount, folderId: p.folderId ?? null })
    return all
  }, [trainingsQ.data, practicesQ.data])

  const folderNodes = useMemo(() => buildFolderTree(dirs), [dirs])

  const q = search.trim().toLowerCase()
  const filtered = useMemo(() => {
    if (!q) return { folders: folderNodes, items }
    const match = (it: BookletItem) => it.title.toLowerCase().includes(q)
    const filterFolders = (nodes: FolderNode[]): FolderNode[] => {
      const out: FolderNode[] = []
      for (const n of nodes) {
        const children = filterFolders(n.children)
        const own = n.dir.name.toLowerCase().includes(q)
        const hasItems = items.some((it) => it.folderId === n.dir.id && match(it))
        if (own || children.length > 0 || hasItems) out.push({ ...n, children })
      }
      return out
    }
    return { folders: filterFolders(folderNodes), items: items.filter(match) }
  }, [q, folderNodes, items])

  const isLoading = trainingsQ.isLoading || practicesQ.isLoading || dirsQ.isLoading
  const activeDirName = activeFolderId != null ? folderNameOf(folderNodes, activeFolderId) : null
  const folderIds = useMemo(() => new Set(dirs.map((d) => d.id)), [dirs])

  function openItem(item: BookletItem) {
    if (item.type === 'training') openTraining(item.id)
    else openPractice(item.id)
  }

  async function invalidateBooklets() {
    await Promise.all([
      qc.invalidateQueries({ queryKey: ['trainings'] }),
      qc.invalidateQueries({ queryKey: ['practices'] }),
      qc.invalidateQueries({ queryKey: ['booklet-directories'] }),
    ])
  }

  function endDrag() {
    setDrag(null)
    setDropZone(null)
  }

  // ---------- 题册移动 ----------

  async function moveBooklet(it: BookletItem, folderId: number | null) {
    try {
      if (it.type === 'training') await api.setTrainingFolder(it.id, folderId)
      else await api.setPracticeFolder(it.id, folderId)
      await invalidateBooklets()
    } catch (e) {
      toast.error(e instanceof Error ? e.message : '移动失败')
    } finally {
      endDrag()
    }
  }

  // ---------- 目录移动（拖拽改层级/顺序） ----------

  /** targetId 是否是被拖目录自身或其子孙（禁止拖入自己的子树）。 */
  function isSelfOrDescendant(targetId: number, dragId: number): boolean {
    let cur: number | null = targetId
    const seen = new Set<number>()
    while (cur !== null) {
      if (seen.has(cur)) return true
      seen.add(cur)
      if (cur === dragId) return true
      const d = dirs.find((x) => x.id === cur)
      cur = d?.parentId ?? null
    }
    return false
  }

  /** 目录拖拽的目标校验：child 不能落入自身子树；before/after 仅限同父兄弟。 */
  function canDropFolderOn(targetId: number, mode: FolderDropMode): boolean {
    if (drag?.kind !== 'folder') return false
    const dragNode = drag.node.dir
    if (mode === 'child') return !isSelfOrDescendant(targetId, dragNode.id)
    if (targetId === dragNode.id) return false
    const dragParent = dirs.find((x) => x.id === dragNode.id)?.parentId ?? null
    const targetParent = dirs.find((x) => x.id === targetId)?.parentId ?? null
    return dragParent === targetParent
  }

  /** 由当前扁平目录重新装配布局：移除被拖目录 → 插入目标位 → DFS 展平赋序。 */
  function buildFolderLayout(dragId: number, target: { id: number; mode: FolderDropMode | 'root' }): BookletDirectory[] {
    const byParent = new Map<number | null, number[]>()
    const sorted = [...dirs].sort((a, b) => a.orderNo - b.orderNo || a.id - b.id)
    for (const d of sorted) {
      const p = d.parentId ?? null
      if (!byParent.has(p)) byParent.set(p, [])
      byParent.get(p)!.push(d.id)
    }
    const dragParent = dirs.find((x) => x.id === dragId)?.parentId ?? null
    byParent.set(dragParent, (byParent.get(dragParent) ?? []).filter((x) => x !== dragId))
    if (target.mode === 'root') {
      byParent.set(null, [...(byParent.get(null) ?? []), dragId])
    } else if (target.mode === 'child') {
      if (!byParent.has(target.id)) byParent.set(target.id, [])
      byParent.get(target.id)!.push(dragId)
    } else {
      const tp = dirs.find((x) => x.id === target.id)?.parentId ?? null
      const arr = [...(byParent.get(tp) ?? [])]
      const ti = arr.indexOf(target.id)
      arr.splice(target.mode === 'before' ? ti : ti + 1, 0, dragId)
      byParent.set(tp, arr)
    }
    const out: BookletDirectory[] = []
    const walk = (parentId: number | null) => {
      for (const id of byParent.get(parentId) ?? []) {
        out.push({ id, name: dirs.find((x) => x.id === id)?.name ?? '', parentId, orderNo: out.length + 1 })
        walk(id)
      }
    }
    walk(null)
    return out
  }

  async function commitFolderLayout(dragId: number, target: { id: number; mode: FolderDropMode }) {
    if (!canDropFolderOn(target.id, target.mode)) return endDrag()
    try {
      await api.setBookletDirectoryLayout(buildFolderLayout(dragId, target))
      await qc.invalidateQueries({ queryKey: ['booklet-directories'] })
    } catch (e) {
      toast.error(e instanceof Error ? e.message : '移动失败')
    } finally {
      endDrag()
    }
  }

  async function commitFolderToRoot(dragId: number) {
    try {
      await api.setBookletDirectoryLayout(buildFolderLayout(dragId, { id: -1, mode: 'root' }))
      await qc.invalidateQueries({ queryKey: ['booklet-directories'] })
    } catch (e) {
      toast.error(e instanceof Error ? e.message : '移动失败')
    } finally {
      endDrag()
    }
  }

  // ---------- 放置事件处理 ----------

  /** 目录行 dragover：题册=放进目录；目录=按三区语义（上/下边缘=排序，中间=子目录）。 */
  function handleFolderDragOver(id: number, mode: FolderDropMode, e: DragEvent) {
    if (!drag) return
    if (drag.kind === 'folder' && !canDropFolderOn(id, mode)) return
    e.preventDefault()
    e.stopPropagation()
    e.dataTransfer.dropEffect = 'move'
    setDropZone((prev) => (prev?.kind === 'folder' && prev.id === id && prev.mode === mode ? prev : { kind: 'folder', id, mode }))
  }

  function handleFolderDrop(id: number, mode: FolderDropMode, e: DragEvent) {
    e.preventDefault()
    e.stopPropagation()
    if (!drag) return endDrag()
    if (drag.kind === 'booklet') void moveBooklet(drag.item, id)
    else void commitFolderLayout(drag.node.dir.id, { id, mode })
  }

  function handleRootDragOver(e: DragEvent) {
    if (!drag) return
    e.preventDefault()
    e.dataTransfer.dropEffect = 'move'
    setDropZone((prev) => (prev?.kind === 'root' ? prev : { kind: 'root' }))
  }

  function handleRootDrop(e: DragEvent) {
    e.preventDefault()
    if (!drag) return endDrag()
    if (drag.kind === 'booklet') void moveBooklet(drag.item, null)
    else void commitFolderToRoot(drag.node.dir.id)
  }

  async function createItem(kind: 'training' | 'practice' | 'directory', name: string) {
    try {
      if (kind === 'directory') {
        await api.createBookletDirectory(name, activeFolderId)
        await qc.invalidateQueries({ queryKey: ['booklet-directories'] })
      } else if (kind === 'training') {
        const { id } = await api.createTraining(name, '', [], activeFolderId)
        await qc.invalidateQueries({ queryKey: ['trainings'] })
        openTraining(id)
      } else {
        const { id } = await api.createPractice(name, '', [], activeFolderId)
        await qc.invalidateQueries({ queryKey: ['practices'] })
        openPractice(id)
      }
      setCreating(null)
    } catch (e) {
      toast.error(e instanceof Error ? e.message : '创建失败')
    }
  }

  return (
    <div className="flex h-full flex-col bg-sidebar">
      <div className="px-3 pt-3 pb-2">
        {/* 标题行：题册 + 计数 +「+题册」下拉 */}
        <div className="mb-2 flex items-center gap-1.5">
          <span className="text-sm font-semibold">题册</span>
          <Badge variant="secondary" className="h-4 px-1.5 text-[10px] tabular-nums">
            {items.length}
          </Badge>
          {activeDirName && (
            <Badge variant="outline" className="h-4 gap-0.5 px-1.5 text-[10px] text-primary">
              <FolderIcon className="size-2.5" />
              {activeDirName}
              <button type="button" title="取消选中目录（新建将放入根目录）" onClick={() => setActiveFolderId(null)}>
                <XIcon className="size-2.5" />
              </button>
            </Badge>
          )}
          <DropdownMenu>
            <DropdownMenuTrigger
              className="ml-auto inline-flex h-7 items-center gap-1 rounded-lg border border-input bg-background px-2 text-[0.8rem] font-medium transition-colors hover:bg-muted"
              title="新建题册或目录"
            >
              <PlusIcon data-icon="inline-start" size={14} /> 题册
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem onClick={() => setCreating('training')}>
                <FileCodeIcon /> 新建训练
              </DropdownMenuItem>
              <DropdownMenuItem onClick={() => setCreating('practice')}>
                <ListChecksIcon /> 新建练习
              </DropdownMenuItem>
              <DropdownMenuItem onClick={() => setCreating('directory')}>
                <FolderPlusIcon /> 新建目录
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
        <div className="relative">
          <SearchIcon className="pointer-events-none absolute top-1/2 left-5 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="搜索题册名称…"
            className="pl-8"
          />
        </div>
      </div>

      <ScrollArea className="min-h-0 flex-1 px-2 pb-2">
        {isLoading && <div className="px-2 py-4 text-center text-xs text-muted-foreground">加载中…</div>}
        {!isLoading && filtered.items.length === 0 && filtered.folders.length === 0 && (
          <div className="px-2 py-4 text-center text-xs text-muted-foreground">暂无题册</div>
        )}
        <div className="space-y-0.5">
          {filtered.folders.map((node) => (
            <FolderRow
              key={node.dir.id}
              node={node}
              level={0}
              items={filtered.items}
              dirs={dirs}
              expanded={expanded}
              activeFolderId={activeFolderId}
              drag={drag}
              dropZone={dropZone}
              renamingId={renamingId}
              view={view}
              openItem={openItem}
              onToggleExpand={(id) => setExpanded((prev) => ({ ...prev, [id]: !(prev[id] ?? true) }))}
              onSelect={(id) => setActiveFolderId(id)}
              onRename={(id) => setRenamingId(id)}
              onRenameDone={async () => {
                setRenamingId(null)
                await qc.invalidateQueries({ queryKey: ['booklet-directories'] })
              }}
              onCreateSub={(id) => {
                setActiveFolderId(id)
                setCreating('directory')
              }}
              onDelete={setDeletingDir}
              onFolderDragStart={(node, e) => {
                e.dataTransfer.effectAllowed = 'move'
                e.dataTransfer.setData('text/plain', `folder:${node.dir.id}`)
                setDrag({ kind: 'folder', node })
              }}
              onFolderDragEnd={endDrag}
              onFolderDragOver={handleFolderDragOver}
              onFolderDrop={handleFolderDrop}
              onItemDragStart={(it, e) => {
                e.dataTransfer.effectAllowed = 'move'
                e.dataTransfer.setData('text/plain', `${it.type}:${it.id}`)
                setDrag({ kind: 'booklet', item: it })
              }}
              onItemDragEnd={endDrag}
              onItemDragOver={(e) => {
                if (drag) {
                  e.preventDefault()
                  e.stopPropagation()
                }
              }}
            />
          ))}
          {/* 根目录下的题册 */}
          {filtered.items
            .filter((it) => it.folderId === null || !folderIds.has(it.folderId))
            .map((it) => (
              <BookletRow
                key={`${it.type}-${it.id}`}
                item={it}
                level={0}
                active={view.kind === it.type && view.id === it.id}
                drag={drag}
                onClick={() => openItem(it)}
                onDragStart={(e) => {
                  e.dataTransfer.effectAllowed = 'move'
                  e.dataTransfer.setData('text/plain', `${it.type}:${it.id}`)
                  setDrag({ kind: 'booklet', item: it })
                }}
                onDragEnd={endDrag}
              />
            ))}
        </div>

        {/* 拖到此处移到根目录 / 顶层 */}
        {drag && (
          <div
            className={`mt-1 rounded-md border border-dashed px-2 py-1.5 text-center text-[10px] text-muted-foreground transition-colors ${
              dropZone?.kind === 'root' ? 'border-primary bg-primary/10 text-primary' : ''
            }`}
            onDragOver={handleRootDragOver}
            onDrop={handleRootDrop}
          >
            {drag.kind === 'folder' ? '拖到此处移到顶层' : '拖到此处移到根目录'}
          </div>
        )}
      </ScrollArea>

      {/* 新建对话框 */}
      <CreateDialog
        kind={creating}
        onOpenChange={(v) => !v && setCreating(null)}
        onSubmit={(name) => creating && void createItem(creating, name)}
      />

      {/* 删除目录确认 */}
      <ConfirmDialog
        open={deletingDir !== null}
        onOpenChange={(v) => !v && setDeletingDir(null)}
        title={`删除目录「${deletingDir?.dir.name ?? ''}」？`}
        description={getDescendantCount(deletingDir) > 0 ? '其子目录与归属题册将上移一层，题册本身不会被删除。' : '该目录下的题册将移到根目录，不会被删除。'}
        onConfirm={async () => {
          if (!deletingDir) return
          try {
            await api.deleteBookletDirectory(deletingDir.dir.id)
            if (activeFolderId === deletingDir.dir.id) setActiveFolderId(null)
            await invalidateBooklets()
            setDeletingDir(null)
          } catch (e) {
            toast.error(e instanceof Error ? e.message : '删除失败')
          }
        }}
      />
    </div>
  )
}

// ---------- 目录树装配 ----------

function buildFolderTree(dirs: BookletDirectory[]): FolderNode[] {
  const nodes = new Map<number, FolderNode>()
  for (const d of dirs) nodes.set(d.id, { dir: d, children: [] })
  const roots: FolderNode[] = []
  for (const d of dirs) {
    const n = nodes.get(d.id)!
    const parent = d.parentId != null ? nodes.get(d.parentId) : undefined
    if (parent) parent.children.push(n)
    else roots.push(n)
  }
  const sortNodes = (list: FolderNode[]) => {
    list.sort((a, b) => a.dir.orderNo - b.dir.orderNo || a.dir.name.localeCompare(b.dir.name, 'zh-Hans-CN'))
    for (const n of list) sortNodes(n.children)
  }
  sortNodes(roots)
  return roots
}

function folderNameOf(nodes: FolderNode[], id: number): string | null {
  for (const n of nodes) {
    if (n.dir.id === id) return n.dir.name
    const hit = folderNameOf(n.children, id)
    if (hit) return hit
  }
  return null
}

// 直接子目录数量（用于删除确认文案）
function getDescendantCount(node: FolderNode | null): number {
  if (!node) return 0
  return node.children.reduce((acc, c) => acc + 1 + getDescendantCount(c), 0)
}

function countInSubtree(node: FolderNode, items: BookletItem[]): number {
  const own = items.filter((it) => it.folderId === node.dir.id).length
  return own + node.children.reduce((acc, c) => acc + countInSubtree(c, items), 0)
}

// ---------- 目录行 ----------

function FolderRow(props: {
  node: FolderNode
  level: number
  items: BookletItem[]
  dirs: BookletDirectory[]
  expanded: Record<number, boolean>
  activeFolderId: number | null
  drag: DragState
  dropZone: DropZone
  renamingId: number | null
  view: ReturnType<typeof useAppState>['view']
  openItem: (it: BookletItem) => void
  onToggleExpand: (id: number) => void
  onSelect: (id: number) => void
  onRename: (id: number) => void
  onRenameDone: () => void
  onCreateSub: (id: number) => void
  onDelete: (node: FolderNode) => void
  onFolderDragStart: (node: FolderNode, e: DragEvent) => void
  onFolderDragEnd: () => void
  onFolderDragOver: (id: number, mode: FolderDropMode, e: DragEvent) => void
  onFolderDrop: (id: number, mode: FolderDropMode, e: DragEvent) => void
  onItemDragStart: (it: BookletItem, e: DragEvent) => void
  onItemDragEnd: () => void
  onItemDragOver: (e: DragEvent) => void
}) {
  const { node, level } = props
  const open = props.expanded[node.dir.id] ?? true
  // 菜单锚点保持：打开期间及关闭动画期间强制可见，防止弹层闪现左上角
  const [menuOpen, setMenuOpen, anchorVisible] = useMenuAnchorHold()
  const [renameValue, setRenameValue] = useState(node.dir.name)
  const isActive = props.activeFolderId === node.dir.id
  const isDropTarget = props.dropZone?.kind === 'folder' && props.dropZone.id === node.dir.id
  const dropMode = props.dropZone?.kind === 'folder' && props.dropZone.id === node.dir.id ? props.dropZone.mode : null
  const own = props.items.filter((it) => it.folderId === node.dir.id)
  const bookletCount = own.length + node.children.reduce((acc, c) => acc + countInSubtree(c, props.items), 0)
  const isDraggedFolder = props.drag?.kind === 'folder' && props.drag.node.dir.id === node.dir.id

  /** 三区语义：上/下边缘=同级排序插入位，中间=改为子目录。 */
  function dropModeOf(e: DragEvent): FolderDropMode {
    const r = e.currentTarget.getBoundingClientRect()
    const y = (e.clientY - r.top) / Math.max(r.height, 1)
    return y < 0.28 ? 'before' : y > 0.72 ? 'after' : 'child'
  }

  async function submitRename() {
    const name = renameValue.trim()
    if (!name || name === node.dir.name) {
      props.onRenameDone()
      return
    }
    try {
      await api.renameBookletDirectory(node.dir.id, name)
      props.onRenameDone()
    } catch (e) {
      toast.error(e instanceof Error ? e.message : '重命名失败')
    }
  }

  return (
    <div>
      <div
        className={`group flex items-center rounded-md pr-1 ${
          isDraggedFolder ? 'opacity-40' : ''
        } ${
          isDropTarget
            ? dropMode === 'child'
              ? 'bg-primary/10 ring-1 ring-primary'
              : dropMode === 'before'
                ? 'rounded-md shadow-[0_-2px_0_0_var(--primary)]'
                : 'rounded-md shadow-[0_2px_0_0_var(--primary)]'
            : isActive
              ? 'bg-accent/60'
              : 'hover:bg-muted'
        }`}
        style={{ paddingLeft: `${level * 14}px` }}
      >
        <button
          type="button"
          className="flex size-5 shrink-0 items-center justify-center"
          onClick={() => props.onToggleExpand(node.dir.id)}
          title={open ? '折叠' : '展开'}
        >
          <ChevronRightIcon className={`size-3.5 text-muted-foreground transition-transform ${open ? 'rotate-90' : ''}`} />
        </button>
        <div className="min-w-0 flex-1">
          {props.renamingId === node.dir.id ? (
            <Input
              value={renameValue}
              onChange={(e) => setRenameValue(e.target.value)}
              onBlur={submitRename}
              onKeyDown={(e) => {
                if (e.key === 'Enter') void submitRename()
                if (e.key === 'Escape') props.onRenameDone()
              }}
              className="h-6 w-full text-xs"
              autoFocus
            />
          ) : (
            <button
              type="button"
              draggable
              onDragStart={(e) => props.onFolderDragStart(node, e)}
              onDragEnd={props.onFolderDragEnd}
              onDragOver={(e) => {
                if (!props.drag) return
                // 题册拖拽：整行=放进目录；目录拖拽：三区语义（注意在事件处理期读取 currentTarget）
                props.onFolderDragOver(node.dir.id, props.drag.kind === 'booklet' ? 'child' : dropModeOf(e), e)
              }}
              onDrop={(e) => {
                e.preventDefault()
                e.stopPropagation()
                if (!props.drag) return
                props.onFolderDrop(node.dir.id, props.drag.kind === 'booklet' ? 'child' : dropModeOf(e), e)
              }}
              onClick={() => {
                props.onToggleExpand(node.dir.id)
                props.onSelect(node.dir.id)
              }}
              className={`flex min-w-0 flex-1 items-center gap-1.5 rounded-md py-1.5 text-left ${
                props.drag?.kind === 'folder' ? 'cursor-grab active:cursor-grabbing' : ''
              }`}
              title={`${node.dir.name}（点击展开/收起并选中；拖动可改变层级与顺序；拖入题册可移动到此目录）`}
            >
              <FolderIcon className={`size-3.5 shrink-0 ${isActive ? 'text-primary' : 'text-primary/70'}`} />
              <span className={`flex-1 truncate font-medium ${isActive ? 'text-primary' : ''}`}>{node.dir.name}</span>
              <span className="shrink-0 text-xs tabular-nums text-muted-foreground">
                {bookletCount > 0 ? bookletCount : ''}
              </span>
            </button>
          )}
        </div>
        <div className={`shrink-0 items-center gap-0.5 ${anchorVisible ? 'flex' : 'hidden group-hover:flex'}`}>
          <DropdownMenu open={menuOpen} onOpenChange={setMenuOpen}>
            <DropdownMenuTrigger className="rounded p-1 hover:bg-background" title="目录操作" onClick={(e) => e.stopPropagation()}>
              <MoreVerticalIcon className="size-3.5" />
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem onClick={() => { props.onCreateSub(node.dir.id); setMenuOpen(false) }}>
                <FolderPlusIcon className="size-3.5" /> 新建子目录
              </DropdownMenuItem>
              <DropdownMenuItem onClick={() => { setRenameValue(node.dir.name); props.onRename(node.dir.id); setMenuOpen(false) }}>
                <PencilIcon className="size-3.5" /> 重命名
              </DropdownMenuItem>
              <DropdownMenuItem variant="destructive" onClick={() => { props.onDelete(node); setMenuOpen(false) }}>
                <TrashIcon className="size-3.5" /> 删除
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </div>

      {open && (
        <div>
          {node.children.map((c) => (
            <FolderRow
              key={c.dir.id}
              node={c}
              level={level + 1}
              items={props.items}
              dirs={props.dirs}
              expanded={props.expanded}
              activeFolderId={props.activeFolderId}
              drag={props.drag}
              dropZone={props.dropZone}
              renamingId={props.renamingId}
              view={props.view}
              openItem={props.openItem}
              onToggleExpand={props.onToggleExpand}
              onSelect={props.onSelect}
              onRename={props.onRename}
              onRenameDone={props.onRenameDone}
              onCreateSub={props.onCreateSub}
              onDelete={props.onDelete}
              onFolderDragStart={props.onFolderDragStart}
              onFolderDragEnd={props.onFolderDragEnd}
              onFolderDragOver={props.onFolderDragOver}
              onFolderDrop={props.onFolderDrop}
              onItemDragStart={props.onItemDragStart}
              onItemDragEnd={props.onItemDragEnd}
              onItemDragOver={props.onItemDragOver}
            />
          ))}
          {own.map((it) => (
            <BookletRow
              key={`${it.type}-${it.id}`}
              item={it}
              level={level + 1}
              active={props.view.kind === it.type && props.view.id === it.id}
              drag={props.drag}
              onClick={() => props.openItem(it)}
              onDragStart={(e) => props.onItemDragStart(it, e)}
              onDragEnd={props.onItemDragEnd}
              onDragOver={props.onItemDragOver}
            />
          ))}
        </div>
      )}
    </div>
  )
}

// ---------- 题册行 ----------

function BookletRow(props: {
  item: BookletItem
  level: number
  active: boolean
  drag: DragState
  onClick: () => void
  onDragStart: (e: DragEvent) => void
  onDragEnd: () => void
  onDragOver?: (e: DragEvent) => void
}) {
  const { item } = props
  const isDragged = props.drag?.kind === 'booklet' && props.drag.item.type === item.type && props.drag.item.id === item.id
  return (
    <div className="flex items-center" style={{ paddingLeft: `${props.level * 14}px` }}>
      <button
        type="button"
        draggable
        onDragStart={props.onDragStart}
        onDragEnd={props.onDragEnd}
        onDragOver={props.onDragOver}
        onClick={props.onClick}
        className={`group flex w-full items-start gap-2 rounded-md px-2 py-2 text-left transition-colors ${
          props.active ? 'bg-accent text-accent-foreground' : 'hover:bg-muted'
        } ${isDragged ? 'opacity-40' : ''}`}
        title="点击打开；可拖入目录或根区域"
      >
        {item.type === 'training' ? (
          <FileCodeIcon className="mt-0.5 size-3.5 shrink-0 text-primary/70" />
        ) : (
          <ListChecksIcon className="mt-0.5 size-3.5 shrink-0 text-primary/70" />
        )}
        <div className="min-w-0 flex-1">
          <div className="truncate text-sm">{item.title}</div>
          <div className="mt-0.5 flex items-center gap-1.5">
            <Badge variant="outline" className="h-3.5 px-1 text-[9px]">
              {item.type === 'training' ? '训练' : '练习'}
            </Badge>
            <span className="text-[10px] text-muted-foreground">{item.count} 题</span>
          </div>
        </div>
      </button>
    </div>
  )
}

// ---------- 新建题册/目录对话框 ----------

function CreateDialog(props: {
  kind: 'training' | 'practice' | 'directory' | null
  onOpenChange: (v: boolean) => void
  onSubmit: (name: string) => void
}) {
  const [name, setName] = useState('')
  const labels: Record<'training' | 'practice' | 'directory', string> = {
    training: '训练',
    practice: '练习',
    directory: '目录',
  }
  const title = props.kind ? `新建${labels[props.kind]}` : ''
  return (
    <Dialog open={props.kind !== null} onOpenChange={props.onOpenChange}>
      <DialogContent className="sm:max-w-xs">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
        </DialogHeader>
        <div className="space-y-1.5">
          <Label>{props.kind === 'directory' ? '目录名称（可继续创建子目录）' : '名称'}</Label>
          <Input
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder={props.kind === 'directory' ? '例如：竞赛真题' : '例如：期中复习'}
            autoFocus
            onKeyDown={(e) => {
              if (e.key === 'Enter' && name.trim()) {
                props.onSubmit(name.trim())
                setName('')
              }
            }}
          />
        </div>
        <DialogFooter>
          <Button
            variant="outline"
            onClick={() => {
              props.onOpenChange(false)
              setName('')
            }}
          >
            取消
          </Button>
          <Button
            onClick={() => {
              if (!name.trim()) return
              props.onSubmit(name.trim())
              setName('')
            }}
            disabled={!name.trim()}
          >
            <CheckIcon data-icon="inline-start" /> 创建
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}