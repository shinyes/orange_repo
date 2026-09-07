// 练习快速编辑弹窗（门户管理员）：练习列表卡片 ✎ 打开——
// 即改即存：标题/描述 + 题目列表（从题库加题 / 移除 / 上移下移排序）。
// 与管理端空间管理共用同一组空间练习 API；学生不显示入口。
import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import {
  ArrowDownIcon,
  ArrowUpIcon,
  ClipboardListIcon,
  LoaderCircleIcon,
  PlusIcon,
  XIcon,
} from 'lucide-react'

import { api } from '@/api'
import type { SpacePracticeItem } from '@/api/types'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { ProblemPickerDialog, typeLabel } from '@/components/portal/problem-picker'

export function PracticeQuickEditDialog(props: {
  spaceId: number
  practiceId: number
  practiceTitle: string
  open: boolean
  onOpenChange: (v: boolean) => void
  /** 内容变化后（刷新列表题数/详情缓存） */
  onChanged: () => void
}) {
  const { spaceId, practiceId } = props
  const qc = useQueryClient()
  const q = useQuery({
    queryKey: ['portal-practice-edit', spaceId, practiceId],
    queryFn: () => api.getSpacePractice(spaceId, practiceId),
    enabled: props.open,
  })

  const [title, setTitle] = useState(props.practiceTitle)
  const [description, setDescription] = useState('')
  const [busy, setBusy] = useState(false)
  const [pickerOpen, setPickerOpen] = useState(false)

  // 打开时同步初始标题/描述（详情到达后回填描述）
  const practice = q.data?.practice
  const items: SpacePracticeItem[] = q.data?.items ?? []

  function syncMeta() {
    if (practice) {
      setTitle(practice.title || props.practiceTitle)
      setDescription(practice.description || '')
    }
  }
  // practice 首次加载/变化后回填表单（用 key 控制子组件生命周期更简单——直接 useEffect 依赖 practiceId）
  const [metaLoaded, setMetaLoaded] = useState(false)
  if (practice && !metaLoaded) {
    setMetaLoaded(true)
    syncMeta()
  }
  // 切换练习（同一实例复用）重置
  const [lastPid, setLastPid] = useState(practiceId)
  if (lastPid !== practiceId) {
    setLastPid(practiceId)
    setMetaLoaded(false)
  }

  const invalidate = () => {
    void qc.invalidateQueries({ queryKey: ['portal-practice-edit', spaceId, practiceId] })
    props.onChanged()
  }

  async function run(action: () => Promise<unknown>, okMsg: string) {
    setBusy(true)
    try {
      await action()
      toast.success(okMsg)
      invalidate()
    } catch (e) {
      toast.error(e instanceof Error ? e.message : '操作失败')
    } finally {
      setBusy(false)
    }
  }

  // 标题/描述即改即存
  function saveMeta() {
    const t = title.trim()
    if (!t) {
      toast.error('标题不能为空')
      return
    }
    void run(
      () => api.updateSpacePractice(spaceId, practiceId, { title: t, description: description.trim() }),
      '已保存',
    )
  }

  // 排序：交换两条目标顺序（服务端按 itemIds 全量排序）
  function moveItem(idx: number, dir: -1 | 1) {
    const target = idx + dir
    if (target < 0 || target >= items.length) return
    const ids = items.map((i) => i.id)
    ;[ids[idx], ids[target]] = [ids[target], ids[idx]]
    void run(() => api.reorderPracticeItems(practiceId, ids), '已调整顺序')
  }

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className="flex max-h-[85vh] flex-col gap-0 p-0 sm:max-w-2xl">
        <DialogHeader className="border-b px-5 py-3">
          <DialogTitle className="flex items-center gap-2 text-base">
            <ClipboardListIcon className="size-4 text-primary" /> 编辑练习 · {practice?.title ?? props.practiceTitle}
          </DialogTitle>
          <DialogDescription>即改即存：标题/描述与题目清单（加题/移除/排序）。学生端不可见编辑入口。</DialogDescription>
        </DialogHeader>

        <div className="min-h-0 flex-1 space-y-4 overflow-y-auto px-5 py-4">
          {q.isLoading ? (
            <div className="flex items-center gap-2 py-8 text-sm text-muted-foreground">
              <LoaderCircleIcon className="size-4 animate-spin" /> 加载中…
            </div>
          ) : (
            <>
              {/* 元信息 */}
              <div className="space-y-3">
                <div className="flex gap-2">
                  <div className="min-w-0 flex-1 space-y-1.5">
                    <Label>练习标题</Label>
                    <Input
                      value={title}
                      onChange={(e) => setTitle(e.target.value)}
                      onBlur={saveMeta}
                      onKeyDown={(e) => e.key === 'Enter' && saveMeta()}
                    />
                  </div>
                  <Button size="icon-sm" variant="outline" className="mt-6" title="保存标题" onClick={saveMeta} disabled={busy}>
                    <LoaderCircleIcon className={busy ? 'size-4 animate-spin' : 'size-4'} />
                  </Button>
                </div>
                <div className="space-y-1.5">
                  <Label>描述</Label>
                  <Input
                    value={description}
                    onChange={(e) => setDescription(e.target.value)}
                    onBlur={saveMeta}
                    placeholder="练习说明（可选）"
                  />
                </div>
              </div>

              {/* 题目清单 */}
              <div>
                <div className="mb-1.5 flex items-center justify-between">
                  <span className="text-sm font-medium">题目（{items.length}）</span>
                  <Button size="xs" variant="outline" onClick={() => setPickerOpen(true)}>
                    <PlusIcon data-icon="inline-start" /> 从题库加题
                  </Button>
                </div>
                {items.length === 0 ? (
                  <p className="rounded-lg border border-dashed px-3 py-4 text-center text-xs text-muted-foreground">
                    暂无题目，点「从题库加题」添加
                  </p>
                ) : (
                  <ul className="divide-y rounded-lg border bg-background px-2">
                    {items.map((it, idx) => (
                      <li key={it.id} className="flex items-center gap-2 py-1.5 text-sm">
                        <span className="w-5 text-center text-[10px] text-muted-foreground tabular-nums">{idx + 1}</span>
                        <span className="min-w-0 flex-1 truncate">{it.problemTitle || `#${it.problemId}`}</span>
                        {it.problemType && <Badge variant="secondary" className="text-[10px]">{typeLabel(it.problemType)}</Badge>}
                        <button
                          type="button"
                          title="上移"
                          className="rounded p-1 text-muted-foreground hover:bg-muted disabled:opacity-30"
                          disabled={idx === 0}
                          onClick={() => moveItem(idx, -1)}
                        >
                          <ArrowUpIcon className="size-3.5" />
                        </button>
                        <button
                          type="button"
                          title="下移"
                          className="rounded p-1 text-muted-foreground hover:bg-muted disabled:opacity-30"
                          disabled={idx === items.length - 1}
                          onClick={() => moveItem(idx, 1)}
                        >
                          <ArrowDownIcon className="size-3.5" />
                        </button>
                        <button
                          type="button"
                          title="移除"
                          className="rounded p-1 text-muted-foreground hover:bg-muted hover:text-destructive"
                          onClick={() => {
                            void run(() => api.deleteSpaceItem(it.id), '已移除')
                          }}
                        >
                          <XIcon className="size-3.5" />
                        </button>
                      </li>
                    ))}
                  </ul>
                )}
              </div>
            </>
          )}
        </div>

        <ProblemPickerDialog
          open={pickerOpen}
          onOpenChange={setPickerOpen}
          title="向练习加题"
          existingIds={items.map((i) => i.problemId)}
          onSubmit={async (problemIds) => {
            try {
              await api.addSpacePracticeItems(spaceId, practiceId, problemIds)
              toast.success(`已加入 ${problemIds.length} 道题目`)
              setPickerOpen(false)
              invalidate()
            } catch (e) {
              toast.error(e instanceof Error ? e.message : '加入失败')
            }
          }}
        />
      </DialogContent>
    </Dialog>
  )
}
