// 域设置弹窗（域管理行内「设置」）：目前含「排行榜公开」开关。
// 关闭后普通成员无法查看本域排行榜（接口返回 403），管理员始终可看。
import { useState } from 'react'
import { toast } from 'sonner'
import { SettingsIcon } from 'lucide-react'

import { adminApi } from '@/api/admin'
import type { Domain } from '@/api/types'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Switch } from '@/components/ui/switch'

export function DomainSettingsDialog(props: {
  domain: Domain | null
  onOpenChange: (v: boolean) => void
  /** 保存成功后回调（父组件用于刷新域列表）。 */
  onSaved: () => void
}) {
  const d = props.domain
  const [busy, setBusy] = useState(false)
  const [lastDomainId, setLastDomainId] = useState<number | null>(null)
  const [leaderboardPublic, setLeaderboardPublic] = useState(true)
  // 打开时从当前域记录初始化（简单同步：每次打开的域变化时重置）
  if (d && lastDomainId !== d.id) {
    setLastDomainId(d.id)
    setLeaderboardPublic(d.leaderboardPublic ?? true) // 存量域缺省按公开
  } else if (!d && lastDomainId !== null) {
    setLastDomainId(null)
  }

  async function save() {
    if (!d) return
    setBusy(true)
    try {
      await adminApi.updateDomainSettings(d.id, { leaderboardPublic })
      toast.success(leaderboardPublic ? '排行榜已公开' : '排行榜已设为不公开（仅管理员可见）')
      props.onSaved()
      props.onOpenChange(false)
    } catch (e) {
      toast.error(e instanceof Error ? e.message : '保存失败')
    } finally {
      setBusy(false)
    }
  }

  return (
    <Dialog open={d !== null} onOpenChange={props.onOpenChange}>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <SettingsIcon className="size-4" /> 域设置{d && ` · ${d.name}`}
          </DialogTitle>
          <DialogDescription>配置该域的行为；保存后立即生效。</DialogDescription>
        </DialogHeader>

        <div className="space-y-1.5">
          <div className="flex items-center justify-between gap-3 rounded-lg border bg-background px-3 py-2.5">
            <div className="min-w-0">
              <p className="text-sm font-medium">排行榜公开</p>
              <p className="text-xs leading-relaxed text-muted-foreground">
                关闭后普通成员无法查看本域排行榜，管理员不受影响
              </p>
            </div>
            <Switch
              checked={leaderboardPublic}
              disabled={busy}
              onCheckedChange={setLeaderboardPublic}
              className="shrink-0"
            />
          </div>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => props.onOpenChange(false)}>
            取消
          </Button>
          <Button onClick={() => void save()} disabled={busy}>
            {busy ? '保存中…' : '保存'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
