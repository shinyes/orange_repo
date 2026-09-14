// 书包（Scratch 工程库）：文件夹树 + 工程列表的共同实现。
// 两个入口共用本文件的取数逻辑与列表 UI：
//   · 顶栏「书包」面板（BackpackDialog）：完整管理（新建文件夹/改名/移动/删除/下载）
//   · Scratch 页「从书包打开」（BackpackPickerDialog）：只挑一个工程载入
import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { toast } from 'sonner'
import { FolderIcon, FolderOpenIcon, HardDriveDownloadIcon, Loader2Icon, PencilIcon, Trash2Icon } from 'lucide-react'

import { api } from '@/api'
import type { ScratchFolder, ScratchProject } from '@/api/types'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'

/** 书包取数：文件夹 + 工程（folderId 省略=全部；0=根目录）。 */
export function useBackpack(folderId?: number) {
  const foldersQ = useQuery({ queryKey: ['scratch-folders'], queryFn: api.scratchFolders })
  const projectsQ = useQuery({ queryKey: ['scratch-projects', folderId ?? 'all'], queryFn: () => api.scratchProjects(folderId) })
  return {
    folders: foldersQ.data?.folders ?? [],
    usage: foldersQ.data?.usage,
    projects: projectsQ.data?.projects ?? [],
    loading: foldersQ.isLoading || projectsQ.isLoading,
    refetch: () => { void foldersQ.refetch(); void projectsQ.refetch() },
  }
}

/** 文件夹扁平列表 → 带缩进的选项（层级用前缀表示，避免再写一棵树组件）。 */
export function folderOptions(folders: ScratchFolder[]): { id: number; label: string }[] {
  const byParent = new Map<number | 'root', ScratchFolder[]>()
  for (const f of folders) {
    const key = f.parentId ?? 'root'
    const arr = byParent.get(key) ?? []
    arr.push(f)
    byParent.set(key, arr)
  }
  const out: { id: number; label: string }[] = []
  const walk = (parent: number | 'root', depth: number) => {
    for (const f of (byParent.get(parent) ?? []).sort((a, b) => a.name.localeCompare(b.name, 'zh-CN'))) {
      out.push({ id: f.id, label: `${'　'.repeat(depth)}${depth > 0 ? '└ ' : ''}${f.name}` })
      walk(f.id, depth + 1)
    }
  }
  walk('root', 0)
  return out
}

export function formatBytes(n: number): string {
  if (n >= 1024 * 1024) return `${(n / 1024 / 1024).toFixed(1)} MB`
  if (n >= 1024) return `${(n / 1024).toFixed(0)} KB`
  return `${n} B`
}

/** 顶栏「书包」：完整管理面板。 */
export function BackpackDialog({ open, onOpenChange, onOpenInScratch }: {
  open: boolean
  onOpenChange: (v: boolean) => void
  /** 在 Scratch 里打开（由调用方负责跳转到 Scratch 空间页并带上 openProject） */
  onOpenInScratch: (p: ScratchProject) => void
}) {
  const [folderId, setFolderId] = useState<number | undefined>(undefined)
  const { folders, projects, usage, loading, refetch } = useBackpack(folderId)
  const [busy, setBusy] = useState(false)
  const options = useMemo(() => folderOptions(folders), [folders])

  async function newFolder() {
    const name = window.prompt('新建文件夹名称')
    if (!name?.trim()) return
    setBusy(true)
    try {
      await api.createScratchFolder(name.trim(), folderId && folderId > 0 ? folderId : null)
      toast.success('文件夹已创建')
      refetch()
    } catch (e) {
      toast.error(e instanceof Error ? e.message : '创建失败')
    } finally {
      setBusy(false)
    }
  }

  async function renameProject(p: ScratchProject) {
    const name = window.prompt('重命名作品', p.name)
    if (name === null || !name.trim()) return
    try {
      await api.updateScratchProject(p.id, { name: name.trim() })
      toast.success('已重命名')
      refetch()
    } catch (e) {
      toast.error(e instanceof Error ? e.message : '重命名失败')
    }
  }

  async function moveProject(p: ScratchProject, to: number) {
    try {
      await api.updateScratchProject(p.id, { folderId: to })
      toast.success('已移动')
      refetch()
    } catch (e) {
      toast.error(e instanceof Error ? e.message : '移动失败')
    }
  }

  async function removeProject(p: ScratchProject) {
    if (!window.confirm(`删除《${p.name}》？该操作不可撤销。`)) return
    try {
      await api.deleteScratchProject(p.id)
      toast.success('已删除')
      refetch()
    } catch (e) {
      toast.error(e instanceof Error ? e.message : '删除失败')
    }
  }

  async function download(p: ScratchProject) {
    try {
      const bytes = await api.scratchProjectBytes(p.id)
      const url = URL.createObjectURL(new Blob([bytes], { type: 'application/x.scratch.sb3' }))
      const a = document.createElement('a')
      a.href = url
      a.download = `${p.name}.sb3`
      a.click()
      URL.revokeObjectURL(url)
    } catch (e) {
      toast.error(e instanceof Error ? e.message : '下载失败')
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex max-h-[85vh] flex-col rounded-2xl border-slate-200 shadow-xl sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2 text-[#4c97ff]">`n            <span className="inline-flex size-7 items-center justify-center rounded-lg bg-[#4c97ff]/10 text-base">🎒</span>`n            书包`n          </DialogTitle>
          <DialogDescription>
            保存的 Scratch 作品（服务端存储，含图片与声音素材；换设备也能打开）。
            {usage && ` 已用 ${formatBytes(usage.usedBytes)} / ${formatBytes(usage.quotaBytes)}。`}
          </DialogDescription>
        </DialogHeader>

        <div className="flex min-h-0 flex-1 gap-3">
          {/* 文件夹栏 */}
          <div className="w-44 shrink-0 space-y-1 overflow-y-auto rounded-xl border border-slate-200 bg-slate-50/70 p-2">
            <button
              type="button"
              className={`flex w-full items-center gap-1.5 rounded px-1.5 py-1 text-left text-xs ${folderId === undefined ? 'bg-accent' : 'hover:bg-muted'}`}
              onClick={() => setFolderId(undefined)}
            >
              <FolderOpenIcon className="size-3.5" /> 全部作品
            </button>
            <button
              type="button"
              className={`flex w-full items-center gap-1.5 rounded px-1.5 py-1 text-left text-xs ${folderId === 0 ? 'bg-accent' : 'hover:bg-muted'}`}
              onClick={() => setFolderId(0)}
            >
              <FolderIcon className="size-3.5" /> 未分类（根目录）
            </button>
            <div className="my-1 border-t" />
            {options.map((o) => (
              <button
                key={o.id}
                type="button"
                className={`block w-full truncate rounded px-1.5 py-1 text-left text-xs ${folderId === o.id ? 'bg-accent' : 'hover:bg-muted'}`}
                title={o.label}
                onClick={() => setFolderId(o.id)}
              >
                {o.label}
              </button>
            ))}
            <div className="pt-1">
              <Button size="xs" variant="outline" disabled={busy} onClick={() => void newFolder()}>
                新建文件夹
              </Button>
            </div>
          </div>

          {/* 作品列表 */}
          <div className="min-h-0 flex-1 overflow-y-auto rounded-lg border">
            {loading ? (
              <div className="flex h-40 items-center justify-center text-xs text-muted-foreground">
                <Loader2Icon className="mr-2 size-3.5 animate-spin" /> 加载中…
              </div>
            ) : projects.length === 0 ? (
              <div className="flex h-40 flex-col items-center justify-center gap-1 text-xs text-muted-foreground">
                <span>这里还没有作品</span>
                <span>进入 Scratch 创作页，点「保存到书包」即可</span>
              </div>
            ) : (
              projects.map((p) => (
                <div key={p.id} className="flex items-center gap-2 border-b px-2.5 py-2 text-xs last:border-b-0 hover:bg-muted/40">
                  <div className="min-w-0 flex-1">
                    <p className="truncate font-medium">{p.name}</p>
                    <p className="text-[11px] text-muted-foreground">
                      {formatBytes(p.size)} · 更新于 {new Date(p.updatedAt).toLocaleString('zh-CN', { hour12: false })}
                    </p>
                  </div>
                  <select
                    className="h-7 rounded border bg-background px-1 text-[11px]"
                    value={p.folderId ?? 0}
                    onChange={(e) => void moveProject(p, Number(e.target.value))}
                    title="移动到文件夹"
                  >
                    <option value={0}>根目录</option>
                    {options.map((o) => <option key={o.id} value={o.id}>{o.label.trim()}</option>)}
                  </select>
                  <Button size="xs" variant="outline" onClick={() => { onOpenChange(false); onOpenInScratch(p) }}>
                    在 Scratch 中打开
                  </Button>
                  <Button size="icon-xs" variant="ghost" title="重命名" onClick={() => void renameProject(p)}>
                    <PencilIcon className="size-3.5" />
                  </Button>
                  <Button size="icon-xs" variant="ghost" title="下载 .sb3" onClick={() => void download(p)}>
                    <HardDriveDownloadIcon className="size-3.5" />
                  </Button>
                  <Button size="icon-xs" variant="ghost" title="删除" onClick={() => void removeProject(p)}>
                    <Trash2Icon className="size-3.5" />
                  </Button>
                </div>
              ))
            )}
          </div>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>关闭</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

/** Scratch 页「从书包打开」：只挑一个作品。 */
export function BackpackPickerDialog({ open, onOpenChange, onPick }: {
  open: boolean
  onOpenChange: (v: boolean) => void
  onPick: (p: ScratchProject) => void
}) {
  const { projects, loading } = useBackpack()
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>从书包打开</DialogTitle>
          <DialogDescription>选择要载入编辑器的作品（当前作品未保存的改动会丢失）。</DialogDescription>
        </DialogHeader>
        <div className="max-h-[55vh] overflow-y-auto rounded-lg border">
          {loading ? (
            <div className="flex h-32 items-center justify-center text-xs text-muted-foreground">
              <Loader2Icon className="mr-2 size-3.5 animate-spin" /> 加载中…
            </div>
          ) : projects.length === 0 ? (
            <div className="flex h-32 items-center justify-center text-xs text-muted-foreground">书包里还没有作品</div>
          ) : (
            projects.map((p) => (
              <button
                key={p.id}
                type="button"
                className="flex w-full items-center gap-2 border-b px-3 py-2.5 text-left text-xs last:border-b-0 hover:bg-muted/50"
                onClick={() => onPick(p)}
              >
                <div className="min-w-0 flex-1">
                  <p className="truncate font-medium">{p.name}</p>
                  <p className="text-[11px] text-muted-foreground">
                    {formatBytes(p.size)} · {new Date(p.updatedAt).toLocaleString('zh-CN', { hour12: false })}
                  </p>
                </div>
                <span className="text-[11px] text-primary">载入 →</span>
              </button>
            ))
          )}
        </div>
      </DialogContent>
    </Dialog>
  )
}
