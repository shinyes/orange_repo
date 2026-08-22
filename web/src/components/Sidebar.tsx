import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import {
  ChevronRightIcon,
  DownloadIcon,
  FileCodeIcon,
  FolderIcon,
  FolderPlusIcon,
  ListChecksIcon,
  PencilIcon,
  PlusIcon,
  SearchIcon,
  SettingsIcon,
  TrashIcon,
  UploadIcon,
  XIcon,
} from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Separator } from '@/components/ui/separator'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { api } from '@/lib/api'
import { useAppState } from '@/lib/app-context'
import type { DirectoryNode, ProblemSummary, ProblemType, Practice, Training } from '@/lib/types'
import { AddToGroupDialog, ConfirmDialog, DirectoryDialog, ImportDialog, NewProblemDialog } from './dialogs'

const TYPE_LABEL: Record<ProblemType, string> = { programming: '编程', single_choice: '单选', true_false: '判断' }

// 左栏：管理（新建/上传/下载/搜索/目录树/题目列表/训练练习入口）。
export function Sidebar({ onLogout, onOpenSettings }: { onLogout: () => void; onOpenSettings: () => void }) {
  const { filter, patchFilter, checked, clearChecked } = useAppState()
  const [newProblem, setNewProblem] = useState(false)
  const [newDirParent, setNewDirParent] = useState<{ open: boolean; parent: number | null }>({ open: false, parent: null })
  const [renameDir, setRenameDir] = useState<DirectoryNode | null>(null)
  const [deleteDir, setDeleteDir] = useState<DirectoryNode | null>(null)
  const [importOpen, setImportOpen] = useState(false)
  const [addToGroup, setAddToGroup] = useState<'training' | 'practice' | null>(null)

  async function deleteDirectory(dir: DirectoryNode) {
    try {
      await api.deleteDirectory(dir.id)
      toast.success('目录已删除')
    } catch (e) {
      toast.error(e instanceof Error ? e.message : '删除失败')
    }
  }

  return (
    <div className="flex h-full flex-col bg-sidebar">
      {/* 头部 */}
      <div className="flex items-center gap-2 px-3 pt-3">
        <span className="flex size-8 items-center justify-center rounded-lg bg-primary text-lg text-primary-foreground">🍊</span>
        <div className="min-w-0 flex-1 leading-tight">
          <div className="truncate text-sm font-semibold">OrangeRepo</div>
          <div className="text-[10px] text-muted-foreground">题库管理 · OrangeOJ 兼容</div>
        </div>
        <Button variant="ghost" size="icon-sm" title="修改密码" onClick={onOpenSettings}>
          <SettingsIcon />
        </Button>
        <Button variant="ghost" size="icon-sm" title="退出登录" onClick={onLogout}>
          <XIcon />
        </Button>
      </div>

      {/* 操作区 */}
      <div className="flex items-center gap-1.5 px-3 pt-3">
        <Button size="sm" className="flex-1" onClick={() => setNewProblem(true)}>
          <PlusIcon data-icon="inline-start" /> 题目
        </Button>
        <Button
          size="sm"
          variant="outline"
          className="flex-1"
          onClick={() => setNewDirParent({ open: true, parent: filter.dirId })}
        >
          <FolderPlusIcon data-icon="inline-start" /> 目录
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

      {/* 搜索 */}
      <div className="relative px-3 pt-2">
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

      {/* 标签 */}
      <TagBar />

      <Separator className="mt-1" />

      {/* 目录树 + 题目列表 */}
      <ScrollArea className="min-h-0 flex-1">
        <div className="px-2 py-1">
          <AllProblemsNode />
          <DirectoryTreeView
            level={0}
            onRename={setRenameDir}
            onDelete={setDeleteDir}
            onAddChild={(parent) => setNewDirParent({ open: true, parent })}
          />
        </div>
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

      {/* 训练 / 练习 分区 */}
      <GroupSection kind="training" />
      <GroupSection kind="practice" />

      {/* 对话框 */}
      <NewProblemDialog open={newProblem} onOpenChange={setNewProblem} />
      <DirectoryDialog
        mode="create"
        parent={newDirParent.parent}
        open={newDirParent.open}
        onOpenChange={(v) => setNewDirParent({ open: v, parent: v ? newDirParent.parent : null })}
      />
      {renameDir && (
        <DirectoryDialog mode="rename" target={renameDir} open={!!renameDir} onOpenChange={(v) => !v && setRenameDir(null)} />
      )}
      <ConfirmDialog
        open={!!deleteDir}
        onOpenChange={(v) => !v && setDeleteDir(null)}
        title={`删除目录「${deleteDir?.name ?? ''}」？`}
        description="子目录与其中题目将上移到上级目录，不会删除题目。"
        onConfirm={() => {
          if (deleteDir) void deleteDirectory(deleteDir)
          setDeleteDir(null)
        }}
      />
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

// ---------- 标签栏 ----------

function TagBar() {
  const { filter, patchFilter } = useAppState()
  const tagsQuery = useQuery({ queryKey: ['tags'], queryFn: api.tags })
  const tags = tagsQuery.data?.tags ?? []
  if (tags.length === 0) return null
  return (
    <div className="max-h-28 overflow-y-auto px-3 pt-2 pb-1">
      <div className="flex flex-wrap gap-1">
        {tags.map((t) => {
          const active = filter.tags.includes(t.tag)
          return (
            <button
              key={t.tag}
              type="button"
              onClick={() => patchFilter({ tags: active ? filter.tags.filter((x) => x !== t.tag) : [...filter.tags, t.tag] })}
            >
              <Badge variant={active ? 'default' : 'outline'} className="cursor-pointer text-xs hover:bg-muted">
                {t.tag} · {t.count}
              </Badge>
            </button>
          )
        })}
      </div>
    </div>
  )
}

// ---------- 目录树 ----------

function AllProblemsNode() {
  const { filter, patchFilter, goHome, view } = useAppState()
  const dirsQuery = useQuery({ queryKey: ['directories'], queryFn: api.directories })
  const total = countTree(dirsQuery.data?.directories ?? [])
  const active = filter.dirId === null
  return (
    <button
      type="button"
      onClick={() => {
        patchFilter({ dirId: null })
        if (view.kind !== 'empty') goHome()
      }}
      className={`mb-0.5 flex w-full items-center gap-1.5 rounded-md px-2 py-1.5 text-left text-sm ${
        active ? 'bg-accent font-medium text-accent-foreground' : 'hover:bg-muted'
      }`}
    >
      <FolderIcon className="size-4 shrink-0 text-primary" />
      <span className="flex-1 truncate">全部题目</span>
      <span className="text-xs text-muted-foreground">{total}</span>
    </button>
  )
}

function countTree(nodes: DirectoryNode[]): number {
  let n = nodes.reduce((acc, d) => acc + d.problemCount, 0)
  for (const d of nodes) n += countTree(d.children)
  return n
}

function DirectoryTreeView(props: {
  level: number
  onRename: (d: DirectoryNode) => void
  onDelete: (d: DirectoryNode) => void
  onAddChild: (parent: number) => void
}) {
  const dirsQuery = useQuery({ queryKey: ['directories'], queryFn: api.directories })
  return (
    <>
      {(dirsQuery.data?.directories ?? []).map((d) => (
        <DirRow
          key={d.id}
          node={d}
          level={props.level}
          onRename={props.onRename}
          onDelete={props.onDelete}
          onAddChild={props.onAddChild}
        />
      ))}
    </>
  )
}

function DirRow(props: {
  node: DirectoryNode
  level: number
  onRename: (d: DirectoryNode) => void
  onDelete: (d: DirectoryNode) => void
  onAddChild: (parent: number) => void
}) {
  const { node, level } = props
  const [expanded, setExpanded] = useState(level < 1)
  const { filter, patchFilter, goHome, view } = useAppState()
  const active = filter.dirId === node.id

  return (
    <div>
      <div
        className={`group flex items-center rounded-md pr-1 ${active ? 'bg-accent text-accent-foreground' : 'hover:bg-muted'}`}
        style={{ paddingLeft: `${level * 14}px` }}
      >
        <button type="button" className="flex size-5 shrink-0 items-center justify-center" onClick={() => setExpanded(!expanded)}>
          <ChevronRightIcon className={`size-3.5 text-muted-foreground transition-transform ${expanded ? 'rotate-90' : ''}`} />
        </button>
        <button
          type="button"
          className="flex min-w-0 flex-1 items-center gap-1.5 py-1.5 text-left text-sm"
          onClick={() => {
            patchFilter({ dirId: node.id })
            if (view.kind !== 'empty') goHome()
          }}
        >
          <FolderIcon className="size-4 shrink-0 text-primary/70" />
          <span className="flex-1 truncate">{node.name}</span>
          <span className="text-xs text-muted-foreground group-hover:hidden">
            {node.problemCount > 0 ? node.problemCount : ''}
          </span>
        </button>
        <div className="hidden shrink-0 items-center gap-0.5 group-hover:flex">
          <button
            type="button"
            title="新增子目录"
            className="rounded p-1 hover:bg-background"
            onClick={() => props.onAddChild(node.id)}
          >
            <PlusIcon className="size-3.5" />
          </button>
          <button type="button" title="重命名" className="rounded p-1 hover:bg-background" onClick={() => props.onRename(node)}>
            <PencilIcon className="size-3.5" />
          </button>
          <button type="button" title="删除" className="rounded p-1 hover:bg-background" onClick={() => props.onDelete(node)}>
            <TrashIcon className="size-3.5 text-destructive" />
          </button>
        </div>
      </div>
      {expanded && (
        <>
          {node.children.map((c) => (
            <DirRow
              key={c.id}
              node={c}
              level={level + 1}
              onRename={props.onRename}
              onDelete={props.onDelete}
              onAddChild={props.onAddChild}
            />
          ))}
          {node.children.length === 0 && (
            <div className="py-1 text-xs text-muted-foreground" style={{ paddingLeft: `${(level + 1) * 14 + 22}px` }}>
              （空目录）
            </div>
          )}
        </>
      )}
    </div>
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

// ---------- 训练 / 练习 分区 ----------

function GroupSection({ kind }: { kind: 'training' | 'practice' }) {
  const isTraining = kind === 'training'
  const { view, openTraining, openPractice } = useAppState()
  const qc = useQueryClient()
  const [creating, setCreating] = useState(false)
  const [title, setTitle] = useState('')
  const listQuery = useQuery({
    queryKey: ['group-section', kind],
    queryFn: async (): Promise<{ trainings?: Training[]; practices?: Practice[] }> =>
      isTraining ? await api.trainings() : await api.practices(),
  })
  const groups = isTraining ? listQuery.data?.trainings : listQuery.data?.practices

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
      <div className="flex items-center gap-1 px-3 py-1.5">
        {isTraining ? (
          <FolderIcon className="size-3.5 text-muted-foreground" />
        ) : (
          <ListChecksIcon className="size-3.5 text-muted-foreground" />
        )}
        <span className="text-xs font-medium text-muted-foreground">{isTraining ? '训练计划' : '练习'}</span>
        <button
          type="button"
          className="ml-auto rounded p-0.5 hover:bg-muted"
          title={`新建${isTraining ? '训练' : '练习'}`}
          onClick={() => setCreating(!creating)}
        >
          <PlusIcon className="size-3.5" />
        </button>
      </div>
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
    </div>
  )
}
