// /admin/problems 题目管理工作区（三栏）：自 web App.tsx Main 的三栏布局适配。
// 栏结构（与 web 一致）：
//   第一栏 TagFilterColumn（标签树/搜索/类型筛） + 第二栏 ProblemListColumn（题目列表/批量）
//   题册栏 BookletColumn（训练/练习模板，从题目列头展开） + 右栏 RightPane（题目/训练/练习详情或空态）。
// 域切换/管理入口/设置/备份/登出已上移 AdminLayout 顶栏，本页不再重复。
// 路由化适配：题目/训练/练习详情的「打开」保留在上下文（view），删除后 goHome() 回到空态。
import { lazy, Suspense, useEffect, useRef, useState } from 'react'

import { useAppState } from '@/pages/admin/app-context'
import { useDomain } from '@/pages/admin/domain-context'
import { TagFilterColumn, ProblemListColumn } from '@/pages/admin/Sidebar'
import { EmptyState } from '@/pages/admin/empty-state'

// 右栏/题册栏懒加载（与 web 一致：ProblemPane 含编辑器与 KaTeX 渲染，体积大）。
const ProblemPane = lazy(() => import('@/pages/admin/ProblemPane').then((m) => ({ default: m.ProblemPane })))
const BookletColumn = lazy(() => import('@/pages/admin/BookletColumn').then((m) => ({ default: m.BookletColumn })))
const TrainingDetail = lazy(() => import('@/pages/admin/GroupsPane').then((m) => ({ default: m.TrainingDetail })))
const PracticeDetail = lazy(() => import('@/pages/admin/GroupsPane').then((m) => ({ default: m.PracticeDetail })))

function PaneFallback() {
  return <div className="flex h-full items-center justify-center text-sm text-muted-foreground">加载中…</div>
}

export function ProblemsWorkspace() {
  const [showBooklets, setShowBooklets] = useState(false)
  const { view, goHome } = useAppState()
  const { domainId } = useDomain()
  const role = useRole()
  const needsDomainPick = role === 'global_admin' && domainId == null

  // 打开训练/练习时自动展开题册栏（与 web Main 行为一致）
  useEffect(() => {
    if (view.kind === 'training' || view.kind === 'practice') setShowBooklets(true)
  }, [view.kind === 'training', view.kind === 'practice'])

  // 切换域后回到空态并收起题册栏：旧域打开的题目/题册详情不再属于当前域（同 web Main）
  const lastDomain = useRef<number | null>(null)
  useEffect(() => {
    if (lastDomain.current !== null && lastDomain.current !== domainId) {
      goHome()
      setShowBooklets(false)
    }
    lastDomain.current = domainId
  }, [domainId, goHome])

  return (
    <div className="flex h-full overflow-hidden">
      <aside className={needsDomainPick ? 'w-[min(420px,90vw)] shrink-0 border-r' : 'w-[270px] shrink-0 border-r'}>
        <TagFilterColumn />
      </aside>
      {!needsDomainPick && (
        <>
          <aside className="w-[320px] shrink-0 border-r">
            <ProblemListColumn showBooklets={showBooklets} onToggleBooklets={() => setShowBooklets((v) => !v)} />
          </aside>
          {showBooklets && (
            <aside className="w-[280px] shrink-0 border-r">
              <Suspense fallback={null}>
                <BookletColumn />
              </Suspense>
            </aside>
          )}
          <main className="min-w-0 flex-1 overflow-y-auto">
            <Suspense fallback={<PaneFallback />}>
              <RightPane />
            </Suspense>
          </main>
        </>
      )}
    </div>
  )
}

function useRole(): string | null {
  const { user } = useDomain()
  return user?.role ?? null
}

function RightPane() {
  const { view } = useAppState()
  switch (view.kind) {
    case 'problem':
      return <ProblemPane key={`p${view.id}`} id={view.id} />
    case 'training':
      return <TrainingDetail key={`t${view.id}`} id={view.id} />
    case 'practice':
      return <PracticeDetail key={`x${view.id}`} id={view.id} />
    default:
      return <EmptyState />
  }
}
