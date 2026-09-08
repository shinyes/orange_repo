import { useState } from 'react'
import { Link } from 'react-router-dom'
import { ClipboardListIcon, EyeIcon, PencilIcon, PlusIcon, Trash2Icon } from 'lucide-react'
import { toast } from 'sonner'

import type { PracticeBrief } from '@/api/types'
import { api } from '@/api'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { NewPracticeDialog, VisibleUsersDialog } from '@/components/portal/space-item-dialogs'
import { PracticeQuickEditDialog } from '@/components/portal/PracticeQuickEditDialog'
import { ConfirmDeleteDialog } from '@/components/portal/confirm-delete-dialog'
import { useSpaceHome } from './useSpaceHome'
import { usePortalCtx } from './SpaceShell'
import { usePortalSession } from './portal-context'

// 练习列表（空间壳内 tab 页）：整卷交卷式练习卡片，点击进入独立作答页。
// 管理员：顶部可新建练习、卡片 hover 显示 删除/可见成员/快速编辑（标题/描述/题目）。
export function PracticeList() {
  const { space } = usePortalCtx()
  const home = useSpaceHome(space)
  const list = home.data?.practices ?? []
  const { user } = usePortalSession()
  const canEdit = user.role !== 'member'
  const [creating, setCreating] = useState(false)
  const [editing, setEditing] = useState<PracticeBrief | null>(null)
  const [visibleFor, setVisibleFor] = useState<PracticeBrief | null>(null)
  const [deleting, setDeleting] = useState<PracticeBrief | null>(null)
  const [deleteBusy, setDeleteBusy] = useState(false)

  async function remove(p: PracticeBrief) {
    setDeleteBusy(true)
    try {
      await api.deleteSpacePractice(space.id, p.id)
      toast.success('已删除')
      setDeleting(null)
      void home.refetch()
    } catch (e) {
      toast.error(e instanceof Error ? e.message : '删除失败')
    } finally {
      setDeleteBusy(false)
    }
  }

  return (
    <div className="mx-auto w-full max-w-3xl px-4 py-6 lg:px-8">
      <div className="mb-1 flex flex-wrap items-center justify-between gap-2">
        <h1 className="text-lg font-semibold">练习</h1>
        {canEdit && (
          <Button onClick={() => setCreating(true)} className="h-8 text-xs">
            <PlusIcon data-icon="inline-start" /> 新建练习
          </Button>
        )}
      </div>
      <p className="mb-4 text-xs text-muted-foreground">整卷作答后统一交卷评分，可重复交卷（每次记录）</p>
      {home.isLoading && <p className="py-10 text-center text-sm text-muted-foreground">加载中…</p>}
      {home.isError && (
        <div className="py-10 text-center text-sm text-red-500">
          加载失败{home.error instanceof Error ? `：${home.error.message}` : ''}
          <button type="button" className="ml-2 text-primary underline" onClick={() => void home.refetch()}>重试</button>
        </div>
      )}
      <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
        {list.map((p) => (
          <PracticeCard
            key={p.id}
            p={p}
            spaceId={space.id}
            canEdit={canEdit}
            onEdit={() => setEditing(p)}
            onVisible={() => setVisibleFor(p)}
            onDelete={() => setDeleting(p)}
          />
        ))}
      </div>
      {!home.isLoading && list.length === 0 && (
        <div className="rounded-xl border border-dashed p-12 text-center text-sm text-muted-foreground">
          本空间暂无练习
        </div>
      )}

      <NewPracticeDialog spaceId={space.id} open={creating} onOpenChange={setCreating} onCreated={() => void home.refetch()} />

      {editing && (
        <PracticeQuickEditDialog
          spaceId={space.id}
          practiceId={editing.id}
          practiceTitle={editing.title}
          open
          onOpenChange={(v) => !v && setEditing(null)}
          onChanged={() => void home.refetch()}
        />
      )}

      {visibleFor && (
        <VisibleUsersDialog
          spaceId={space.id}
          kind="practice"
          itemId={visibleFor.id}
          title={visibleFor.title}
          open
          onClose={() => setVisibleFor(null)}
          onSaved={() => void home.refetch()}
        />
      )}

      <ConfirmDeleteDialog
        open={deleting !== null}
        title={deleting ? `删除练习「${deleting.title}」？` : '删除练习'}
        description="删除后不可恢复，成员将无法再进入该练习。"
        busy={deleteBusy}
        onOpenChange={(v) => { if (!v && !deleteBusy) setDeleting(null) }}
        onConfirm={() => { if (deleting) return remove(deleting) }}
      />
    </div>
  )
}

function PracticeCard({ p, spaceId, canEdit, onEdit, onVisible, onDelete }: {
  p: PracticeBrief
  spaceId: number
  canEdit: boolean
  onEdit: () => void
  onVisible: () => void
  onDelete: () => void
}) {
  return (
    <div className="group relative rounded-2xl border bg-card transition-colors hover:border-primary/50">
      {canEdit && (
        <div className="absolute top-2 right-2 z-10 flex items-center gap-1">
          <button
            type="button"
            title="删除练习"
            onClick={onDelete}
            className="flex size-7 items-center justify-center rounded-md border bg-background text-muted-foreground opacity-0 transition-opacity hover:border-red-400/40 hover:text-red-500 focus:opacity-100 group-hover:opacity-100"
          >
            <Trash2Icon className="size-3.5" />
          </button>
          <button
            type="button"
            title="设置可见成员（默认无成员可见）"
            onClick={onVisible}
            className="flex size-7 items-center justify-center rounded-md border bg-background text-muted-foreground opacity-0 transition-opacity hover:border-primary/40 hover:text-primary focus:opacity-100 group-hover:opacity-100"
          >
            <EyeIcon className="size-3.5" />
          </button>
          <button
            type="button"
            title="快速编辑练习（标题/描述/题目）"
            onClick={onEdit}
            className="flex size-7 items-center justify-center rounded-md border bg-background text-muted-foreground opacity-0 transition-opacity hover:border-primary/40 hover:text-primary focus:opacity-100 group-hover:opacity-100"
          >
            <PencilIcon className="size-3.5" />
          </button>
        </div>
      )}
      <Link
        to={`/s/${spaceId}/practice/${p.id}`}
        className="flex w-full flex-col gap-1.5 rounded-2xl p-4 pr-24"
      >
        <span className="flex min-w-0 items-center gap-2 font-medium">
          <ClipboardListIcon className="size-4 shrink-0 text-primary" />
          <span className="min-w-0 truncate">{p.title}</span>
          {p.isPublic && <Badge variant="secondary" className="shrink-0 px-1.5 text-[10px] font-normal text-emerald-600">公开</Badge>}
        </span>
        {p.description && <p className="line-clamp-2 text-xs text-muted-foreground">{p.description}</p>}
        <span className="mt-1 flex items-center justify-between text-xs text-muted-foreground">
          <span>{p.problemCount} 题</span>
          <span>整卷交卷 · 可重做</span>
        </span>
      </Link>
    </div>
  )
}
