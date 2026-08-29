import { useState } from 'react'
import { toast } from 'sonner'
import { ImageIcon, TrashIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { api } from '@/lib/api'

// 设置对话框：修改密码 + 清理未被引用的图片。
export function PasswordDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (v: boolean) => void }) {
  const [oldPassword, setOld] = useState('')
  const [newPassword, setNew] = useState('')
  const [confirm, setConfirm] = useState('')
  const [busy, setBusy] = useState(false)
  // 图片清理
  const [imageInfo, setImageInfo] = useState<{ orphaned: number; total: number } | null>(null)
  const [scanning, setScanning] = useState(false)
  const [cleaning, setCleaning] = useState(false)
  const [confirmClean, setConfirmClean] = useState(false)

  async function scan() {
    setScanning(true)
    try {
      setImageInfo(await api.scanOrphanImages())
      setConfirmClean(false)
    } catch (e) {
      toast.error(e instanceof Error ? e.message : '扫描失败')
    } finally {
      setScanning(false)
    }
  }

  async function cleanup() {
    setCleaning(true)
    try {
      const { removed } = await api.cleanupOrphanImages()
      toast.success(`已清理 ${removed} 张未关联图片`)
      setConfirmClean(false)
      setImageInfo((prev) => (prev ? { ...prev, orphaned: prev.orphaned - removed, total: prev.total - removed } : prev))
    } catch (e) {
      toast.error(e instanceof Error ? e.message : '清理失败')
    } finally {
      setCleaning(false)
    }
  }

  async function submit() {
    if (!newPassword) {
      toast.error('新密码不能为空')
      return
    }
    if (newPassword !== confirm) {
      toast.error('两次输入的新密码不一致')
      return
    }
    setBusy(true)
    try {
      await api.changePassword(oldPassword, newPassword)
      toast.success('密码已修改')
      onOpenChange(false)
      setOld('')
      setNew('')
      setConfirm('')
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '修改失败')
    } finally {
      setBusy(false)
    }
  }

  async function openChanged(v: boolean) {
    if (v) setImageInfo(null)
    onOpenChange(v)
  }

  return (
    <Dialog open={open} onOpenChange={openChanged}>
      <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-sm">
        <DialogHeader>
          <DialogTitle>设置</DialogTitle>
          <DialogDescription>密码与会话管理、清理未被题目引用的图片。</DialogDescription>
        </DialogHeader>

        {/* 修改密码 */}
        <div className="space-y-1.5">
          <Label className="text-sm font-medium">修改密码</Label>
          <div className="space-y-3">
            <div className="space-y-1.5">
              <Label>当前密码</Label>
              <Input type="password" value={oldPassword} onChange={(e) => setOld(e.target.value)} />
            </div>
            <div className="space-y-1.5">
              <Label>新密码</Label>
              <Input type="password" value={newPassword} onChange={(e) => setNew(e.target.value)} />
            </div>
            <div className="space-y-1.5">
              <Label>确认新密码</Label>
              <Input type="password" value={confirm} onChange={(e) => setConfirm(e.target.value)} />
            </div>
            <Button onClick={submit} disabled={busy} className="w-full">
              保存密码
            </Button>
          </div>
        </div>

        {/* 图片清理 */}
        <div className="space-y-1.5 border-t pt-3">
          <Label className="text-sm font-medium">清理图片</Label>
          <p className="text-xs text-muted-foreground">
            删除未被任何题目引用的上传图片（题面/答案/题解中仍在使用的图片会自动保留）。
          </p>
          {imageInfo === null ? (
            <Button variant="outline" size="sm" onClick={() => void scan()} disabled={scanning}>
              <ImageIcon data-icon="inline-start" /> {scanning ? '扫描中…' : '扫描未关联图片'}
            </Button>
          ) : (
            <div className="space-y-2">
              <div className="text-xs">
                共 {imageInfo.total} 张图片，其中 <span className="font-medium text-destructive">{imageInfo.orphaned}</span> 张未被引用。
              </div>
              {!confirmClean ? (
                <div className="flex gap-1.5">
                  <Button size="xs" variant="outline" onClick={() => void scan()} disabled={scanning}>
                    重新扫描
                  </Button>
                  <Button size="xs" variant="outline" className="text-destructive" disabled={imageInfo.orphaned === 0 || cleaning} onClick={() => setConfirmClean(true)}>
                    <TrashIcon data-icon="inline-start" /> 清理
                  </Button>
                </div>
              ) : (
                <div className="rounded-lg border border-destructive/40 bg-destructive/5 p-2">
                  <p className="text-xs">确认删除 {imageInfo.orphaned} 张未关联图片？此操作不可撤销。</p>
                  <div className="mt-1.5 flex gap-1.5">
                    <Button size="xs" variant="outline" onClick={() => setConfirmClean(false)}>
                      取消
                    </Button>
                    <Button size="xs" variant="outline" className="text-destructive" onClick={() => void cleanup()} disabled={cleaning}>
                      {cleaning ? '清理中…' : '确认删除'}
                    </Button>
                  </div>
                </div>
              )}
            </div>
          )}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            关闭
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
