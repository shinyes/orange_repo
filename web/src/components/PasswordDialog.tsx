import { useState } from 'react'
import { toast } from 'sonner'
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

// 修改密码对话框。
export function PasswordDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (v: boolean) => void }) {
  const [oldPassword, setOld] = useState('')
  const [newPassword, setNew] = useState('')
  const [confirm, setConfirm] = useState('')
  const [busy, setBusy] = useState(false)

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

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle>修改密码</DialogTitle>
          <DialogDescription>修改后需要使用新密码重新登录。</DialogDescription>
        </DialogHeader>
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
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            取消
          </Button>
          <Button onClick={submit} disabled={busy}>
            保存
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
