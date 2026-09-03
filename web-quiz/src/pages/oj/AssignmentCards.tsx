import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { FolderKanbanIcon, ClipboardListIcon, InboxIcon } from 'lucide-react'

import { api } from '@/lib/api'
import type { OjPracticeBrief, OjTrainingBrief } from '@/lib/types'
import { Badge } from '@/components/ui/badge'

// 训练/练习卡片（学生视角任务列表）。
export function TrainingCards() {
  const q = useQuery({ queryKey: ['oj-assigned'], queryFn: api.ojAssigned })
  const list = q.data?.trainings ?? []
  return (
    <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
      {list.map((t) => (
        <TrainingCard key={t.id} t={t} />
      ))}
      {!q.isLoading && list.length === 0 && (
        <div className="col-span-full">
          <Empty text="暂无训练任务" hint="管理员布置训练后会显示在这里" />
        </div>
      )}
    </div>
  )
}

function TrainingCard({ t }: { t: OjTrainingBrief }) {
  const pct = t.problemCount > 0 ? Math.round((t.accepted / t.problemCount) * 100) : 0
  return (
    <Link
      to={`/training/${t.id}`}
      className="flex w-full flex-col gap-2 rounded-xl border bg-card p-4 transition-colors hover:border-primary/50 hover:bg-primary/5"
    >
      <div className="flex items-center justify-between gap-2">
        <span className="flex min-w-0 items-center gap-2 font-medium">
          <FolderKanbanIcon className="size-4 shrink-0 text-primary" />
          <span className="truncate">{t.title}</span>
        </span>
        {t.chapterCount > 0 && <Badge variant="secondary">{t.chapterCount} 章</Badge>}
      </div>
      {t.description && <p className="line-clamp-2 text-xs text-muted-foreground">{t.description}</p>}
      <ProgressBar accepted={t.accepted} total={t.problemCount} pct={pct} />
    </Link>
  )
}

export function PracticeCards() {
  const q = useQuery({ queryKey: ['oj-assigned'], queryFn: api.ojAssigned })
  const list = q.data?.practices ?? []
  return (
    <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
      {list.map((p) => (
        <PracticeCard key={p.id} p={p} />
      ))}
      {!q.isLoading && list.length === 0 && (
        <div className="col-span-full">
          <Empty text="暂无练习任务" hint="管理员布置练习后会显示在这里" />
        </div>
      )}
    </div>
  )
}

function PracticeCard({ p }: { p: OjPracticeBrief }) {
  const pct = p.problemCount > 0 ? Math.round((p.accepted / p.problemCount) * 100) : 0
  return (
    <Link
      to={`/practice/${p.id}`}
      className="flex w-full flex-col gap-2 rounded-xl border bg-card p-4 transition-colors hover:border-primary/50 hover:bg-primary/5"
    >
      <span className="flex min-w-0 items-center gap-2 font-medium">
        <ClipboardListIcon className="size-4 shrink-0 text-primary" />
        <span className="truncate">{p.title}</span>
      </span>
      {p.description && <p className="line-clamp-2 text-xs text-muted-foreground">{p.description}</p>}
      <ProgressBar accepted={p.accepted} total={p.problemCount} pct={pct} />
    </Link>
  )
}

export function ProgressBar({ accepted, total, pct }: { accepted: number; total: number; pct: number }) {
  return (
    <div className="flex items-center gap-2">
      <div className="h-1.5 flex-1 overflow-hidden rounded-full bg-muted">
        <div className="h-full rounded-full bg-emerald-500 transition-all" style={{ width: `${pct}%` }} />
      </div>
      <span className="text-xs text-muted-foreground">
        {accepted}/{total} 已完成
      </span>
    </div>
  )
}

function Empty({ text, hint }: { text: string; hint: string }) {
  return (
    <div className="rounded-xl border border-dashed p-10 text-center">
      <InboxIcon className="mx-auto mb-2 size-8 text-muted-foreground" />
      <p className="text-sm text-muted-foreground">{text}</p>
      <p className="mt-1 text-xs text-muted-foreground/70">{hint}</p>
    </div>
  )
}
