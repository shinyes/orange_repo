import { useState } from 'react'
import { Link } from 'react-router-dom'
import { ClipboardListIcon, EyeIcon, PencilIcon, PlusIcon } from 'lucide-react'

import type { PracticeBrief } from '@/api/types'
import { Button } from '@/components/ui/button'
import { NewPracticeDialog, VisibleUsersDialog } from '@/components/portal/space-item-dialogs'
import { PracticeQuickEditDialog } from '@/components/portal/PracticeQuickEditDialog'
import { useSpaceHome } from './useSpaceHome'
import { usePortalCtx } from './SpaceShell'
import { usePortalSession } from './portal-context'

// 练习列表（空间壳内 tab 页）：整卷交卷式练习卡片，点击进入独立作答页。
// 管理员：顶部可新建练习、卡片 ✎ 快速编辑（标题/描述/题目）+ 眼睛分配可见成员。
export function PracticeList() {
  const { space } = usePortalCtx()
  const home = useSpaceHome(space)
  const list = home.data?.practices ?? []
  const { user } = usePortalSession()
  const canEdit = user.role !== 'member'
  const [creating, setCreating] = useState(false)
  const [editing, setEditing] = useState<PracticeBrief | null>(null)
  const [visibleFor, setVisibleFor] = useState<PracticeBrief | null>(null)

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
      <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
        {list.map((p) => (
          <PracticeCard
            key={p.id}
            p={p}
            spaceId={space.id}
            canEdit={canEdit}
            onEdit={() => setEditing(p)}
            onVisible={() => setVisibleFor(p)}
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
    </div>
  )
}

function PracticeCard({ p, spaceId, canEdit, onEdit, onVisible }: {
  p: PracticeBrief
  spaceId: number
  canEdit: boolean
  onEdit: () => void
  onVisible: () => void
}) {
  return (
    <div className="group relative rounded-2xl border bg-card transition-colors hover:border-primary/50">
      {canEdit && (
        <div className="absolute top-2 right-2 z-10 flex items-center gap-1">
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
        className="flex w-full flex-col gap-1.5 rounded-2xl p-4"
      >
        <span className="flex items-center gap-2 font-medium">
          <ClipboardListIcon className="size-4 shrink-0 text-primary" />
          <span className="min-w-0 truncate pr-14">{p.title}</span>
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
