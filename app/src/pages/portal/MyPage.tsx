import { useState } from 'react'
import { useNavigate, useOutletContext } from 'react-router-dom'
import { KeyRoundIcon, LayoutGridIcon, LogOutIcon } from 'lucide-react'
import { toast } from 'sonner'

import type { ShellContext } from '@/app/App'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { PasswordDialog } from '@/components/PasswordDialog'
import { savedSpaceId } from '@/api/space'

export function roleLabel(role: string): string {
  switch (role) {
    case 'global_admin':
      return '系统管理员'
    case 'domain_admin':
      return '域管理员'
    case 'member':
      return '成员'
    default:
      return role
  }
}

// 我的页：账号信息 + 空间入口（返回空间/空间选择）+ 修改密码 + 退出。
export function MyPage() {
  const { user, onLogout } = useOutletContext<ShellContext>()
  const navigate = useNavigate()
  const [pwOpen, setPwOpen] = useState(false)
  const currentSpace = savedSpaceId()

  return (
    <div className="mx-auto w-full max-w-2xl px-4 py-6 lg:max-w-4xl lg:px-8 lg:py-8">
      <h1 className="mb-4 text-lg font-semibold">我的</h1>

      <div className="rounded-2xl border bg-card p-6">
        <div className="flex items-center gap-4">
          <div className="flex size-14 shrink-0 items-center justify-center rounded-full bg-primary text-xl font-semibold text-primary-foreground">
            {user.username.slice(0, 1).toUpperCase()}
          </div>
          <div className="min-w-0">
            <div className="flex items-center gap-2 font-medium">
              <span className="truncate">{user.username}</span>
              <Badge variant={user.role === 'member' ? 'secondary' : 'default'}>{roleLabel(user.role)}</Badge>
            </div>
            <div className="mt-1 text-xs text-muted-foreground">
              {user.role === 'domain_admin' && user.domainId !== undefined
                ? `归属域 #${user.domainId}`
                : user.role === 'global_admin'
                  ? '可进入全部域空间'
                  : '空间成员（做题账号）'}
            </div>
          </div>
        </div>

        <div className="mt-6 space-y-2.5">
          {currentSpace && (
            <Button className="w-full min-h-10 justify-start" onClick={() => navigate(`/s/${currentSpace}`)}>
              <LayoutGridIcon className="size-4" />
              回到我的空间
            </Button>
          )}
          <Button variant="outline" className="w-full min-h-10 justify-start" onClick={() => navigate('/')}>
            <LayoutGridIcon className="size-4" />
            空间列表 / 切换空间
          </Button>
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
