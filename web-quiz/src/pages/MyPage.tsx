import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { KeyRoundIcon, LogOutIcon, SettingsIcon } from 'lucide-react'
import { toast } from 'sonner'

import { api } from '@/lib/api'
import type { User } from '@/lib/types'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { PasswordDialog } from '@/components/PasswordDialog'

// 我的页：个人信息 + 修改密码 + 退出；管理员含「系统管理」入口。
export function MyPage({ user, onOpenAdmin, onLogout }: { user: User; onOpenAdmin: () => void; onLogout: () => void }) {
  const [pwOpen, setPwOpen] = useState(false)
  const wrong = useQuery({ queryKey: ['wrong-summary'], queryFn: api.wrongSummary })

  return (
    <div className="mx-auto w-full max-w-2xl px-4 py-6">
      <h1 className="mb-4 text-lg font-semibold">我的</h1>

      <div className="rounded-2xl border bg-card p-6">
        <div className="flex items-center gap-4">
          <div className="flex size-14 shrink-0 items-center justify-center rounded-full bg-primary text-xl font-semibold text-primary-foreground">
            {user.username.slice(0, 1).toUpperCase()}
          </div>
          <div className="min-w-0">
            <div className="flex items-center gap-2 font-medium">
              <span className="truncate">{user.username}</span>
              <Badge variant={user.role === 'admin' ? 'default' : 'secondary'}>
                {user.role === 'admin' ? '管理员' : '学生'}
              </Badge>
            </div>
            <div className="mt-1 text-xs text-muted-foreground">
              错题 {wrong.data?.total ?? '…'} 题
            </div>
          </div>
        </div>

        <div className="mt-6 space-y-2.5">
          {user.role === 'admin' && (
            <Button className="w-full min-h-10 justify-start" onClick={onOpenAdmin}>
              <SettingsIcon className="size-4" />
              系统管理
            </Button>
          )}
          <Button variant="outline" className="w-full min-h-10 justify-start" onClick={() => setPwOpen(true)}>
            <KeyRoundIcon className="size-4" />
            修改密码
          </Button>
          <Button
            variant="ghost"
            className="w-full min-h-10 justify-start text-red-600 hover:bg-red-50 hover:text-red-600"
            onClick={() => {
              onLogout()
              toast('已退出登录')
            }}
          >
            <LogOutIcon className="size-4" />
            退出登录
          </Button>
        </div>
      </div>

      <PasswordDialog open={pwOpen} onOpenChange={setPwOpen} />
    </div>
  )
}