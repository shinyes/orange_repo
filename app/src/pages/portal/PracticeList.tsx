import { Link, useParams } from 'react-router-dom'
import { ClipboardListIcon } from 'lucide-react'

import type { PracticeBrief } from '@/api/types'
import { useSpaceById } from './portal-context'
import { PageContainer, SpacePageShell } from './SpacePageShell'
import { useSpaceHome } from './useSpaceHome'

// 练习列表：整卷交卷式练习卡片。独立全屏页。
export function PracticeList() {
  const { spaceId } = useParams()
  const sid = Number(spaceId)
  const space = useSpaceById(sid)

  if (space === 'loading') return <FullCenter text="加载中…" />
  if (space === null) return <FullCenter text="空间不存在或无权访问" />

  return (
    <SpacePageShell spaceId={sid} backTo="/" backLabel={space.name}>
      <PracticeListBody spaceId={sid} />
    </SpacePageShell>
  )
}

function PracticeListBody({ spaceId }: { spaceId: number }) {
  const home = useSpaceHome({ id: spaceId })
  const list = home.data?.practices ?? []
  return (
    <PageContainer>
      <h1 className="mb-1 mt-2 text-lg font-semibold">练习</h1>
      <p className="mb-4 text-xs text-muted-foreground">整卷作答后统一交卷评分，可重复交卷（每次记录）</p>
      {home.isLoading && <p className="py-10 text-center text-sm text-muted-foreground">加载中…</p>}
      <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
        {list.map((p) => <PracticeCard key={p.id} p={p} spaceId={spaceId} />)}
      </div>
      {!home.isLoading && list.length === 0 && (
        <div className="rounded-xl border border-dashed p-12 text-center text-sm text-muted-foreground">
          本空间暂无练习
        </div>
      )}
    </PageContainer>
  )
}

function PracticeCard({ p, spaceId }: { p: PracticeBrief; spaceId: number }) {
  return (
    <Link
      to={`/s/${spaceId}/practice/${p.id}`}
      className="flex w-full flex-col gap-1.5 rounded-2xl border bg-card p-4 transition-colors hover:border-primary/50 hover:bg-primary/5"
    >
      <span className="flex items-center gap-2 font-medium">
        <ClipboardListIcon className="size-4 shrink-0 text-primary" />
        <span className="min-w-0 truncate">{p.title}</span>
      </span>
      {p.description && <p className="line-clamp-2 text-xs text-muted-foreground">{p.description}</p>}
      <span className="mt-1 flex items-center justify-between text-xs text-muted-foreground">
        <span>{p.problemCount} 题</span>
        <span>整卷交卷 · 可重做</span>
      </span>
    </Link>
  )
}

function FullCenter({ text }: { text: string }) {
  return <div className="flex h-dvh items-center justify-center text-sm text-muted-foreground">{text}</div>
}
