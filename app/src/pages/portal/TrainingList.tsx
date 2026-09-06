import { FolderKanbanIcon } from 'lucide-react'
import { Link, useParams } from 'react-router-dom'

import type { TrainingBrief } from '@/api/types'
import { useSpaceById } from './portal-context'
import { PageContainer, SpacePageShell } from './SpacePageShell'
import { useSpaceHome } from './useSpaceHome'

// 训练列表：空间内训练卡片（点击进入章节做题）。独立全屏页，顶栏左「返回空间」。
export function TrainingList() {
  const { spaceId } = useParams()
  const sid = Number(spaceId)
  const space = useSpaceById(sid)

  if (space === 'loading') return <FullCenter text="加载中…" />
  if (space === null) return <FullCenter text="空间不存在或无权访问" />
  const spaceName = space.name

  return (
    <SpacePageShell spaceId={sid} backTo="/" backLabel={spaceName} spaceName="">
      <TrainingListBody spaceId={sid} />
    </SpacePageShell>
  )
}

function TrainingListBody({ spaceId }: { spaceId: number }) {
  const home = useSpaceHome({ id: spaceId })
  const list = home.data?.trainings ?? []
  return (
    <PageContainer>
      <h1 className="mb-1 mt-2 text-lg font-semibold">训练</h1>
      <p className="mb-4 text-xs text-muted-foreground">按章节组织的训练，客观题限次作答（答对绿勾 / 次数用尽标红）</p>
      {home.isLoading && <p className="py-10 text-center text-sm text-muted-foreground">加载中…</p>}
      <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
        {list.map((t) => <TrainingCard key={t.id} t={t} spaceId={spaceId} />)}
      </div>
      {!home.isLoading && list.length === 0 && (
        <div className="rounded-xl border border-dashed p-12 text-center text-sm text-muted-foreground">
          本空间暂无训练
        </div>
      )}
    </PageContainer>
  )
}

function TrainingCard({ t, spaceId }: { t: TrainingBrief; spaceId: number }) {
  return (
    <Link
      to={`/s/${spaceId}/training/${t.id}`}
      className="flex w-full flex-col gap-1.5 rounded-2xl border bg-card p-4 transition-colors hover:border-primary/50 hover:bg-primary/5"
    >
      <span className="flex items-center gap-2 font-medium">
        <FolderKanbanIcon className="size-4 shrink-0 text-primary" />
        <span className="min-w-0 truncate">{t.title}</span>
      </span>
      {t.description && <p className="line-clamp-2 text-xs text-muted-foreground">{t.description}</p>}
      <span className="mt-1 flex items-center justify-between text-xs text-muted-foreground">
        <span>{t.problemCount} 题</span>
        {t.maxAttempts > 0 ? <span>每题限答 {t.maxAttempts} 次</span> : <span>不限作答次数</span>}
      </span>
    </Link>
  )
}

function FullCenter({ text }: { text: string }) {
  return <div className="flex h-dvh items-center justify-center text-sm text-muted-foreground">{text}</div>
}
