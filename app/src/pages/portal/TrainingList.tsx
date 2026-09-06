import { useState } from 'react'
import { Link } from 'react-router-dom'
import { FolderKanbanIcon, PencilIcon } from 'lucide-react'

import type { TrainingBrief } from '@/api/types'
import { TrainingQuickEditDialog } from '@/components/portal/TrainingQuickEditDialog'
import { usePortalCtx } from './SpaceShell'
import { usePortalSession } from './portal-context'
import { useSpaceHome } from './useSpaceHome'

// 训练列表（空间壳内 tab 页）：训练卡片点击进入独立做题详情；
// 管理员卡片右上显示 ✎ 快速编辑（近全屏弹窗编排章节/题目，改动即存）。
export function TrainingList() {
  const { space } = usePortalCtx()
  const home = useSpaceHome(space)
  const list = home.data?.trainings ?? []
  const { user } = usePortalSession()
  const canEdit = user.role !== 'member'
  const [editing, setEditing] = useState<TrainingBrief | null>(null)

  return (
    <div className="mx-auto w-full max-w-3xl px-4 py-6 lg:px-8">
      <h1 className="mb-1 text-lg font-semibold">训练</h1>
      <p className="mb-4 text-xs text-muted-foreground">按章节组织的训练，客观题限次作答（答对绿勾 / 次数用尽标红）</p>
      {home.isLoading && <p className="py-10 text-center text-sm text-muted-foreground">加载中…</p>}
      <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
        {list.map((t) => (
          <TrainingCard key={t.id} t={t} spaceId={space.id} canEdit={canEdit} onEdit={() => setEditing(t)} />
        ))}
      </div>
      {!home.isLoading && list.length === 0 && (
        <div className="rounded-xl border border-dashed p-12 text-center text-sm text-muted-foreground">
          本空间暂无训练
        </div>
      )}

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
    </div>
  )
}

function TrainingCard({ t, spaceId, canEdit, onEdit }: {
  t: TrainingBrief
  spaceId: number
  canEdit: boolean
  onEdit: () => void
}) {
  return (
    <div className="group relative rounded-2xl border bg-card transition-colors hover:border-primary/50">
      {canEdit && (
        <button
          type="button"
          title="快速编辑训练（章节/题目）"
          onClick={onEdit}
          className="absolute top-2 right-2 z-10 flex size-7 items-center justify-center rounded-md border bg-background text-muted-foreground opacity-0 transition-opacity hover:border-primary/40 hover:text-primary focus:opacity-100 group-hover:opacity-100"
        >
          <PencilIcon className="size-3.5" />
        </button>
      )}
      <Link
        to={`/s/${spaceId}/training/${t.id}`}
        className="flex w-full flex-col gap-1.5 rounded-2xl p-4"
      >
        <span className="flex items-center gap-2 font-medium">
          <FolderKanbanIcon className="size-4 shrink-0 text-primary" />
          <span className="min-w-0 truncate pr-7">{t.title}</span>
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
