// 域备份/迁移菜单（域管理行内）：导出该域为单文件 / 导入备份包到该域（全部新建，不覆盖现有数据）。
// 导入为异步任务：上传后展示进度条（题目/训练/练习各阶段），完成后提示，避免误操作。
import { useRef, useState } from 'react'
import { toast } from 'sonner'
import { useQueryClient } from '@tanstack/react-query'
import { ArchiveIcon, CheckCircle2Icon, DownloadIcon, Loader2Icon, UploadIcon, XCircleIcon } from 'lucide-react'

import { adminApi } from '@/api/admin'
import type { ImportTaskView } from '@/api/types'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'

export function DomainBackupMenu({ domainId, domainName }: { domainId: number; domainName: string }) {
  const qc = useQueryClient()
  const fileRef = useRef<HTMLInputElement>(null)
  const [busy, setBusy] = useState(false)
  // 导入进度弹窗（task 轮询视图）
  const [task, setTask] = useState<ImportTaskView | null>(null)
  const [taskVisible, setTaskVisible] = useState(false)
  const [taskError, setTaskError] = useState<string | null>(null)

  async function importFile(file: File) {
    setBusy(true)
    setTaskError(null)
    try {
      const { taskId } = await adminApi.startImportBackup(file, domainId)
      setTask({ done: false, ok: false, phase: '解析中', message: '正在解析备份包…', current: 0, total: 0 })
      setTaskVisible(true)
      // 轮询至完成
      for (;;) {
        await new Promise((r) => setTimeout(r, 500))
        const t = await adminApi.importTask(taskId)
        setTask(t)
        if (t.done) {
          if (!t.ok) {
            setTaskError(t.error || '导入失败')
            toast.error('导入失败，已全部回滚')
          } else {
            toast.success(
              `导入完成：题目 ${t.result?.imported ?? 0} 道、训练 ${t.result?.trainings ?? 0} 个、练习 ${t.result?.practices ?? 0} 个（全部新建，未覆盖现有数据）`,
            )
            setTaskVisible(false)
            void qc.invalidateQueries()
          }
          break
        }
      }
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '导入失败')
      setTaskError(err instanceof Error ? err.message : '导入失败')
    } finally {
      setBusy(false)
      if (fileRef.current) fileRef.current.value = ''
    }
  }

  const closeDialog = () => {
    // 任务进行中不允许关闭（防误操作）；仅在出错/完成可关
    if (task && !task.done) return
    setTaskVisible(false)
  }

  // 计算进度百分比（多阶段换算：解析 5% + 题目 60% + 训练 25% + 练习 10%）
  const progress = task ? progressPct(task) : 0
  const running = task != null && !task.done

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger
          disabled={busy}
          title="备份 / 迁移该域"
          className="inline-flex size-7 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
        >
          {busy ? <Loader2Icon className="size-4 animate-spin" /> : <ArchiveIcon className="size-4" />}
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start">
          <DropdownMenuItem
            onClick={() => {
              window.open(adminApi.exportBackupUrl(domainId), '_blank')
            }}
          >
            <DownloadIcon /> 导出该域（备份）
          </DropdownMenuItem>
          <DropdownMenuItem onClick={() => fileRef.current?.click()}>
            <UploadIcon /> 导入备份到该域…
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <p className="px-2 py-1.5 text-[11px] leading-snug text-muted-foreground">
            备份 = {domainName} 域全部题目/目录/训练/练习；导入一律新建副本，不覆盖现有数据
          </p>
        </DropdownMenuContent>
      </DropdownMenu>
      <input
        ref={fileRef}
        type="file"
        accept=".zip"
        className="hidden"
        onChange={(e) => {
          const f = e.target.files?.[0]
          if (f) void importFile(f)
        }}
      />

      {/* 导入进度对话框 */}
      <Dialog open={taskVisible} onOpenChange={() => closeDialog()}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              {task?.ok ? <CheckCircle2Icon className="size-4 text-emerald-600" /> : taskError ? <XCircleIcon className="size-4 text-red-600" /> : null}
              {task?.done ? (task?.ok ? '导入完成' : '导入失败') : '正在导入…'}
            </DialogTitle>
            <DialogDescription>
              {task?.done
                ? task?.ok
                  ? `题目 ${task.result?.imported ?? 0} 道、训练 ${task.result?.trainings ?? 0} 个、练习 ${task.result?.practices ?? 0} 个`
                  : '导入出错，已全部回滚，未影响现有数据'
                : '导入过程中请勿关闭页面或重复操作'}
            </DialogDescription>
          </DialogHeader>

          {!task?.done && (
            <div className="space-y-2 py-2">
              <div className="flex items-center justify-between text-xs text-muted-foreground">
                <span className="flex items-center gap-1.5">
                  <Loader2Icon className="size-3.5 animate-spin text-primary" />
                  {task?.message ?? '准备中…'}
                </span>
                <span className="tabular-nums">{progress}%</span>
              </div>
              <div className="h-2 w-full overflow-hidden rounded-full bg-muted">
                <div
                  className="h-full rounded-full bg-primary transition-all duration-300"
                  style={{ width: `${Math.max(2, progress)}%` }}
                />
              </div>
            </div>
          )}

          {taskError && (
            <p className="rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-xs leading-relaxed text-red-600">{taskError}</p>
          )}

          <DialogFooter>
            {running && <p className="mr-auto self-center text-[11px] text-muted-foreground">进行中，暂不能关闭</p>}
            {!running && (
              <Button onClick={closeDialog} variant={taskError ? 'default' : 'outline'}>
                关闭
              </Button>
            )}
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}

// 阶段 → 百分比换算：解析 5% → 题目 60% → 训练 25% → 练习 10%
function progressPct(t: ImportTaskView): number {
  if (t.done) return t.ok ? 100 : 100
  const base: Record<string, [number, number]> = {
    解析中: [0, 5],
    题目: [5, 65],
    训练: [65, 90],
    练习: [90, 100],
  }
  const seg = base[t.phase] ?? [0, 5]
  const span = seg[1] - seg[0]
  const p = t.total > 0 ? Math.min(1, t.current / t.total) : 0
  return Math.round(seg[0] + p * span)
}
