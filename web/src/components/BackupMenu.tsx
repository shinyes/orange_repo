// 全库备份/迁移菜单：导出整库单文件 / 导入备份包恢复（全部新建，不覆盖现有数据）。
import { useRef, useState } from 'react'
import { toast } from 'sonner'
import { useQueryClient } from '@tanstack/react-query'
import { ArchiveIcon, DownloadIcon, UploadIcon, Loader2Icon } from 'lucide-react'

import { api } from '@/lib/api'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'

export function BackupMenu() {
  const qc = useQueryClient()
  const fileRef = useRef<HTMLInputElement>(null)
  const [busy, setBusy] = useState(false)

  async function importFile(file: File) {
    setBusy(true)
    try {
      const r = await api.importBackup(file)
      toast.success(`备份导入完成：题目 ${r.imported} 道、训练 ${r.trainings} 个、练习 ${r.practices} 个（全部新建，未覆盖现有数据）`)
      await qc.invalidateQueries()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '导入失败')
    } finally {
      setBusy(false)
      if (fileRef.current) fileRef.current.value = ''
    }
  }

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger
          disabled={busy}
          title="全库备份 / 迁移"
          className="inline-flex size-7 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
        >
          {busy ? <Loader2Icon className="size-4 animate-spin" /> : <ArchiveIcon className="size-4" />}
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start">
          <DropdownMenuItem
            onClick={() => {
              window.open(api.exportBackupUrl(), '_blank')
            }}
          >
            <DownloadIcon /> 导出全库（备份）
          </DropdownMenuItem>
          <DropdownMenuItem onClick={() => fileRef.current?.click()}>
            <UploadIcon /> 导入全库备份…
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <p className="px-2 py-1.5 text-[11px] leading-snug text-muted-foreground">
            备份含全部题目/目录/训练/练习；导入一律新建副本，不覆盖现有数据
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
    </>
  )
}
