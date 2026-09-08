// 训练快速编辑弹窗（做题端管理入口）：管理员在训练列表卡片点 ✎ 打开——
// 近全屏 Dialog，即改即存：标题/描述/限次 + 章节增删改名排序 + 章节内题目加/移/排序。
// 与管理端空间管理共用同一组空间训练 API；学生不显示入口。
import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import {
  ArrowDownIcon,
  ArrowUpIcon,
  BookOpenIcon,
  GripVerticalIcon,
  LoaderCircleIcon,
  PencilIcon,
  PlusIcon,
  SaveIcon,
  Trash2Icon,
  XIcon,
} from 'lucide-react'

import { api } from '@/api'
import type { SpaceChapter, SpaceTraining } from '@/api/types'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { ProblemPickerDialog, typeLabel } from '@/components/portal/problem-picker'
import { PublicToggleRow } from '@/components/portal/public-toggle'

export function TrainingQuickEditDialog(props: {
  spaceId: number
  trainingId: number
  trainingTitle: string
  open: boolean
  onOpenChange: (v: boolean) => void
  /** 内容变化后（刷新列表题数/详情缓存） */
  onChanged: () => void
}) {
  const { spaceId, trainingId } = props
  const qc = useQueryClient()
  const q = useQuery({
    queryKey: ['portal-training-edit', spaceId, trainingId],
    queryFn: () => api.getSpaceTraining(spaceId, trainingId),
    enabled: props.open,
  })

  // 章节编辑态（行内重命名输入）
  const [renaming, setRenaming] = useState<number | null>(null)
  const [renameText, setRenameText] = useState('')
  const [newChapter, setNewChapter] = useState('')
  const [busy, setBusy] = useState(false)
  // 选题弹窗
  const [pickerOpen, setPickerOpen] = useState(false)
  const [pickerChapter, setPickerChapter] = useState<number | null>(null)

  const invalidate = () => {
    void qc.invalidateQueries({ queryKey: ['portal-training-edit', spaceId, trainingId] })
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

  const training: SpaceTraining | undefined = q.data?.training
  const chapters: SpaceChapter[] = (q.data?.chapters ?? []).map((c) => ({ ...c, items: c.items ?? [] }))

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className="flex h-[100dvh] w-full flex-col gap-0 overflow-hidden rounded-none border-0 p-0 sm:h-auto sm:max-w-3xl sm:rounded-2xl sm:border md:max-h-[88vh]">
        <DialogHeader className="border-b px-4 py-3">
          <DialogTitle className="flex items-center gap-2 text-base">
            <PencilIcon className="size-4 text-primary" />
            编辑训练「{props.trainingTitle}」
          </DialogTitle>
          <DialogDescription>改动即保存；章节/题目可排序、重命名、增删。</DialogDescription>
        </DialogHeader>

        <div className="min-h-0 flex-1 overflow-y-auto px-4 py-3">
          {q.isLoading ? (
            <div className="flex items-center justify-center gap-2 py-10 text-sm text-muted-foreground">
              <LoaderCircleIcon className="size-4 animate-spin" /> 加载训练…
            </div>
          ) : !training ? (
            <p className="py-10 text-center text-sm text-muted-foreground">训练不存在或无权访问</p>
          ) : (
            <div className="space-y-4">
              {/* 基本信息 */}
              <div className="grid grid-cols-1 gap-3 rounded-xl border bg-background p-3 sm:grid-cols-2">
                <div className="space-y-1 sm:col-span-2">
                  <Label className="text-xs">标题</Label>
                  <MetaInput
                    key={`title-${training.title}`}
                    defaultValue={training.title}
                    placeholder="训练标题"
                    onSave={(v) => run(() => api.updateSpaceTraining(spaceId, trainingId, { title: v }), '标题已更新')}
                  />
                </div>
                <div className="space-y-1 sm:col-span-2">
                  <Label className="text-xs">描述（可选）</Label>
                  <MetaInput
                    key={`desc-${training.description}`}
                    defaultValue={training.description}
                    placeholder="训练说明"
                    onSave={(v) => run(() => api.updateSpaceTraining(spaceId, trainingId, { description: v }), '描述已更新')}
                  />
                </div>
                <div className="space-y-1">
                  <Label className="text-xs">客观题限答次数（0=不限）</Label>
                  <div className="flex items-center gap-2">
                    <Input
                      key={`ma-${training.maxAttempts}`}
                      type="number"
                      min={0}
                      defaultValue={training.maxAttempts}
                      className="h-8 w-24"
                      onBlur={(e) => {
                        const v = Math.max(0, Number(e.target.value) || 0)
                        if (v !== training.maxAttempts) {
                          void run(() => api.updateSpaceTraining(spaceId, trainingId, { maxAttempts: v }), '限答次数已更新')
                        }
                      }}
                    />
                    <span className="text-xs text-muted-foreground">答对绿勾 / 达限标红</span>
                  </div>
                </div>
                {/* 公开开关 */}
                <div className="sm:col-span-2">
                  <PublicToggleRow
                    checked={!!training.isPublic}
                    disabled={busy}
                    onCheckedChange={(v) =>
                      void run(
                        () => api.updateSpaceTraining(spaceId, trainingId, { isPublic: v }),
                        v ? '已设为公开（空间内所有成员可见）' : '已设为仅可见名单可见',
                      )
                    }
                  />
                </div>
              </div>

              {/* 章节列表 */}
              <div className="rounded-xl border bg-background">
                <div className="flex items-center gap-2 border-b px-3 py-2">
                  <BookOpenIcon className="size-4 text-primary/70" />
                  <span className="text-sm font-medium">章节与题目（{chapters.length} 章）</span>
                  <div className="ml-auto flex gap-1.5">
                    <Input
                      value={newChapter}
                      onChange={(e) => setNewChapter(e.target.value)}
                      placeholder="新章节名"
                      className="h-7 w-36 text-xs"
                      onKeyDown={(e) => {
                        if (e.key === 'Enter' && newChapter.trim()) {
                          const name = newChapter.trim()
                          setNewChapter('')
                          void run(() => api.createSpaceChapter(spaceId, trainingId, name), '章节已创建')
                        }
                      }}
                    />
                    <Button
                      size="xs"
                      variant="outline"
                      disabled={!newChapter.trim() || busy}
                      onClick={() => {
                        const name = newChapter.trim()
                        setNewChapter('')
                        void run(() => api.createSpaceChapter(spaceId, trainingId, name), '章节已创建')
                      }}
                    >
                      <PlusIcon data-icon="inline-start" /> 添加章节
                    </Button>
                  </div>
                </div>

                {chapters.length === 0 ? (
                  <p className="px-3 py-6 text-center text-xs text-muted-foreground">还没有章节，先在上方添加。</p>
                ) : (
                  chapters.map((ch, ci) => (
                    <div key={ch.id} className="border-b px-3 py-2 last:border-b-0">
                      <div className="flex items-center gap-1.5">
                        <GripVerticalIcon className="size-3.5 shrink-0 text-muted-foreground/40" />
                        {renaming === ch.id ? (
                          <>
                            <Input
                              autoFocus
                              value={renameText}
                              onChange={(e) => setRenameText(e.target.value)}
                              className="h-7 flex-1 text-sm"
                              onKeyDown={(e) => {
                                if (e.key === 'Enter' && renameText.trim()) {
                                  const name = renameText.trim()
                                  setRenaming(null)
                                  void run(() => api.renameSpaceChapter(ch.id, name), '章节已重命名')
                                }
                                if (e.key === 'Escape') setRenaming(null)
                              }}
                            />
                            <Button size="icon-xs" variant="ghost" disabled={!renameText.trim() || busy} onClick={() => { const name = renameText.trim(); setRenaming(null); void run(() => api.renameSpaceChapter(ch.id, name), '章节已重命名') }}>
                              <SaveIcon className="size-3.5" />
                            </Button>
                          </>
                        ) : (
                          <span className="min-w-0 flex-1 truncate text-sm font-medium">{ch.title || `第 ${ci + 1} 章`}</span>
                        )}
                        <Badge variant="outline" className="text-[10px]">{ch.items.length} 题</Badge>
                        {/* 排序 */}
                        <Button size="icon-xs" variant="ghost" title="上移" disabled={ci === 0 || busy} onClick={() => {
                          const ids = chapters.map((c) => c.id)
                          ;[ids[ci - 1], ids[ci]] = [ids[ci], ids[ci - 1]]
                          void run(() => api.reorderSpaceChapters(trainingId, ids), '章节顺序已更新')
                        }}>
                          <ArrowUpIcon className="size-3.5" />
                        </Button>
                        <Button size="icon-xs" variant="ghost" title="下移" disabled={ci === chapters.length - 1 || busy} onClick={() => {
                          const ids = chapters.map((c) => c.id)
                          ;[ids[ci], ids[ci + 1]] = [ids[ci + 1], ids[ci]]
                          void run(() => api.reorderSpaceChapters(trainingId, ids), '章节顺序已更新')
                        }}>
                          <ArrowDownIcon className="size-3.5" />
                        </Button>
                        <Button size="icon-xs" variant="ghost" title="重命名" onClick={() => { setRenaming(ch.id); setRenameText(ch.title ?? '') }}>
                          <PencilIcon className="size-3.5" />
                        </Button>
                        <Button size="icon-xs" variant="ghost" className="text-destructive" title="删除章节（含题目条目）" disabled={busy} onClick={() => {
                          if (!confirm(`删除章节「${ch.title || ci + 1}」？其题目条目将一并删除。`)) return
                          void run(() => api.deleteSpaceChapter(ch.id), '章节已删除')
                        }}>
                          <Trash2Icon className="size-3.5" />
                        </Button>
                        <Button size="xs" variant="outline" onClick={() => { setPickerChapter(ch.id); setPickerOpen(true) }}>
                          <PlusIcon data-icon="inline-start" /> 加题
                        </Button>
                      </div>

                      {/* 章节条目 */}
                      <ul className="mt-1 space-y-0.5">
                        {ch.items.map((it, ii) => (
                          <li key={it.id} className="flex items-center gap-1.5 rounded-md px-1.5 py-1 text-sm hover:bg-muted/50">
                            <span className="w-4 text-center text-[10px] text-muted-foreground">{ii + 1}</span>
                            <span className="min-w-0 flex-1 truncate">{it.problemTitle || `题目 #${it.problemId}`}</span>
                            {it.problemType && <Badge variant="secondary" className="text-[10px]">{typeLabel(it.problemType)}</Badge>}
                            <Button size="icon-xs" variant="ghost" title="上移" disabled={ii === 0 || busy} onClick={() => {
                              const ids = ch.items.map((x) => x.id)
                              ;[ids[ii - 1], ids[ii]] = [ids[ii], ids[ii - 1]]
                              void run(() => api.reorderSpaceChapterItems(ch.id, ids), '题目顺序已更新')
                            }}>
                              <ArrowUpIcon className="size-3" />
                            </Button>
                            <Button size="icon-xs" variant="ghost" title="下移" disabled={ii === ch.items.length - 1 || busy} onClick={() => {
                              const ids = ch.items.map((x) => x.id)
                              ;[ids[ii], ids[ii + 1]] = [ids[ii + 1], ids[ii]]
                              void run(() => api.reorderSpaceChapterItems(ch.id, ids), '题目顺序已更新')
                            }}>
                              <ArrowDownIcon className="size-3" />
                            </Button>
                            <Button size="icon-xs" variant="ghost" className="text-destructive" title="移除题目" disabled={busy} onClick={() => {
                              void run(() => api.deleteSpaceItem(it.id), '题目已移除')
                            }}>
                              <XIcon className="size-3" />
                            </Button>
                          </li>
                        ))}
                        {ch.items.length === 0 && <li className="py-1 pl-6 text-xs text-muted-foreground">空章节</li>}
                      </ul>
                    </div>
                  ))
                )}
              </div>
            </div>
          )}
        </div>

        {/* 选题弹窗 */}
        {pickerChapter != null && (
          <ProblemPickerDialog
            open={pickerOpen}
            onOpenChange={(v) => { setPickerOpen(v); if (!v) setPickerChapter(null) }}
            title={`向章节加题（${chapters.find((c) => c.id === pickerChapter)?.title ?? ''}）`}
            existingIds={chapters.find((c) => c.id === pickerChapter)?.items.map((i) => i.problemId) ?? []}
            onSubmit={async (problemIds) => {
              try {
                await api.addSpaceChapterItems(spaceId, pickerChapter, problemIds)
                toast.success(`已加入 ${problemIds.length} 道题目`)
                invalidate()
                setPickerOpen(false)
                setPickerChapter(null)
              } catch (e) {
                toast.error(e instanceof Error ? e.message : '加入失败')
              }
            }}
          />
        )}
      </DialogContent>
    </Dialog>
  )
}

// 失焦保存的输入框（标题/描述）：Enter 确认、Escape 还原。
function MetaInput(props: { defaultValue: string; placeholder?: string; onSave: (v: string) => void }) {
  const [v, setV] = useState(props.defaultValue)
  return (
    <Input
      value={v}
      onChange={(e) => setV(e.target.value)}
      placeholder={props.placeholder}
      className="h-8 text-sm"
      onKeyDown={(e) => {
        if (e.key === 'Enter') props.onSave(v.trim())
        if (e.key === 'Escape') setV(props.defaultValue)
      }}
      onBlur={() => {
        if (v.trim() !== props.defaultValue.trim()) props.onSave(v.trim())
      }}
    />
  )
}
