// 从当前域仓库题库选题的共享弹窗（空间训练加题 / 练习加题 / 快速编辑共用）。
// 题目来自当前域仓库（api.problems 走 dq 自动带 domainId）；已在目标内的题目置灰。
import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'

import { api } from '@/api'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { LoaderCircleIcon, SearchIcon } from 'lucide-react'

export function typeLabel(t?: string): string {
  switch (t) {
    case 'programming': return '编程'
    case 'single_choice': return '单选'
    case 'true_false': return '判断'
    default: return '未知'
  }
}

export function ProblemPickerDialog(props: {
  open: boolean
  onOpenChange: (v: boolean) => void
  title: string
  /** 已在本空间目标（章节/练习）里的题目 ID，置灰不可重复加入。 */
  existingIds?: number[]
  /** 提交所选题目 ID。调用方负责调 API 并关闭。 */
  onSubmit: (problemIds: number[]) => void
}) {
  const [query, setQuery] = useState('')
  const [selected, setSelected] = useState<Set<number>>(new Set())
  const [submitting, setSubmitting] = useState(false)
  const problemsQ = useQuery({
    queryKey: ['space-problem-picker'],
    queryFn: () => api.problems({ q: '', tags: [], type: '' }),
    enabled: props.open,
  })
  const all = problemsQ.data?.problems ?? []
  const existing = new Set(props.existingIds ?? [])
  const q = query.trim().toLowerCase()
  const visible = q ? all.filter((p) => p.title.toLowerCase().includes(q) || String(p.id).includes(q)) : all

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{props.title}</DialogTitle>
          <DialogDescription>从当前域仓库题库中选择题目加入。已加入的题目置灰。</DialogDescription>
        </DialogHeader>

        <div className="relative">
          <SearchIcon className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="搜索标题 / ID…"
            className="pl-8"
          />
        </div>

        <div className="max-h-72 space-y-1 overflow-y-auto">
          {problemsQ.isLoading ? (
            <div className="flex items-center justify-center gap-2 py-6 text-xs text-muted-foreground">
              <LoaderCircleIcon className="size-4 animate-spin" /> 加载题目…
            </div>
          ) : visible.length === 0 ? (
            <p className="py-6 text-center text-xs text-muted-foreground">没有匹配的题目</p>
          ) : (
            visible.map((p) => {
              const isExisting = existing.has(p.id)
              const isSelected = selected.has(p.id)
              return (
                <label
                  key={p.id}
                  className={`flex cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-sm hover:bg-muted ${
                    isExisting ? 'cursor-not-allowed opacity-50' : isSelected ? 'bg-primary/5' : ''
                  }`}
                >
                  <input
                    type="checkbox"
                    className="size-3.5 accent-[var(--primary)]"
                    disabled={isExisting}
                    checked={isSelected || isExisting}
                    onChange={() =>
                      setSelected((prev) => {
                        const next = new Set(prev)
                        if (next.has(p.id)) next.delete(p.id)
                        else next.add(p.id)
                        return next
                      })
                    }
                  />
                  <Badge variant="outline" className="shrink-0 px-1.5 text-[10px] text-muted-foreground">
                    {typeLabel(p.type)}
                  </Badge>
                  <span className="min-w-0 flex-1 truncate">{p.title}</span>
                  <span className="shrink-0 text-[10px] text-muted-foreground">#{p.id}</span>
                  {isExisting && <span className="shrink-0 text-[10px] text-muted-foreground">已加入</span>}
                </label>
              )
            })
          )}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => props.onOpenChange(false)}>取消</Button>
          <Button
            disabled={selected.size === 0 || submitting}
            onClick={async () => {
              setSubmitting(true)
              try {
                await props.onSubmit([...selected])
              } finally {
                setSubmitting(false)
                setSelected(new Set())
              }
            }}
          >
            {submitting ? '加入中…' : `加入所选（${selected.size}）`}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
