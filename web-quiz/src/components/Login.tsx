import { useState, type FormEvent } from 'react'
import { toast } from 'sonner'
import { Loader2Icon } from 'lucide-react'

import { api } from '@/lib/api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import type { User } from '@/lib/types'

export function Login({ onSuccess }: { onSuccess: (user: User) => void }) {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [busy, setBusy] = useState(false)

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    if (busy) return
    setBusy(true)
    try {
      await api.login(username.trim(), password)
      const d = await api.me()
      if (!d.authenticated || !d.user) throw new Error('登录状态异常')
      onSuccess(d.user)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '登录失败')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="flex min-h-dvh items-center justify-center bg-muted/30 px-4">
      <div className="w-full max-w-sm rounded-2xl border bg-background p-8 shadow-sm">
        <div className="mb-2 text-center">
          <img src="/favicon.png" alt="Orange 刷题" className="mx-auto mb-2 size-16 rounded-2xl" />
          <h1 className="text-xl font-semibold">Orange 刷题</h1>
          <p className="mt-1 text-xs text-muted-foreground">登录后开始刷题</p>
        </div>
        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="space-y-1.5">
            <Label htmlFor="username">用户名</Label>
            <Input id="username" value={username} onChange={(e) => setUsername(e.target.value)} placeholder="用户名" autoComplete="username" required />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="password">密码</Label>
            <Input id="password" type="password" value={password} onChange={(e) => setPassword(e.target.value)} placeholder="密码" autoComplete="current-password" required />
          </div>
          <Button type="submit" className="w-full min-h-10" disabled={busy}>
            {busy && <Loader2Icon className="size-4 animate-spin" />}
            登录
          </Button>
        </form>
      </div>
    </div>
  )
}