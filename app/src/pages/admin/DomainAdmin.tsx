// 域管理（global_admin 全宽页，/admin/domains）：域 CRUD + 设/看域管理员。
// 迁移适配：返回按钮从 view.kind 状态机（goHome）改为 URL 导航回 /admin/problems。
import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { ArrowLeftIcon, Building2Icon, LoaderCircleIcon, PencilIcon, PlusIcon, ShieldCheckIcon, Trash2Icon, UsersIcon } from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { api, ApiError } from '@/api'
import { useDomain } from '@/pages/admin/domain-context'
import type { Domain } from '@/api/types'
import { ConfirmDialog } from './dialogs'

export function DomainAdmin() {
  const qc = useQueryClient()
  const navigate = useNavigate()
  const { domainId, setDomainId } = useDomain()

  const domainsQ = useQuery({ queryKey: ['admin', 'domains'], queryFn: api.domains })
  const domains = domainsQ.data?.domains ?? []

  const [creating, setCreating] = useState(false)
  const [renaming, setRenaming] = useState<Domain | null>(null)
  const [deleting, setDeleting] = useState<Domain | null>(null)
  const [forceDelete, setForceDelete] = useState(false)
  const [adminsOf, setAdminsOf] = useState<Domain | null>(null)

  const invalidate = () => {
    void qc.invalidateQueries({ queryKey: ['admin', 'domains'] })
    void qc.invalidateQueries({ queryKey: ['domains', 'select'] })
  }

  const del = useMutation({
    mutationFn: ({ id, force }: { id: number; force: boolean }) => api.deleteDomain(id, force),
    onSuccess: (_d, v) => {
      toast.success('域已删除')
      if (domainId === v.id) setDomainId(null)
      invalidate()
      setDeleting(null)
      setForceDelete(false)
    },
    onError: (e) => {
      if (e instanceof ApiError && e.status === 409) {
        setForceDelete(true)
        toast.error(`${e.message}。请勾选强制删除后重试。`)
      } else {
        toast.error(e instanceof Error ? e.message : '删除失败')
        setDeleting(null)
        setForceDelete(false)
      }
    },
  })

  return (
    <div className="h-full overflow-y-auto">
    <div className="mx-auto max-w-5xl px-6 py-6">
      <div className="mb-5 flex items-center gap-2">
        <Button variant="ghost" size="icon-sm" title="返回题目管理" onClick={() => navigate('/admin/problems')}>
          <ArrowLeftIcon />
        </Button>
        <h1 className="text-xl font-semibold">域管理</h1>
        <Badge variant="secondary" className="text-xs">系统管理员</Badge>
        <div className="ml-auto">
          <Button size="sm" onClick={() => setCreating(true)}>
            <PlusIcon data-icon="inline-start" /> 新建域
          </Button>
        </div>
      </div>

      <div className="overflow-hidden rounded-xl border">
        {domainsQ.isLoading ? (
          <div className="flex items-center justify-center gap-2 py-10 text-sm text-muted-foreground">
            <LoaderCircleIcon className="size-4 animate-spin" /> 加载中…
          </div>
        ) : domains.length === 0 ? (
          <div className="py-10 text-center text-sm text-muted-foreground">还没有域，点击右上角「新建域」创建。</div>
        ) : (
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b bg-muted/40 text-left text-xs text-muted-foreground">
                <th className="px-4 py-2 font-medium">名称</th>
                <th className="px-4 py-2 font-medium">ID</th>
                <th className="px-4 py-2 font-medium">创建时间</th>
                <th className="px-4 py-2 font-medium">操作</th>
              </tr>
            </thead>
            <tbody className="divide-y">
              {domains.map((d) => (
                <tr key={d.id} className="hover:bg-muted/30">
                  <td className="px-4 py-2.5">
                    <div className="flex items-center gap-2 font-medium">
                      <Building2Icon className="size-4 text-muted-foreground" />
                      {d.name}
                      {domainId === d.id && <Badge variant="secondary" className="text-[10px]">当前</Badge>}
                    </div>
                  </td>
                  <td className="px-4 py-2.5 tabular-nums text-muted-foreground">{d.id}</td>
                  <td className="px-4 py-2.5 text-muted-foreground">{new Date(d.createdAt).toLocaleString('zh-CN')}</td>
                  <td className="px-4 py-2.5">
                    <div className="flex items-center gap-1">
                      <Button size="xs" variant="ghost" onClick={() => setAdminsOf(d)}>
                        <ShieldCheckIcon data-icon="inline-start" /> 域管理员
                      </Button>
                      <Button size="xs" variant="ghost" onClick={() => setRenaming(d)}>
                        <PencilIcon data-icon="inline-start" /> 改名
                      </Button>
                      <Button
                        size="xs"
                        variant="ghost"
                        className="text-destructive"
                        onClick={() => {
                          setDeleting(d)
                          setForceDelete(false)
                        }}
                      >
                        <Trash2Icon data-icon="inline-start" /> 删除
                      </Button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      <p className="mt-3 text-xs text-muted-foreground">
        域内题目/标签/题册目录相互隔离。删除域将级联删除域内题目与空间（提示需强制时勾选确认）。
      </p>

      <CreateDomainDialog open={creating} onOpenChange={setCreating} onCreated={invalidate} />
      <RenameDomainDialog domain={renaming} onOpenChange={(v) => !v && setRenaming(null)} onDone={invalidate} />

      {deleting && (
        <ConfirmDialog
          open
          onOpenChange={(v) => !v && setDeleting(null)}
          title={`删除域「${deleting.name}」？`}
          description={
            forceDelete
              ? '该域内题目与空间将被一并删除，操作不可撤销！'
              : '域内题目与空间将一并删除；若域内仍有题目，后端会拒绝并提示需强制删除。'
          }
          confirmLabel={forceDelete ? '确认连同题目删除' : '删除'}
          onConfirm={() => del.mutate({ id: deleting.id, force: forceDelete })}
        />
      )}

      <DomainAdminsDialog domain={adminsOf} onOpenChange={(v) => !v && setAdminsOf(null)} />
    </div>
    </div>
  )
}

// ---------- 新建域 ----------

function CreateDomainDialog(props: { open: boolean; onOpenChange: (v: boolean) => void; onCreated: () => void }) {
  const [name, setName] = useState('')
  const [adminUsername, setAdminUsername] = useState('')
  const [adminPassword, setAdminPassword] = useState('')
  const [busy, setBusy] = useState(false)

  async function submit() {
    if (!name.trim()) {
      toast.error('请输入域名称')
      return
    }
    setBusy(true)
    try {
      await api.createDomain(name.trim(), adminUsername.trim() || undefined, adminPassword || undefined)
      toast.success(`域「${name.trim()}」已创建`)
      props.onCreated()
      props.onOpenChange(false)
      setName('')
      setAdminUsername('')
      setAdminPassword('')
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
          <DialogTitle>新建域</DialogTitle>
          <DialogDescription>创建后可在此域下建空间，并可选择设初始域管理员。</DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          <div className="space-y-1.5">
            <Label>域名称</Label>
            <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="例如：华东师大附中" autoFocus />
          </div>
          <div className="rounded-lg border border-dashed p-3">
            <div className="mb-2 text-xs font-medium text-muted-foreground">初始域管理员（可选）</div>
            <div className="space-y-2">
              <Input value={adminUsername} onChange={(e) => setAdminUsername(e.target.value)} placeholder="管理员用户名" />
              <Input
                type="password"
                value={adminPassword}
                onChange={(e) => setAdminPassword(e.target.value)}
                placeholder="密码（用户不存在时必填）"
              />
            </div>
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => props.onOpenChange(false)}>取消</Button>
          <Button onClick={() => void submit()} disabled={busy || !name.trim()}>
            {busy ? '创建中…' : '创建'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

// ---------- 改名 ----------

function RenameDomainDialog(props: { domain: Domain | null; onOpenChange: (v: boolean) => void; onDone: () => void }) {
  const [name, setName] = useState('')
  const [busy, setBusy] = useState(false)
  const d = props.domain
  // 打开时预填当前名称（简单同步：每次打开重置）
  const [lastDomainId, setLastDomainId] = useState<number | null>(null)
  if (d && lastDomainId !== d.id) {
    setLastDomainId(d.id)
    setName(d.name)
  } else if (!d && lastDomainId !== null) {
    setLastDomainId(null)
  }

  async function save() {
    if (!d || !name.trim()) return
    setBusy(true)
    try {
      await api.renameDomain(d.id, name.trim())
      toast.success('域已重命名')
      props.onDone()
      props.onOpenChange(false)
    } catch (e) {
      toast.error(e instanceof Error ? e.message : '重命名失败')
    } finally {
      setBusy(false)
    }
  }

  return (
    <Dialog open={d !== null} onOpenChange={props.onOpenChange}>
      <DialogContent className="sm:max-w-xs">
        <DialogHeader>
          <DialogTitle>重命名域</DialogTitle>
        </DialogHeader>
        <Input
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="新名称"
          autoFocus
          onKeyDown={(e) => e.key === 'Enter' && void save()}
        />
        <DialogFooter>
          <Button variant="outline" onClick={() => props.onOpenChange(false)}>取消</Button>
          <Button onClick={() => void save()} disabled={!name.trim() || busy}>
            {busy ? '保存中…' : '保存'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

// ---------- 域管理员菜单（从现有账号中设置；注册账号请到「用户管理」） ----------

function DomainAdminsDialog(props: { domain: Domain | null; onOpenChange: (v: boolean) => void }) {
  const d = props.domain
  const qc = useQueryClient()
  const [selectedId, setSelectedId] = useState<number | null>(null)
  const [busy, setBusy] = useState(false)
  const [removing, setRemoving] = useState<number | null>(null)

  const adminsQ = useQuery({
    queryKey: ['admin', 'domains', d?.id, 'admins'],
    queryFn: () => api.domainAdmins(d!.id),
    enabled: d != null,
  })
  const admins = adminsQ.data?.admins ?? []
  const adminIds = new Set(admins.map((a) => a.id))

  // 候选：现有普通成员账号（注册请到用户管理页）
  const allQ = useQuery({
    queryKey: ['admin', 'all-users'],
    queryFn: () => api.allUsers(),
    enabled: d != null,
  })
  const candidates = (allQ.data?.users ?? []).filter((u) => u.role === 'member' && !adminIds.has(u.id))

  async function add() {
    if (!d) return
    const user = (allQ.data?.users ?? []).find((u) => u.id === selectedId)
    if (!user) {
      toast.error('请选择要设为域管理员的账号')
      return
    }
    setBusy(true)
    try {
      await api.setDomainAdmin(d.id, user.username)
      toast.success(`${user.username} 已是该域管理员`)
      setSelectedId(null)
      await qc.invalidateQueries({ queryKey: ['admin', 'domains', d.id, 'admins'] })
    } catch (e) {
      toast.error(e instanceof Error ? e.message : '设置失败')
    } finally {
      setBusy(false)
    }
  }

  async function remove(uid: number, uname: string) {
    if (!d) return
    setRemoving(uid)
    try {
      await api.removeDomainAdmin(d.id, uid)
      toast.success(`已将 ${uname} 移出域管理员（账号保留为普通成员）`)
      await qc.invalidateQueries({ queryKey: ['admin', 'domains', d.id, 'admins'] })
    } catch (e) {
      toast.error(e instanceof Error ? e.message : '移除失败')
    } finally {
      setRemoving(null)
    }
  }

  return (
    <Dialog open={d !== null} onOpenChange={props.onOpenChange}>
      <DialogContent className="sm:max-w-sm">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <ShieldCheckIcon className="size-4" /> 域管理员{d && ` · ${d.name}`}
          </DialogTitle>
          <DialogDescription>管理该域的域管理员；移除后账号保留为普通成员。</DialogDescription>
        </DialogHeader>

        <div className="space-y-2">
          <Label>现有域管理员</Label>
          {admins.length === 0 ? (
            <div className="rounded-lg border border-dashed px-3 py-2 text-xs text-muted-foreground">暂无域管理员，请从下方成员中设置</div>
          ) : (
            <ul className="divide-y rounded-lg border">
              {admins.map((a) => (
                <li key={a.id} className="flex items-center gap-2 px-3 py-1.5 text-sm">
                  <UsersIcon className="size-3.5 shrink-0 text-muted-foreground" />
                  <span className="min-w-0 flex-1 truncate">{a.username}</span>
                  <span className="text-[10px] text-muted-foreground">#{a.id}</span>
                  <Button
                    size="icon-xs"
                    variant="ghost"
                    className="text-destructive"
                    title="移除域管理员（账号保留为普通成员）"
                    disabled={removing === a.id}
                    onClick={() => void remove(a.id, a.username)}
                  >
                    {removing === a.id ? <LoaderCircleIcon className="size-3.5 animate-spin" /> : <Trash2Icon className="size-3.5" />}
                  </Button>
                </li>
              ))}
            </ul>
          )}
        </div>

        <div className="space-y-1.5 border-t pt-3">
          <Label>设为域管理员（从现有成员中选择）</Label>
          <div className="flex gap-2">
            <Select
              value={selectedId != null ? String(selectedId) : ''}
              onValueChange={(v) => setSelectedId(Number(v))}
            >
              <SelectTrigger className="flex-1">
                <SelectValue placeholder={candidates.length === 0 ? '暂无可选的成员账号' : '选择成员账号'} />
              </SelectTrigger>
              <SelectContent>
                {candidates.map((u) => (
                  <SelectItem key={u.id} value={String(u.id)}>
                    {u.username}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Button onClick={() => void add()} disabled={busy || selectedId == null} className="shrink-0">
              {busy ? '提交中…' : '设置'}
            </Button>
          </div>
          {candidates.length === 0 && (
            <p className="text-xs text-muted-foreground">
              没有可选的普通成员账号——请先到「用户管理」页新建成员账号。
            </p>
          )}
          <p className="text-xs text-muted-foreground">
            已是系统管理员/其他域管理员的账号不在此列出；如需更换请先移除现任者。
          </p>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => props.onOpenChange(false)}>关闭</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
