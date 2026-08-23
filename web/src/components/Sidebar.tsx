import { useMemo, useState, type DragEvent } from 'react'
import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import {
  ChevronRightIcon,
  ChevronsDownUpIcon,
  ChevronsUpDownIcon,
  DownloadIcon,
  FileCodeIcon,
  ListChecksIcon,
  LogOutIcon,
  MoreVerticalIcon,
  PencilIcon,
  PlusIcon,
  SearchIcon,
  SettingsIcon,
  TagIcon,
  TrashIcon,
  UploadIcon,
} from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { ScrollArea } from '@/components/ui/scroll-area'
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
import { api } from '@/lib/api'
import { useAppState } from '@/lib/app-context'
import type { ProblemSummary, ProblemType, Practice, TagCount, TagNode, Training } from '@/lib/types'
import { AddToGroupDialog, ConfirmDialog, ImportDialog, NewProblemDialog } from './dialogs'

const TYPE_LABEL: Record<ProblemType, string> = { programming: '编程', single_choice: '单选', true_false: '判断' }

// 第一栏：标签筛选（搜索 / 类型 / 标签树）。
export function TagFilterColumn({ onLogout, onOpenSettings }: { onLogout: () => void; onOpenSettings: () => void }) {
  const { filter, patchFilter } = useAppState()

  return (
    <div className="flex h-full flex-col bg-sidebar">
      {/* 头部 */}
      <div className="flex items-center gap-2 px-3 pt-3">
        <span className="flex size-8 items-center justify-center rounded-lg bg-primary text-lg text-primary-foreground">🍊</span>
        <div className="min-w-0 flex-1 leading-tight">
          <div className="truncate text-sm font-semibold">OrangeRepo</div>
          <div className="text-[10px] text-muted-foreground">标签筛选</div>
        </div>
        <Button variant="ghost" size="icon-sm" title="修改密码" onClick={onOpenSettings}>
          <SettingsIcon />
        </Button>
        <Button variant="outline" size="sm" title="退出登录" onClick={onLogout}>
          <LogOutIcon data-icon="inline-start" />
          退出
        </Button>
      </div>

      {/* 搜索 */}
      <div className="relative px-3 pt-3">
        <SearchIcon className="pointer-events-none absolute top-1/2 left-5 size-4 -translate-y-1/2 text-muted-foreground" />
        <Input
          value={filter.q}
          onChange={(e) => patchFilter({ q: e.target.value })}
          placeholder="搜索标题 / 标签…"
          className="pl-8"
        />
      </div>

      {/* 类型过滤 */}
      <div className="flex gap-1 px-3 pt-2">
        {(
          [
            ['', '全部'],
            ['programming', '编程'],
            ['single_choice', '单选'],
            ['true_false', '判断'],
          ] as [ProblemType | '', string][]
        ).map(([v, label]) => (
          <button
            key={label}
            type="button"
            onClick={() => patchFilter({ type: v })}
            className={`rounded-md px-2 py-1 text-xs transition-colors ${
              filter.type === v ? 'bg-primary text-primary-foreground' : 'text-muted-foreground hover:bg-muted'
            }`}
          >
            {label}
          </button>
        ))}
      </div>

      {/* 标签树 */}
      <ScrollArea className="min-h-0 flex-1">
        <TagTreePanel />
      </ScrollArea>
    </div>
  )
}

// 第二栏：题目查看（操作 / 批量 / 题目列表 / 训练练习）。
export function ProblemListColumn() {
  const { checked, clearChecked } = useAppState()
  const [newProblem, setNewProblem] = useState(false)
  const [importOpen, setImportOpen] = useState(false)
  const [addToGroup, setAddToGroup] = useState<'training' | 'practice' | null>(null)

  return (
    <div className="flex h-full flex-col bg-background">
      {/* 操作区 */}
      <div className="flex items-center gap-1.5 px-3 pt-3">
        <Button size="sm" className="flex-1" onClick={() => setNewProblem(true)}>
          <PlusIcon data-icon="inline-start" /> 题目
        </Button>
        <DropdownMenu>
          <DropdownMenuTrigger
            className="inline-flex size-8 items-center justify-center rounded-lg border border-input bg-transparent text-sm transition-colors hover:bg-muted"
            title="导入 ZIP"
          >
            <UploadIcon className="size-4" />
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start">
            <DropdownMenuItem onClick={() => setImportOpen(true)}>导入 OrangeOJ ZIP…</DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
        <ExportDropdown />
      </div>

      {/* 题目列表 */}
      <ScrollArea className="min-h-0 flex-1">
        <ProblemList />
      </ScrollArea>

      {/* 批量操作条 */}
      {checked.length > 0 && (
        <div className="flex items-center gap-1 border-t bg-background p-2 shadow-sm">
          <Badge variant="secondary" className="mr-auto">
            已选 {checked.length}
          </Badge>
          <Button size="xs" variant="outline" onClick={() => setAddToGroup('training')}>
            加入训练
          </Button>
          <Button size="xs" variant="outline" onClick={() => setAddToGroup('practice')}>
            加入练习
          </Button>
          <Button size="xs" variant="ghost" onClick={clearChecked}>
            清空
          </Button>
        </div>
      )}

      {/* 训练 / 练习 分区（可折叠） */}
      <GroupSection kind="training" />
      <GroupSection kind="practice" />

      {/* 对话框 */}
      <NewProblemDialog open={newProblem} onOpenChange={setNewProblem} />
      <ImportDialog open={importOpen} onOpenChange={setImportOpen} />
      <AddToGroupDialog
        open={addToGroup !== null}
        onOpenChange={(v) => !v && setAddToGroup(null)}
        kind={addToGroup ?? 'training'}
        problemIds={checked}
      />
    </div>
  )
}

// ---------- 导出下拉 ----------

function ExportDropdown() {
  const { filter, checked } = useAppState()
  function exportFiltered() {
    window.open(api.exportProblemsUrl(filter))
  }
  function exportChecked() {
    window.open(api.exportProblemsUrl(filter, checked))
  }
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        className="inline-flex size-8 items-center justify-center rounded-lg border border-input bg-transparent text-sm transition-colors hover:bg-muted"
        title="导出"
      >
        <DownloadIcon className="size-4" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuItem onClick={exportFiltered}>导出当前筛选结果</DropdownMenuItem>
        <DropdownMenuItem onClick={exportChecked} disabled={checked.length === 0}>
          导出选中题目（{checked.length}）
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

// ---------- 标签树（多选 AND 前缀筛选 · 动态 facet 计数 · 子树重命名/删除） ----------

const TAG_SEARCH_THRESHOLD = 20

/** 由分面平面列表构建标签树；虚拟祖先节点计数为 0 时由服务端直接给出。 */
function buildTagTree(items: TagCount[]): TagNode[] {
  const nodes = new Map<string, TagNode>()
  const ensure = (tag: string): TagNode => {
    let n = nodes.get(tag)
    if (!n) {
      n = { tag, label: tag.slice(tag.lastIndexOf('/') + 1), count: 0, children: [] }
      nodes.set(tag, n)
    }
    return n
  }
  for (const it of items) {
    ensure(it.tag).count = it.count
    const parts = it.tag.split('/')
    for (let i = 1; i < parts.length; i++) ensure(parts.slice(0, i).join('/'))
  }
  const roots: TagNode[] = []
  for (const n of nodes.values()) {
    const idx = n.tag.lastIndexOf('/')
    if (idx === -1) roots.push(n)
    else nodes.get(n.tag.slice(0, idx))!.children.push(n)
  }
  return roots
}

function sortTree(nodes: TagNode[], sortBy: 'count' | 'name') {
  for (const n of nodes) sortTree(n.children, sortBy)
  nodes.sort((x, y) =>
    sortBy === 'name'
      ? x.label.localeCompare(y.label, 'zh-Hans-CN')
      : y.count - x.count || x.label.localeCompare(y.label, 'zh-Hans-CN'),
  )
}

/** 树内查找：保留命中节点及其祖先链以维持树形结构。 */
function filterTree(nodes: TagNode[], q: string): TagNode[] {
  const out: TagNode[] = []
  for (const n of nodes) {
    const children = filterTree(n.children, q)
    if (n.tag.toLowerCase().includes(q) || children.length > 0) {
      out.push({ ...n, children })
    }
  }
  return out
}

function collectPaths(nodes: TagNode[], out: string[] = []): string[] {
  for (const n of nodes) {
    out.push(n.tag)
    collectPaths(n.children, out)
  }
  return out
}

function subtreeSize(node: TagNode): number {
  return 1 + node.children.reduce((acc, c) => acc + subtreeSize(c), 0)
}

/** 该节点是否已被某个严格祖先标签隐含（前缀规则下点不点都命中）。 */
function impliedBySelected(tag: string, selected: string[]): boolean {
  const parts = tag.split('/')
  for (let i = 1; i < parts.length; i++) {
    if (selected.includes(parts.slice(0, i).join('/'))) return true
  }
  return false
}

function TagTreePanel() {
  const { filter, patchFilter, view, goHome } = useAppState()
  const qc = useQueryClient()
  const [search, setSearch] = useState('')
  const [sortBy, setSortBy] = useState<'count' | 'name'>('count')
  const [expanded, setExpanded] = useState<Record<string, boolean>>({})
  const [renaming, setRenaming] = useState<string | null>(null)
  const [deleting, setDeleting] = useState<TagNode | null>(null)
  // 标签拖拽改层级：拖起节点放到目标节点下成为其子标签（复用重命名接口联动题目）
  const [tagDragFrom, setTagDragFrom] = useState<string | null>(null)
  const [tagDropTarget, setTagDropTarget] = useState<string | null>(null)

  /** 目标是否可放置：不能是自身、也不能落进自己的子树（否则会连带改写目标路径）。 */
  function canDropTag(target: string, from: string): boolean {
    if (!from) return false
    const to = `${target}/${from.slice(from.lastIndexOf('/') + 1)}`
    return to !== from && !to.startsWith(from + '/')
  }

  function handleTagDragOver(target: string, e: DragEvent) {
    if (!tagDragFrom) return
    e.stopPropagation() // 可否放置都阻断冒泡：无效目标不触发根区“移到顶层”
    if (!canDropTag(target, tagDragFrom)) return
    e.preventDefault()
    e.dataTransfer.dropEffect = 'move'
    setTagDropTarget((prev) => (prev === target ? prev : target))
  }

  function handleTagDrop(target: string) {
    if (!tagDragFrom || !canDropTag(target, tagDragFrom)) return endTagDrag()
    const to = `${target}/${tagDragFrom.slice(tagDragFrom.lastIndexOf('/') + 1)}`
    renameMut.mutate({ from: tagDragFrom, to })
  }

  function endTagDrag() {
    setTagDragFrom(null)
    setTagDropTarget(null)
  }

  const tagsQuery = useQuery({
    queryKey: ['tags', filter],
    queryFn: () => api.tags(filter),
    placeholderData: keepPreviousData,
  })
  const raw = tagsQuery.data?.tags ?? []
  const total = tagsQuery.data?.total

  const tree = useMemo(() => {
    const roots = buildTagTree(raw)
    sortTree(roots, sortBy)
    const q = search.trim().toLowerCase()
    return q ? filterTree(roots, q) : roots
  }, [raw, sortBy, search])

  const allPaths = useMemo(() => collectPaths(tree), [tree])
  const isOpen = (tag: string) => expanded[tag] ?? !tag.includes('/')

  function toggleSelect(tag: string) {
    patchFilter({
      tags: filter.tags.includes(tag) ? filter.tags.filter((x) => x !== tag) : [...filter.tags, tag],
    })
    if (view.kind !== 'empty') goHome()
  }

  async function invalidateAfterTagChange() {
    await Promise.all([
      qc.invalidateQueries({ queryKey: ['tags'] }),
      qc.invalidateQueries({ queryKey: ['problems'] }),
    ])
  }

  const renameMut = useMutation({
    mutationFn: ({ from, to }: { from: string; to: string }) => api.renameTag(from, to),
    onSuccess: async (_d, v) => {
      toast.success(`已重命名为「${v.to}」，涉及题目同步更新`)
      // 选中集与展开状态里的旧路径一并改写
      patchFilter({
        tags: filter.tags.map((t) =>
          t === v.from || t.startsWith(v.from + '/') ? v.to + t.slice(v.from.length) : t,
        ),
      })
      setExpanded((prev) =>
        Object.fromEntries(
          Object.entries(prev).map(([k, val]) => [
            k === v.from || k.startsWith(v.from + '/') ? v.to + k.slice(v.from.length) : k,
            val,
          ]),
        ),
      )
      await invalidateAfterTagChange()
      setRenaming(null)
    },
    onError: (e) => toast.error(e instanceof Error ? e.message : '重命名失败'),
  })

  const deleteMut = useMutation({
    mutationFn: (tag: string) => api.deleteTag(tag),
    onSuccess: async (d, tag) => {
      toast.success(`已从 ${d.updated} 道题目上移除该标签及子标签`)
      patchFilter({ tags: filter.tags.filter((t) => t !== tag && !t.startsWith(tag + '/')) })
      await invalidateAfterTagChange()
      setDeleting(null)
    },
    onError: (e) => toast.error(e instanceof Error ? e.message : '删除失败'),
  })

  // 无任何标签且无选中时整块隐藏，减少噪音
  if (!tagsQuery.isSuccess && raw.length === 0) return null
  if (raw.length === 0 && filter.tags.length === 0) return null

  return (
    <div className="px-2 py-2">
      {/* 头部：标题 + 命中数 + 展开/折叠 + 排序 */}
      <div className="mb-1 flex items-center gap-1 px-1">
        <span className="text-xs font-medium text-muted-foreground">标签</span>
        {typeof total === 'number' && (
          <Badge variant="secondary" className="h-4 px-1.5 text-[10px] tabular-nums">
            命中 {total}
          </Badge>
        )}
        <div className="ml-auto flex items-center gap-0.5">
          <button
            type="button"
            title="展开全部"
            className="rounded p-0.5 text-muted-foreground hover:text-foreground"
            onClick={() => setExpanded(Object.fromEntries(allPaths.map((t) => [t, true])))}
          >
            <ChevronsUpDownIcon className="size-3.5" />
          </button>
          <button
            type="button"
            title="折叠全部"
            className="rounded p-0.5 text-muted-foreground hover:text-foreground"
            onClick={() => setExpanded(Object.fromEntries(allPaths.map((t) => [t, false])))}
          >
            <ChevronsDownUpIcon className="size-3.5" />
          </button>
          {(
            [
              ['count', '数量'],
              ['name', '名称'],
            ] as ['count' | 'name', string][]
          ).map(([v, label]) => (
            <button
              key={v}
              type="button"
              onClick={() => setSortBy(v)}
              className={`rounded px-1 py-0.5 text-[10px] ${
                sortBy === v ? 'bg-muted font-medium text-foreground' : 'text-muted-foreground hover:text-foreground'
              }`}
            >
              {label}
            </button>
          ))}
        </div>
      </div>

      {/* 已选标签置顶：完整路径 chips + 单个移除 + 一键清空 */}
      {filter.tags.length > 0 && (
        <div className="mb-1.5 flex flex-wrap items-center gap-1 px-1">
          {filter.tags.map((t) => (
            <Badge key={t} variant="default" className="gap-1 text-xs">
              {t}
              <button type="button" className="ml-0.5 rounded-full hover:text-destructive" onClick={() => toggleSelect(t)} title="移除该标签">
                ×
              </button>
            </Badge>
          ))}
          <button
            type="button"
            className="text-[10px] text-muted-foreground underline-offset-2 hover:text-destructive hover:underline"
            onClick={() => patchFilter({ tags: [] })}
          >
            清空
          </button>
        </div>
      )}

      {/* 标签较多时提供树内查找 */}
      {allPaths.length > TAG_SEARCH_THRESHOLD && (
        <div className="relative mb-1.5 px-1">
          <SearchIcon className="pointer-events-none absolute top-1/2 left-3.5 size-3 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder={`在 ${allPaths.length} 个标签中查找…`}
            className="h-7 pl-6 text-xs"
          />
        </div>
      )}

      {/* 树体：根区作为“移到顶层”的落点 */}
      {tree.length === 0 ? (
        <div className="px-2 py-1 text-xs text-muted-foreground">{search ? '没有匹配的标签' : ''}</div>
      ) : (
        <div
          onDragOver={(e) => {
            if (!tagDragFrom) return
            e.preventDefault()
            e.dataTransfer.dropEffect = 'move'
          }}
          onDrop={(e) => {
            e.preventDefault()
            if (!tagDragFrom) return endTagDrag()
            const to = tagDragFrom.slice(tagDragFrom.lastIndexOf('/') + 1)
            setTagDragFrom(null)
            setTagDropTarget(null)
            if (to !== tagDragFrom) renameMut.mutate({ from: tagDragFrom, to })
          }}
        >
          {tree.map((n) => (
            <TagRow
              key={n.tag}
              node={n}
              level={0}
              selected={filter.tags}
              expanded={expanded}
              tagDragFrom={tagDragFrom}
              tagDropTarget={tagDropTarget}
              onToggleExpand={(tag) => setExpanded((prev) => ({ ...prev, [tag]: !isOpen(tag) }))}
              onToggleSelect={toggleSelect}
              onRename={setRenaming}
              onDelete={setDeleting}
              onTagDragStart={setTagDragFrom}
              onTagDragEnd={endTagDrag}
              onTagDragOver={handleTagDragOver}
              onTagDrop={handleTagDrop}
            />
          ))}
          {tagDragFrom && !tagDropTarget && (
            <div className="mt-1 rounded-md border border-dashed px-2 py-1.5 text-center text-[10px] text-muted-foreground">
              拖到此处移到顶层
            </div>
          )}
        </div>
      )}

      {/* 重命名对话框 */}
      {renaming !== null && (
        <RenameTagDialog
          tag={renaming}
          open
          onOpenChange={(v) => !v && setRenaming(null)}
          onSubmit={(to) => renameMut.mutate({ from: renaming, to })}
          pending={renameMut.isPending}
        />
      )}
      {/* 删除确认 */}
      <ConfirmDialog
        open={deleting !== null}
        onOpenChange={(v) => !v && setDeleting(null)}
        title={`删除标签「${deleting?.tag ?? ''}」？`}
        description={
          deleting && deleting.children.length > 0
            ? `将从所有题目上移除该标签及其 ${subtreeSize(deleting) - 1} 个子标签，涉及题目会同步更新。`
            : '将从所有题目上移除该标签，涉及题目会同步更新。'
        }
        onConfirm={() => deleting && deleteMut.mutate(deleting.tag)}
      />
    </div>
  )
}

function TagRow(props: {
  node: TagNode
  level: number
  selected: string[]
  expanded: Record<string, boolean>
  tagDragFrom: string | null
  tagDropTarget: string | null
  onToggleExpand: (tag: string) => void
  onToggleSelect: (tag: string) => void
  onRename: (tag: string) => void
  onDelete: (node: TagNode) => void
  onTagDragStart: (from: string) => void
  onTagDragEnd: () => void
  onTagDragOver: (target: string, e: DragEvent) => void
  onTagDrop: (target: string) => void
}) {
  const { node, level } = props
  // 菜单打开期间必须保持触发器可见：portal 菜单在 body 层，
  // 鼠标移出行的 group-hover 一旦失效，锚点 display:none 会让弹层定位塌缩到左上角
  const [menuOpen, setMenuOpen] = useState(false)
  const hasChildren = node.children.length > 0
  const open = props.expanded[node.tag] ?? level === 0
  const active = props.selected.includes(node.tag)
  const implied = !active && impliedBySelected(node.tag, props.selected)
  const isDraggedTag = props.tagDragFrom === node.tag
  const isValidDrop = !!props.tagDragFrom && props.tagDropTarget === node.tag

  return (
    <div>
      <div
        className={`group flex items-center rounded-md pr-1 ${
          active ? 'bg-accent text-accent-foreground' : 'hover:bg-muted'
        }`}
        style={{ paddingLeft: `${level * 14}px` }}
      >
        <button
          type="button"
          className={`flex size-5 shrink-0 items-center justify-center ${hasChildren ? '' : 'invisible'}`}
          onClick={() => props.onToggleExpand(node.tag)}
        >
          <ChevronRightIcon className={`size-3.5 text-muted-foreground transition-transform ${open ? 'rotate-90' : ''}`} />
        </button>
        <button
          type="button"
          draggable={props.tagDragFrom !== node.tag}
          onDragStart={(e) => {
            e.dataTransfer.effectAllowed = 'move'
            e.dataTransfer.setData('text/plain', node.tag)
            props.onTagDragStart(node.tag)
          }}
          onDragEnd={props.onTagDragEnd}
          onDragOver={(e) => props.onTagDragOver(node.tag, e)}
          onDrop={(e) => {
            e.preventDefault()
            e.stopPropagation()
            props.onTagDrop(node.tag)
          }}
          className={`flex min-w-0 flex-1 items-center gap-1.5 rounded-md py-1.5 text-left text-sm ${
            isValidDrop ? 'bg-primary/10 ring-1 ring-primary' : ''
          } ${isDraggedTag ? 'opacity-40' : ''}`}
          title={`${node.tag}（拖到其他标签上可改为其子标签）${implied ? '（已包含在已选父标签中）' : ''}`}
          onClick={() => props.onToggleSelect(node.tag)}
        >
          <TagIcon className={`size-3.5 shrink-0 ${implied ? 'text-muted-foreground/50' : 'text-primary/70'}`} />
          <span className={`flex-1 truncate ${implied ? 'text-muted-foreground/60' : ''}`}>{node.label}</span>
          <span className={`shrink-0 text-xs tabular-nums ${implied ? 'text-muted-foreground/50' : 'text-muted-foreground group-hover:hidden'}`}>
            {node.count > 0 ? node.count : ''}
          </span>
        </button>
        <div className={`shrink-0 items-center gap-0.5 ${menuOpen ? 'flex' : 'hidden group-hover:flex'}`}>
          <DropdownMenu open={menuOpen} onOpenChange={setMenuOpen}>
            <DropdownMenuTrigger
              className="rounded p-1 hover:bg-background"
              title="标签操作"
              onClick={(e) => e.stopPropagation()}
            >
              <MoreVerticalIcon className="size-3.5" />
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem onClick={() => props.onRename(node.tag)}>
                <PencilIcon className="size-3.5" /> 重命名（子树联动）
              </DropdownMenuItem>
              <DropdownMenuItem variant="destructive" onClick={() => props.onDelete(node)}>
                <TrashIcon className="size-3.5" /> 删除（含子标签）
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </div>
      {open &&
        hasChildren &&
        node.children.map((c) => (
          <TagRow
            key={c.tag}
            node={c}
            level={level + 1}
            selected={props.selected}
            expanded={props.expanded}
            tagDragFrom={props.tagDragFrom}
            tagDropTarget={props.tagDropTarget}
            onToggleExpand={props.onToggleExpand}
            onToggleSelect={props.onToggleSelect}
            onRename={props.onRename}
            onDelete={props.onDelete}
            onTagDragStart={props.onTagDragStart}
            onTagDragEnd={props.onTagDragEnd}
            onTagDragOver={props.onTagDragOver}
            onTagDrop={props.onTagDrop}
          />
        ))}
    </div>
  )
}

function RenameTagDialog(props: {
  tag: string
  open: boolean
  onOpenChange: (v: boolean) => void
  onSubmit: (to: string) => void
  pending: boolean
}) {
  const [value, setValue] = useState(props.tag)
  // 打开时同步预填值
  const [lastOpen, setLastOpen] = useState(false)
  if (props.open && !lastOpen) {
    setLastOpen(true)
    setValue(props.tag)
  } else if (!props.open && lastOpen) {
    setLastOpen(false)
  }

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className="sm:max-w-xs">
        <DialogHeader>
          <DialogTitle>重命名标签</DialogTitle>
        </DialogHeader>
        <Input
          value={value}
          onChange={(e) => setValue(e.target.value)}
          placeholder="完整路径，可用 / 分隔层级"
          autoFocus
          onKeyDown={(e) => e.key === 'Enter' && value.trim() && value !== props.tag && props.onSubmit(value.trim())}
        />
        <p className="text-xs text-muted-foreground">子标签将随前缀一起搬家，涉及题目自动更新。</p>
        <DialogFooter>
          <Button variant="outline" onClick={() => props.onOpenChange(false)}>
            取消
          </Button>
          <Button onClick={() => props.onSubmit(value.trim())} disabled={!value.trim() || value.trim() === props.tag || props.pending}>
            {props.pending ? '提交中…' : '确定'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

// ---------- 题目列表 ----------

function ProblemList() {
  const { filter } = useAppState()
  const listQuery = useQuery({
    queryKey: ['problems', filter],
    queryFn: () => api.problems(filter),
  })
  const problems = listQuery.data?.problems ?? []
  if (!listQuery.isSuccess) return null
  if (problems.length === 0) {
    return <div className="px-4 py-6 text-center text-xs text-muted-foreground">当前范围没有题目</div>
  }
  return (
    <div className="space-y-0.5 px-1 pb-2">
      {problems.map((p) => (
        <ProblemRow key={p.id} problem={p} />
      ))}
    </div>
  )
}

function ProblemRow({ problem }: { problem: ProblemSummary }) {
  const { view, openProblem, checked, toggleChecked } = useAppState()
  const selected = view.kind === 'problem' && view.id === problem.id
  const isChecked = checked.includes(problem.id)
  return (
    <div
      className={`group flex cursor-pointer items-center gap-1.5 rounded-md px-1.5 py-1.5 ${
        selected ? 'bg-accent text-accent-foreground' : 'hover:bg-muted'
      }`}
      onClick={() => openProblem(problem.id)}
    >
      <label className="flex shrink-0 items-center" onClick={(e) => e.stopPropagation()}>
        <input
          type="checkbox"
          className="size-3.5 accent-[var(--primary)]"
          checked={isChecked}
          onChange={() => toggleChecked(problem.id)}
        />
      </label>
      <Badge variant="outline" className="shrink-0 px-1.5 text-[10px] text-muted-foreground">
        {TYPE_LABEL[problem.type]}
      </Badge>
      <span className="min-w-0 flex-1 truncate text-sm">{problem.title}</span>
      <span className="hidden max-w-24 shrink-0 truncate text-[10px] text-muted-foreground group-hover:inline">
        {problem.tags.join(' / ')}
      </span>
    </div>
  )
}

// ---------- 训练 / 练习 分区（可折叠，状态持久化） ----------

function GroupSection({ kind }: { kind: 'training' | 'practice' }) {
  const isTraining = kind === 'training'
  const storageKey = `orangerepo:collapse:${kind}`
  const { view, openTraining, openPractice } = useAppState()
  const qc = useQueryClient()
  const [creating, setCreating] = useState(false)
  const [title, setTitle] = useState('')
  const [collapsed, setCollapsed] = useState(() => localStorage.getItem(storageKey) === '1')
  const listQuery = useQuery({
    queryKey: ['group-section', kind],
    queryFn: async (): Promise<{ trainings?: Training[]; practices?: Practice[] }> =>
      isTraining ? await api.trainings() : await api.practices(),
  })
  const groups = isTraining ? listQuery.data?.trainings : listQuery.data?.practices

  function toggleCollapsed() {
    setCollapsed((v) => {
      localStorage.setItem(storageKey, v ? '0' : '1')
      return !v
    })
    setCreating(false)
  }

  async function create() {
    if (!title.trim()) return
    try {
      const id = isTraining ? (await api.createTraining(title.trim())).id : (await api.createPractice(title.trim())).id
      await qc.invalidateQueries({ queryKey: ['group-section', kind] })
      if (isTraining) openTraining(id)
      else openPractice(id)
      setTitle('')
      setCreating(false)
    } catch (e) {
      toast.error(e instanceof Error ? e.message : '创建失败')
    }
  }

  return (
    <div className="border-t">
      <div className="flex items-center gap-1 px-2 py-1.5">
        <button
          type="button"
          className="flex min-w-0 flex-1 items-center gap-1 rounded-md py-0.5 text-left hover:bg-muted"
          onClick={toggleCollapsed}
          title={collapsed ? '展开' : '折叠'}
        >
          <ChevronRightIcon className={`size-3.5 shrink-0 text-muted-foreground transition-transform ${collapsed ? '' : 'rotate-90'}`} />
          {isTraining ? (
            <FileCodeIcon className="size-3.5 shrink-0 text-muted-foreground" />
          ) : (
            <ListChecksIcon className="size-3.5 shrink-0 text-muted-foreground" />
          )}
          <span className="truncate text-xs font-medium text-muted-foreground">
            {isTraining ? '训练计划' : '练习'}
            {groups && groups.length > 0 && `（${groups.length}）`}
          </span>
        </button>
        <button
          type="button"
          className="rounded p-0.5 hover:bg-muted"
          title={`新建${isTraining ? '训练' : '练习'}`}
          onClick={() => {
            setCollapsed(false)
            localStorage.setItem(storageKey, '0')
            setCreating((v) => !v)
          }}
        >
          <PlusIcon className="size-3.5" />
        </button>
      </div>
      {!collapsed && (
        <>
          {creating && (
            <div className="flex gap-1 px-3 pb-1.5">
              <Input
                value={title}
                onChange={(e) => setTitle(e.target.value)}
                placeholder={`${isTraining ? '训练' : '练习'}名称`}
                className="h-7 text-xs"
                autoFocus
                onKeyDown={(e) => e.key === 'Enter' && create()}
              />
              <Button size="xs" onClick={create}>
                确定
              </Button>
            </div>
          )}
          <div className="max-h-32 overflow-y-auto px-2 pb-2">
            {(groups ?? []).map((g) => {
              const active = view.kind === kind && view.id === g.id
              return (
                <button
                  key={g.id}
                  type="button"
                  className={`flex w-full items-center gap-1.5 rounded-md px-2 py-1 text-left text-xs ${
                    active ? 'bg-accent font-medium text-accent-foreground' : 'hover:bg-muted'
                  }`}
                  onClick={() => (isTraining ? openTraining(g.id) : openPractice(g.id))}
                >
                  <FileCodeIcon className="size-3 shrink-0 text-muted-foreground" />
                  <span className="min-w-0 flex-1 truncate">{g.title}</span>
                  <span className="shrink-0 text-muted-foreground">{g.problemCount}</span>
                </button>
              )
            })}
          </div>
        </>
      )}
    </div>
  )
}
