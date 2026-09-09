// 做题界面的「查看题解」入口（与编辑按钮并列，位于其左侧）：
// 弹窗展示该编程题的官方题解（语言/思路 markdown + 参考代码）。
// 当前与编辑按钮同一权限口径（域/系统管理员可见）；如需要可放开给成员。
import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { LightbulbIcon, Loader2Icon } from 'lucide-react'

import { api } from '@/api'
import { SolutionsView } from '@/pages/admin/problem-view'
import { usePortalSession } from '@/pages/portal/portal-context'
import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog'

export function ViewSolutionButton({ problemId, className }: { problemId: number; className?: string }) {
  const { user } = usePortalSession()
  const [open, setOpen] = useState(false)
  // 普通成员暂不可见（与编辑按钮同口径）
  if (user.role === 'member') return null

  const q = useQuery({
    queryKey: ['view-solution', problemId],
    queryFn: () => api.getProblem(problemId),
    enabled: open,
    retry: 1,
  })

  const solutions = q.data?.problem.solutions ?? []

  return (
    <>
      <button
        type="button"
        title="查看题解"
        aria-label="查看题解"
        onClick={() => setOpen(true)}
        className={
          className ??
          'inline-flex size-6 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-muted hover:text-primary'
        }
      >
        <LightbulbIcon className="size-3.5" />
      </button>

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="flex max-h-[85vh] flex-col sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <LightbulbIcon className="size-4 text-primary" /> 题解
            </DialogTitle>
          </DialogHeader>
          <div className="min-h-0 flex-1 overflow-y-auto">
            {q.isLoading && (
              <div className="flex items-center justify-center gap-2 py-10 text-sm text-muted-foreground">
                <Loader2Icon className="size-4 animate-spin" /> 加载题解…
              </div>
            )}
            {q.isError && (
              <p className="py-10 text-center text-sm text-red-500">
                题解加载失败（{q.error instanceof Error ? q.error.message : '未知错误'}）
              </p>
            )}
            {!q.isLoading && !q.isError && <SolutionsView solutions={solutions} />}
          </div>
        </DialogContent>
      </Dialog>
    </>
  )
}
