import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { api, ApiError } from '@/lib/api'

// 登录页：统一账号库（与刷题服务共享），仅管理员可登录主站。
export function Login({ onSuccess }: { onSuccess: () => void }) {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError('')
    try {
      await api.login(username.trim(), password)
      onSuccess()
    } catch (err) {
      setError(err instanceof ApiError && err.status === 401 ? '用户名或密码错误' : '登录失败，请重试')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="flex h-screen items-center justify-center bg-muted/40">
      <form onSubmit={submit} className="w-full max-w-sm rounded-xl border bg-background p-6 shadow-sm">
        <div className="mb-6 text-center">
          <img src="/favicon.png" alt="OrangeRepo" className="mx-auto mb-3 size-14 rounded-xl" />
          <h1 className="text-lg font-semibold">OrangeRepo</h1>
          <p className="mt-1 text-xs text-muted-foreground">兼容 OrangeOJ 格式的题目仓库管理工具</p>
        </div>
        <div className="space-y-2">
          <Label htmlFor="username">用户名</Label>
          <Input
            id="username"
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            placeholder="默认 admin"
            autoFocus
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="password">密码</Label>
          <Input
            id="password"
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            placeholder="默认密码 123456"
          />
        </div>
        {error && <p className="mt-2 text-sm text-destructive">{error}</p>}
        <Button type="submit" className="mt-4 w-full" disabled={busy || !password || !username.trim()}>
          {busy ? '登录中…' : '进入题库'}
        </Button>
      </form>
    </div>
  )
}
