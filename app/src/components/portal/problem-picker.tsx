// 从当前域仓库选题的共享弹窗（空间训练加题 / 练习加题 / 快速编辑共用）。
// 题目来自当前域仓库（api.problems 走 dq 自动带 domainId）；已在目标内的题目置灰。
//
// 性能：题库可能有数千题——**不改**「一次拉全库再前端过滤」的做法会导致
// 首开加载久 + 渲染数千行卡顿。这里改为：
//   1) 搜索词防抖后交服务端过滤（q 参数，服务端按标题/ID 匹配）；
//   2) 只取前 LIMIT 条（服务端 limit 参数），并展示总数提示；
//   3) 查询结果按搜索词缓存，输入过程中不重复请求同词。
import { useEffect, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'

import { api } from '@/api'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { LoaderCircleIcon, SearchIcon } from 'lucide-react'

/** 单次最多取回并渲染的题目条数（超出提示细化搜索）。 */
const PICK_LIMIT = 100
/** 搜索防抖间隔（毫秒）。 */
const SEARCH_DEBOUNCE_MS = 300

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
  const [debounced, setDebounced] = useState('')
  const [selected, setSelected] = useState<Set<number>>(new Set())
  const [submitting, setSubmitting] = useState(false)

  // 搜索防抖：避免每次按键都打服务端
  useEffect(() => {
    const t = setTimeout(() => setDebounced(query.trim()), SEARCH_DEBOUNCE_MS)
    return () => clearTimeout(t)
  }, [query])

  // 关闭时重置（下次打开回到"最近题目"视图）
  useEffect(() => {
    if (!props.open) {
      setQuery('')
      setDebounced('')
      setSelected(new Set())
    }
  }, [props.open])

  const problemsQ = useQuery({
    queryKey: ['space-problem-picker', debounced],
    queryFn: () => api.problems({ q: debounced, tags: [], type: '', limit: PICK_LIMIT }),
    enabled: props.open,
    staleTime: 60_000, // 同词 1 分钟内复用缓存（反复打开弹窗不再重复请求）
  })
  const list = problemsQ.data?.problems ?? []
  const total = problemsQ.data?.total ?? list.length
  const existing = useMemo(() => new Set(props.existingIds ?? []), [props.existingIds])
  const truncated = total > list.length

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
          ) : list.length === 0 ? (
            <p className="py-6 text-center text-xs text-muted-foreground">
              {debounced ? '没有匹配的题目' : '题库为空'}
            </p>
          ) : (
            list.map((p) => {
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

        {/* 结果被截断时提示细化搜索（避免误以为题库里就这么多） */}
        {truncated && (
          <p className="text-[11px] text-muted-foreground">
            共 {total} 条，仅显示前 {list.length} 条 —— 输入关键词可精确定位。
          </p>
        )}

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
