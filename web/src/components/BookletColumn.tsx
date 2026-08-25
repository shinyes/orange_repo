import { useMemo, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { FileCodeIcon, ListChecksIcon, SearchIcon } from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { ScrollArea } from '@/components/ui/scroll-area'
import { api } from '@/lib/api'
import { useAppState } from '@/lib/app-context'

// 题册列：训练+练习混排，可筛选，点击打开详情。
export function BookletColumn() {
  const { view, openTraining, openPractice } = useAppState()
  const [search, setSearch] = useState('')
  const [newTitle, setNewTitle] = useState('')
  const qc = useQueryClient()
  const trainingsQ = useQuery({ queryKey: ['trainings'], queryFn: api.trainings })
  const practicesQ = useQuery({ queryKey: ['practices'], queryFn: api.practices })

  const items = useMemo(() => {
    const all: { id: number; type: 'training' | 'practice'; title: string; count: number }[] = []
    for (const t of trainingsQ.data?.trainings ?? []) all.push({ id: t.id, type: 'training', title: t.title, count: t.problemCount })
    for (const p of practicesQ.data?.practices ?? []) all.push({ id: p.id, type: 'practice', title: p.title, count: p.problemCount })
    const q = search.trim().toLowerCase()
    if (q) return all.filter((x) => x.title.toLowerCase().includes(q))
    return all
  }, [trainingsQ.data, practicesQ.data, search])

  const isLoading = trainingsQ.isLoading || practicesQ.isLoading

  function openItem(item: { type: string; id: number }) {
    if (item.type === 'training') openTraining(item.id)
    else openPractice(item.id)
  }

  async function createTraining() {
    if (!newTitle.trim()) return
    try {
      const { id } = await api.createTraining(newTitle.trim())
      setNewTitle('')
      await qc.invalidateQueries({ queryKey: ['trainings'] })
      openTraining(id)
    } catch (e) {
      toast.error(e instanceof Error ? e.message : '创建失败')
    }
  }

  async function createPractice() {
    if (!newTitle.trim()) return
    try {
      const { id } = await api.createPractice(newTitle.trim())
      setNewTitle('')
      await qc.invalidateQueries({ queryKey: ['practices'] })
      openPractice(id)
    } catch (e) {
      toast.error(e instanceof Error ? e.message : '创建失败')
    }
  }

  return (
    <div className="flex h-full flex-col bg-sidebar">
      <div className="px-3 pt-3 pb-2">
        <div className="mb-2 flex items-center gap-1.5">
          <span className="text-sm font-semibold">题册</span>
          <Badge variant="secondary" className="h-4 px-1.5 text-[10px] tabular-nums">
            {items.length}
          </Badge>
        </div>
        <div className="relative">
          <SearchIcon className="pointer-events-none absolute top-1/2 left-5 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="搜索题册名称…"
            className="pl-8"
          />
        </div>
      </div>

      <ScrollArea className="min-h-0 flex-1 px-2 pb-2">
        {isLoading && <div className="px-2 py-4 text-center text-xs text-muted-foreground">加载中…</div>}
        {!isLoading && items.length === 0 && (
          <div className="px-2 py-4 text-center text-xs text-muted-foreground">暂无题册</div>
        )}
        {items.map((it) => {
          const active =
            (view.kind === 'training' && view.id === it.id) ||
            (view.kind === 'practice' && view.id === it.id)
          return (
            <button
              key={`${it.type}-${it.id}`}
              type="button"
              className={`flex w-full items-start gap-2 rounded-md px-2 py-2 text-left transition-colors ${
                active ? 'bg-accent text-accent-foreground' : 'hover:bg-muted'
              }`}
              onClick={() => openItem(it)}
            >
              {it.type === 'training' ? (
                <FileCodeIcon className="mt-0.5 size-3.5 shrink-0 text-primary/70" />
              ) : (
                <ListChecksIcon className="mt-0.5 size-3.5 shrink-0 text-primary/70" />
              )}
              <div className="min-w-0 flex-1">
                <div className="truncate text-sm">{it.title}</div>
                <div className="flex items-center gap-1.5 mt-0.5">
                  <Badge variant="outline" className="h-3.5 px-1 text-[9px]">
                    {it.type === 'training' ? '训练' : '练习'}
                  </Badge>
                  <span className="text-[10px] text-muted-foreground">{it.count} 题</span>
                </div>
              </div>
            </button>
          )
        })}
      </ScrollArea>

      {/* 新建题册 */}
      <div className="border-t space-y-1.5 px-3 py-2">
        <div className="flex gap-1">
          <Input
            value={newTitle}
            onChange={(e) => setNewTitle(e.target.value)}
            placeholder="名称"
            className="h-7 text-xs"
            onKeyDown={(e) => {
              if (e.key === 'Enter') void createTraining()
            }}
          />
          <Button size="xs" variant="outline" onClick={createTraining} disabled={!newTitle.trim()}>
            <FileCodeIcon className="size-3" /> 训练
          </Button>
          <Button size="xs" variant="outline" onClick={createPractice} disabled={!newTitle.trim()}>
            <ListChecksIcon className="size-3" /> 练习
          </Button>
        </div>
      </div>
    </div>
  )
}
