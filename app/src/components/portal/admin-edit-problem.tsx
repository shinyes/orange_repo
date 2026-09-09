// 做题界面（训练/练习/做题页）的「编辑题目」入口：仅域/系统管理员可见，
// 点击在弹窗内嵌管理端题目编辑器（保存后自动刷新题目缓存）。
import { useEffect, useRef, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { PencilIcon, Loader2Icon } from 'lucide-react'

import { api } from '@/api'
import { getDomain, setDomain } from '@/api/admin'
import { ProblemEditor } from '@/pages/admin/ProblemPane'
import { usePortalSession } from '@/pages/portal/portal-context'
import {
  Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription,
} from '@/components/ui/dialog'

export function AdminEditProblemButton({ problemId, className }: { problemId: number; className?: string }) {
  const { user } = usePortalSession()
  const qc = useQueryClient()
  const [open, setOpen] = useState(false)
  // global 管理员编辑题目前的管理域上下文（关闭弹窗时恢复）
  const prevDomainRef = useRef<number | null>(null)

  const q = useQuery({
    queryKey: ['admin-edit-problem', problemId],
    queryFn: () => api.getProblem(problemId),
    enabled: open,
    retry: 1,
  })

  // 打开弹窗：记录原管理域；global 管理员切到题目所属域（保证保存走对域接口）
  useEffect(() => {
    if (open && user.role === 'global_admin') {
      prevDomainRef.current = getDomain()
      const p = q.data?.problem
      if (p?.domainId) setDomain(p.domainId)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, q.data, user.role])

  function close() {
    if (open && user.role === 'global_admin') {
      setDomain(prevDomainRef.current)
      prevDomainRef.current = null
    }
    setOpen(false)
  }

  // 普通成员不可见
  if (user.role === 'member') return null

  return (
    <>
      <button
        type="button"
        title="编辑题目（管理员）"
        aria-label="编辑题目"
        onClick={() => setOpen(true)}
        className={
          className ??
          'inline-flex size-6 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-muted hover:text-primary'
        }
      >
        <PencilIcon className="size-3.5" />
      </button>

      <Dialog open={open} onOpenChange={(v) => !v && close()}>
        <DialogContent className="flex max-h-[92vh] w-full flex-col gap-0 p-0 sm:max-w-5xl">
          <DialogHeader className="border-b px-4 py-3">
            <DialogTitle className="flex items-center gap-2 text-base">
              <PencilIcon className="size-4 text-primary" /> 编辑题目 #{problemId}
            </DialogTitle>
            <DialogDescription>保存后做题界面会自动刷新为最新题面/选项/答案。</DialogDescription>
          </DialogHeader>
          <div className="min-h-0 flex-1 overflow-y-auto px-4 py-3">
            {q.isLoading && (
              <div className="flex items-center justify-center gap-2 py-16 text-sm text-muted-foreground">
                <Loader2Icon className="size-4 animate-spin" /> 加载题目…
              </div>
            )}
            {q.isError && (
              <p className="py-16 text-center text-sm text-red-500">
                题目加载失败（{q.error instanceof Error ? q.error.message : '未知错误'}）
              </p>
            )}
            {q.data?.problem && (
              <ProblemEditor
                problem={q.data.problem}
                onSaved={() => {
                  // 刷新做题侧所有题目内容缓存（同题多 key 前缀统一失效）
                  void qc.invalidateQueries({ queryKey: ['oj-problem'] })
                  void qc.invalidateQueries({ queryKey: ['portal-practice'] })
                  void qc.invalidateQueries({ queryKey: ['portal-training'] })
                  close()
                }}
              />
            )}
          </div>
        </DialogContent>
      </Dialog>
    </>
  )
}
