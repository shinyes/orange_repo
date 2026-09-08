import { useState } from 'react'
import { Link } from 'react-router-dom'
import { BookOpenIcon, EyeIcon, PlusIcon, Trash2Icon } from 'lucide-react'
import { toast } from 'sonner'

import type { QuizBrief } from '@/api/types'
import { api } from '@/api'
import { Button } from '@/components/ui/button'
import { NewQuizDialog, VisibleUsersDialog } from '@/components/portal/space-item-dialogs'
import { useSpaceHome } from './useSpaceHome'
import { usePortalCtx } from './SpaceShell'
import { usePortalSession } from './portal-context'

// 空间刷题项目列表（空间壳内 tab 页），点击进入独立刷题页。
// 管理员：顶部可新建刷题项目、卡片眼睛分配可见成员、删除。
export function QuizList() {
  const { space } = usePortalCtx()
  const home = useSpaceHome(space)
  const list = home.data?.quizzes ?? []
  const { user } = usePortalSession()
  const canEdit = user.role !== 'member'
  const [creating, setCreating] = useState(false)
  const [visibleFor, setVisibleFor] = useState<QuizBrief | null>(null)

  async function remove(qz: QuizBrief) {
    if (!confirm(`删除刷题项目「${qz.title}」？`)) return
    try {
      await api.deleteSpaceQuiz(space.id, qz.id)
      toast.success('已删除')
      void home.refetch()
    } catch (e) {
      toast.error(e instanceof Error ? e.message : '删除失败')
    }
  }

  return (
    <div className="mx-auto w-full max-w-3xl px-4 py-6 lg:px-8">
      <div className="mb-1 flex flex-wrap items-center justify-between gap-2">
        <h1 className="text-lg font-semibold">刷题</h1>
        {canEdit && (
          <Button onClick={() => setCreating(true)} className="h-8 text-xs">
            <PlusIcon data-icon="inline-start" /> 新建刷题项目
          </Button>
        )}
      </div>
      <p className="mb-4 text-xs text-muted-foreground">
        范围内单选/判断题循环复习：做过少做 · 错题下轮多做 · 同轮不重复
      </p>
      {home.isLoading && <p className="py-10 text-center text-sm text-muted-foreground">加载中…</p>}
      <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
        {list.map((qz) => (
          <QuizCard
            key={qz.id}
            q={qz}
            spaceId={space.id}
            canEdit={canEdit}
            onVisible={() => setVisibleFor(qz)}
            onRemove={() => void remove(qz)}
          />
        ))}
      </div>
      {!home.isLoading && list.length === 0 && (
        <div className="rounded-xl border border-dashed p-12 text-center text-sm text-muted-foreground">
          本空间暂无刷题项目
        </div>
      )}

      <NewQuizDialog spaceId={space.id} open={creating} onOpenChange={setCreating} onCreated={() => void home.refetch()} />

      {visibleFor && (
        <VisibleUsersDialog
          spaceId={space.id}
          kind="quiz"
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

function QuizCard({ q, spaceId, canEdit, onVisible, onRemove }: {
  q: QuizBrief
  spaceId: number
  canEdit: boolean
  onVisible: () => void
  onRemove: () => void
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
            title="删除刷题项目"
            onClick={onRemove}
            className="flex size-7 items-center justify-center rounded-md border bg-background text-muted-foreground opacity-0 transition-opacity hover:border-red-400/40 hover:text-red-500 focus:opacity-100 group-hover:opacity-100"
          >
            <Trash2Icon className="size-3.5" />
          </button>
        </div>
      )}
      <Link
        to={`/s/${spaceId}/quiz/${q.id}`}
        className="flex w-full flex-col gap-1.5 rounded-2xl p-4"
      >
        <span className="flex items-center gap-2 font-medium">
          <BookOpenIcon className="size-4 shrink-0 text-primary" />
          <span className="min-w-0 truncate pr-16">{q.title}</span>
        </span>
        <span className="mt-1 text-xs text-muted-foreground">
          {q.sourceType === 'repo' ? '题单范围 · 循环复习' : '标签范围 · 循环复习'}
        </span>
      </Link>
    </div>
  )
}
