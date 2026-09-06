// 集中用户管理（/admin/users，仅系统管理员）：全部账号（成员/域管理员/系统管理员）
// 的列表与维护——新建账号、设/取消域管理员、重置密码、删除普通成员。
import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import {
  ArrowLeftIcon,
  KeyRoundIcon,
  PlusIcon,
  ShieldCheckIcon,
  Trash2Icon,
  UserCogIcon,
  UsersIcon,
} from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { api } from '@/api'
import type { AllUser } from '@/api/types'
import { ConfirmDialog } from './dialogs'

function roleBadge(u: AllUser): { label: string; cls: string } {
  switch (u.role) {
    case 'global_admin':
      return { label: '系统管理员', cls: 'bg-amber-100 text-amber-700' }
    case 'domain_admin':
      return { label: '域管理员', cls: 'bg-sky-100 text-sky-700' }
    default:
      return { label: '成员', cls: 'bg-muted text-muted-foreground' }
  }
}

export function UsersAdmin() {
  const navigate = useNavigate()
  const qc = useQueryClient()
  const [createOpen, setCreateOpen] = useState(false)
  const [resetTarget, setResetTarget] = useState<AllUser | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<AllUser | null>(null)
  const [promote, setPromote] = useState<AllUser | null>(null)

  const usersQ = useQuery({ queryKey: ['admin', 'all-users'], queryFn: () => api.allUsers() })
  const domainsQ = useQuery({ queryKey: ['admin', 'domains'], queryFn: () => api.domains() })
  const users = usersQ.data?.users ?? []
  const domains = domainsQ.data?.domains ?? []

  async function invalidate() {
    await qc.invalidateQueries({ queryKey: ['admin', 'all-users'] })
    await qc.invalidateQueries({ queryKey: ['admin', 'domains'] })
  }

  return (
    <div className="h-full overflow-y-auto">
      <div className="mx-auto max-w-4xl px-6 py-6">
        <div className="mb-5 flex items-center gap-2">
          <Button variant="ghost" size="icon-sm" title="返回题目管理" onClick={() => navigate('/admin/problems')}>
            <ArrowLeftIcon />
          </Button>
          <h1 className="text-xl font-semibold">用户管理</h1>
          <Badge variant="secondary" className="text-xs">共 {users.length} 个账号</Badge>
          <div className="ml-auto">
            <Button size="sm" onClick={() => setCreateOpen(true)}>
              <PlusIcon data-icon="inline-start" /> 新建账号
            </Button>
          </div>
        </div>

        {/* 账号列表 */}
        <div className="overflow-hidden rounded-xl border">
          <table className="w-full text-sm">
            <thead className="bg-muted/50 text-left text-xs text-muted-foreground">
              <tr>
                <th className="px-3 py-2 font-medium">用户</th>
                <th className="px-3 py-2 font-medium">角色</th>
                <th className="px-3 py-2 font-medium">所属域</th>
                <th className="px-3 py-2 text-right font-medium">操作</th>
              </tr>
            </thead>
            <tbody className="divide-y">
              {users.map((u) => {
                const rb = roleBadge(u)
                const domainName = u.domainName ?? (u.domainId != null ? `域 #${u.domainId}` : '')
                return (
                  <tr key={u.id} className="hover:bg-muted/30">
                    <td className="px-3 py-2">
                      <span className="flex items-center gap-2">
                        <UsersIcon className="size-3.5 text-muted-foreground" />
                        <span className="font-medium">{u.username}</span>
                        <span className="text-[10px] text-muted-foreground">#{u.id}</span>
                      </span>
                    </td>
                    <td className="px-3 py-2">
                      <span className={`rounded px-1.5 py-0.5 text-[11px] ${rb.cls}`}>{rb.label}</span>
                    </td>
                    <td className="px-3 py-2 text-xs text-muted-foreground">{domainName || '—'}</td>
                    <td className="px-3 py-2">
                      <div className="flex justify-end gap-1">
                        {u.role === 'member' && (
                          <Button size="xs" variant="outline" onClick={() => setPromote(u)} title="设为某域的域管理员">
                            <ShieldCheckIcon data-icon="inline-start" /> 设为域管理员
                          </Button>
                        )}
                        {(u.role === 'domain_admin' || u.role === 'member') && (
                          <Button size="xs" variant="outline" onClick={() => setResetTarget(u)} title="重置密码">
                            <KeyRoundIcon data-icon="inline-start" /> 重置密码
                          </Button>
                        )}
                        {u.role === 'member' && (
                          <Button
                            size="icon-xs"
                            variant="ghost"
                            className="text-destructive"
                            title="删除账号"
                            onClick={() => setDeleteTarget(u)}
                          >
                            <Trash2Icon className="size-3.5" />
                          </Button>
                        )}
                        {u.role === 'domain_admin' && (
                          <Button
                            size="icon-xs"
                            variant="ghost"
                            className="text-destructive"
                            title="取消域管理员（保留为普通成员）"
                            disabled={!u.domainId}
                            onClick={() => {
                              if (u.domainId == null) return
                              void api
                                .removeDomainAdmin(u.domainId, u.id)
                                .then(() => {
                                  toast.success(`已将 ${u.username} 取消域管理员`)
                                  return invalidate()
                                })
                                .catch((e) => toast.error(e instanceof Error ? e.message : '操作失败'))
                            }}
                          >
                            <UserCogIcon className="size-3.5" />
                          </Button>
                        )}
                      </div>
                    </td>
                  </tr>
                )
              })}
              {users.length === 0 && (
                <tr>
                  <td colSpan={4} className="px-3 py-8 text-center text-sm text-muted-foreground">
                    暂无账号
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>

        {/* 提示：角色规则 */}
        <p className="mt-3 text-xs leading-relaxed text-muted-foreground">
          规则：成员（member）加入空间后做题；域管理员管理所属域全部仓库与空间；系统管理员管理全部域与账号。
          已是系统管理员/其他域管理员的账号无法直接改设（请先在目标域移除或使用成员账号提升）。
        </p>
      </div>

      {/* 新建账号对话框 */}
      <CreateUserDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        domains={domains}
        onSaved={() => invalidate()}
      />

      {/* 提升为域管理员（选域） */}
      <PromoteDialog
        user={promote}
        onClose={() => setPromote(null)}
        domains={domains}
        onDone={() => invalidate()}
      />

      {/* 重置密码 */}
      <ResetPasswordDialog user={resetTarget} onClose={() => setResetTarget(null)} onDone={() => invalidate()} />

      {/* 删除成员确认 */}
      <ConfirmDialog
        open={deleteTarget != null}
        onOpenChange={() => setDeleteTarget(null)}
        title="删除账号"
        description={`确定删除成员 ${deleteTarget?.username ?? ''}？其会话/作答关联将一并清除，不可恢复。`}
        confirmLabel="删除"
        onConfirm={() => {
          const target = deleteTarget
          if (!target) return
          void api
            .deleteUser(target.id)
            .then(async () => {
              toast.success('账号已删除')
              setDeleteTarget(null)
              await invalidate()
            })
            .catch((e) => toast.error(e instanceof Error ? e.message : '删除失败'))
        }}
      />
    </div>
  )
}

// ---------- 新建账号（成员 或 域管理员） ----------

function CreateUserDialog(props: {
  open: boolean
  onOpenChange: (v: boolean) => void
  domains: { id: number; name: string }[]
  onSaved: () => Promise<void>
}) {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [asDomainAdmin, setAsDomainAdmin] = useState(false)
  const [domainId, setDomainId] = useState<number | null>(null)
  const [busy, setBusy] = useState(false)

  async function submit() {
    if (!username.trim() || !password) {
      toast.error('用户名与密码必填')
      return
    }
    if (asDomainAdmin && domainId == null) {
      toast.error('请选择归属域')
      return
    }
    setBusy(true)
    try {
      if (asDomainAdmin && domainId != null) {
        // 直接建域管理员：先建成员再升级？后端 setDomainAdmin 支持用户不存在+password → 直接建
        await api.setDomainAdmin(domainId, username.trim(), password)
        toast.success('域管理员账号已创建')
      } else {
        await api.createUser(username.trim(), password)
        toast.success('成员账号已创建')
      }
      setUsername('')
      setPassword('')
      setAsDomainAdmin(false)
      setDomainId(null)
      props.onOpenChange(false)
      await props.onSaved()
    } catch (e) {
      toast.error(e instanceof Error ? e.message : '创建失败')
    } finally {
      setBusy(false)
    }
  }

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle>新建账号</DialogTitle>
          <DialogDescription>可创建普通成员或指定域的域管理员。</DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          <div className="space-y-1.5">
            <Label>用户名</Label>
            <Input value={username} onChange={(e) => setUsername(e.target.value)} placeholder="1-32 字符" />
          </div>
          <div className="space-y-1.5">
            <Label>初始密码</Label>
            <Input type="password" value={password} onChange={(e) => setPassword(e.target.value)} placeholder="登录后可在「我的」修改" />
          </div>
          <label className="flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              className="size-4"
              checked={asDomainAdmin}
              onChange={(e) => {
                setAsDomainAdmin(e.target.checked)
                if (!e.target.checked) setDomainId(null)
              }}
            />
            同时设为域管理员
          </label>
          {asDomainAdmin && (
            <div className="space-y-1.5">
              <Label>归属域</Label>
              <Select
                value={domainId != null ? String(domainId) : ''}
                onValueChange={(v) => setDomainId(Number(v))}
              >
                <SelectTrigger>
                  <SelectValue placeholder="选择域" />
                </SelectTrigger>
                <SelectContent>
                  {props.domains.map((d) => (
                    <SelectItem key={d.id} value={String(d.id)}>
                      {d.name} #{d.id}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          )}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => props.onOpenChange(false)}>
            取消
          </Button>
          <Button onClick={() => void submit()} disabled={busy}>
            {busy ? '创建中…' : '创建'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

// ---------- 提升已有成员为域管理员（选域） ----------

function PromoteDialog(props: {
  user: AllUser | null
  onClose: () => void
  domains: { id: number; name: string }[]
  onDone: () => Promise<void>
}) {
  const [domainId, setDomainId] = useState<number | null>(null)
  const [busy, setBusy] = useState(false)

  async function submit() {
    if (!props.user) return
    if (domainId == null) {
      toast.error('请选择归属域')
      return
    }
    setBusy(true)
    try {
      await api.setDomainAdmin(domainId, props.user.username)
      toast.success(`${props.user.username} 已是该域管理员`)
      props.onClose()
      await props.onDone()
    } catch (e) {
      toast.error(e instanceof Error ? e.message : '设置失败')
    } finally {
      setBusy(false)
    }
  }

  return (
    <Dialog open={props.user != null} onOpenChange={() => props.onClose()}>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle>设为域管理员</DialogTitle>
          <DialogDescription>
            将 {props.user?.username ?? ''}（成员）提升为指定域的域管理员；其将管理该域仓库与全部空间。
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-1.5">
          <Label>归属域</Label>
          <Select
            value={domainId != null ? String(domainId) : ''}
            onValueChange={(v) => setDomainId(Number(v))}
          >
            <SelectTrigger>
              <SelectValue placeholder="选择域" />
            </SelectTrigger>
            <SelectContent>
              {props.domains.map((d) => (
                <SelectItem key={d.id} value={String(d.id)}>
                  {d.name} #{d.id}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={props.onClose}>
            取消
          </Button>
          <Button onClick={() => void submit()} disabled={busy}>
            {busy ? '提交中…' : '设为域管理员'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

// ---------- 重置密码 ----------

function ResetPasswordDialog(props: { user: AllUser | null; onClose: () => void; onDone: () => Promise<void> }) {
  const [password, setPassword] = useState('')
  const [busy, setBusy] = useState(false)

  async function submit() {
    if (!props.user) return
    if (!password) {
      toast.error('请输入新密码')
      return
    }
    setBusy(true)
    try {
      await api.resetUserPassword(props.user.id, password)
      toast.success('密码已重置（该账号全部会话已失效）')
      props.onClose()
      setPassword('')
      await props.onDone()
    } catch (e) {
      toast.error(e instanceof Error ? e.message : '重置失败')
    } finally {
      setBusy(false)
    }
  }

  return (
    <Dialog open={props.user != null} onOpenChange={() => props.onClose()}>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle>重置密码</DialogTitle>
          <DialogDescription>为 {props.user?.username ?? ''} 设置新密码（无需旧密码）。</DialogDescription>
        </DialogHeader>
        <div className="space-y-1.5">
          <Label>新密码</Label>
          <Input type="password" value={password} onChange={(e) => setPassword(e.target.value)} />
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={props.onClose}>
            取消
          </Button>
          <Button onClick={() => void submit()} disabled={busy}>
            {busy ? '提交中…' : '确认重置'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
