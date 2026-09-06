// 域备份/迁移菜单（域管理行内）：导出该域为单文件 / 导入备份包到该域（全部新建，不覆盖现有数据）。
// 仓库内容（题目/目录/训练/练习）与域一一对应，故备份操作以域为单元。
import { useRef, useState } from 'react'
import { toast } from 'sonner'
import { useQueryClient } from '@tanstack/react-query'
import { ArchiveIcon, DownloadIcon, UploadIcon, Loader2Icon } from 'lucide-react'

import { api } from '@/api'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'

export function DomainBackupMenu({ domainId, domainName }: { domainId: number; domainName: string }) {
  const qc = useQueryClient()
  const fileRef = useRef<HTMLInputElement>(null)
  const [busy, setBusy] = useState(false)

  async function importFile(file: File) {
    setBusy(true)
    try {
      const r = await api.importBackup(file, domainId)
      toast.success(`导入完成：题目 ${r.imported} 道、训练 ${r.trainings} 个、练习 ${r.practices} 个（全部新建，未覆盖现有数据）`)
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
          title="备份 / 迁移该域"
          className="inline-flex size-7 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
        >
          {busy ? <Loader2Icon className="size-4 animate-spin" /> : <ArchiveIcon className="size-4" />}
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start">
          <DropdownMenuItem
            onClick={() => {
              window.open(api.exportBackupUrl(domainId), '_blank')
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
    </>
  )
}
