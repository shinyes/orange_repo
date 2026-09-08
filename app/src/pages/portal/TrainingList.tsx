import { useState } from 'react'
import { Link } from 'react-router-dom'
import { EyeIcon, FolderKanbanIcon, PencilIcon, PlusIcon } from 'lucide-react'

import type { TrainingBrief } from '@/api/types'
import { Button } from '@/components/ui/button'
import { TrainingQuickEditDialog } from '@/components/portal/TrainingQuickEditDialog'
import { NewTrainingDialog, VisibleUsersDialog } from '@/components/portal/space-item-dialogs'
import { usePortalCtx } from './SpaceShell'
import { usePortalSession } from './portal-context'
import { useSpaceHome } from './useSpaceHome'

// 训练列表（空间壳内 tab 页）：训练卡片点击进入独立做题详情；
// 管理员：顶部可新建训练、卡片 ✎ 快速编辑 + 眼睛分配可见成员（默认无成员可见）。
export function TrainingList() {
  const { space } = usePortalCtx()
  const home = useSpaceHome(space)
  const list = home.data?.trainings ?? []
  const { user } = usePortalSession()
  const canEdit = user.role !== 'member'
  const [creating, setCreating] = useState(false)
  const [editing, setEditing] = useState<TrainingBrief | null>(null)
  const [visibleFor, setVisibleFor] = useState<TrainingBrief | null>(null)

  return (
    <div className="mx-auto w-full max-w-3xl px-4 py-6 lg:px-8">
      <div className="mb-1 flex flex-wrap items-center justify-between gap-2">
        <h1 className="text-lg font-semibold">训练</h1>
        {canEdit && (
          <Button onClick={() => setCreating(true)} className="h-8 text-xs">
            <PlusIcon data-icon="inline-start" /> 新建训练
          </Button>
        )}
      </div>
      <p className="mb-4 text-xs text-muted-foreground">按章节组织的训练，客观题限次作答（答对绿勾 / 次数用尽标红）</p>
      {home.isLoading && <p className="py-10 text-center text-sm text-muted-foreground">加载中…</p>}
      {home.isError && (
        <div className="py-10 text-center text-sm text-red-500">
          加载失败{home.error instanceof Error ? `：${home.error.message}` : ''}
          <button type="button" className="ml-2 text-primary underline" onClick={() => void home.refetch()}>重试</button>
        </div>
      )}
      <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
        {list.map((t) => (
          <TrainingCard
            key={t.id}
            t={t}
            spaceId={space.id}
            canEdit={canEdit}
            onEdit={() => setEditing(t)}
            onVisible={() => setVisibleFor(t)}
          />
        ))}
      </div>
      {!home.isLoading && list.length === 0 && (
        <div className="rounded-xl border border-dashed p-12 text-center text-sm text-muted-foreground">
          本空间暂无训练
        </div>
      )}

      <NewTrainingDialog spaceId={space.id} open={creating} onOpenChange={setCreating} onCreated={() => void home.refetch()} />

      {editing && (
        <TrainingQuickEditDialog
          spaceId={space.id}
          trainingId={editing.id}
          trainingTitle={editing.title}
          open
          onOpenChange={(v) => !v && setEditing(null)}
          onChanged={() => void home.refetch()}
        />
      )}

      {visibleFor && (
        <VisibleUsersDialog
          spaceId={space.id}
          kind="training"
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

function TrainingCard({ t, spaceId, canEdit, onEdit, onVisible }: {
  t: TrainingBrief
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
            title="快速编辑训练（章节/题目）"
            onClick={onEdit}
            className="flex size-7 items-center justify-center rounded-md border bg-background text-muted-foreground opacity-0 transition-opacity hover:border-primary/40 hover:text-primary focus:opacity-100 group-hover:opacity-100"
          >
            <PencilIcon className="size-3.5" />
          </button>
        </div>
      )}
      <Link
        to={`/s/${spaceId}/training/${t.id}`}
        className="flex w-full flex-col gap-1.5 rounded-2xl p-4"
      >
        <span className="flex items-center gap-2 font-medium">
          <FolderKanbanIcon className="size-4 shrink-0 text-primary" />
          <span className="min-w-0 truncate pr-14">{t.title}</span>
        </span>
        {t.description && <p className="line-clamp-2 text-xs text-muted-foreground">{t.description}</p>}
        <span className="mt-1 flex items-center justify-between text-xs text-muted-foreground">
          <span>{t.problemCount} 题</span>
          {t.maxAttempts > 0 ? <span>每题限答 {t.maxAttempts} 次</span> : <span>不限作答次数</span>}
        </span>
      </Link>
    </div>
  )
}
