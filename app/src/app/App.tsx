// 路由根（src/app/App.tsx）：单前端应用——门户 + 管理端。
// 门户为空间化结构（登录 → 空间选择/直达 → 空间内 训练/练习/刷题/排行榜）；
// 管理员（domain_admin/global_admin）同为做题界面 + 顶栏管理入口；/admin 为管理区
// （题目管理三栏工作区 / 域管理 / 空间管理，路由化；member 访问重定向回 /）。
import { lazy, Suspense, useEffect, useState } from 'react'
import { BrowserRouter, Navigate, NavLink, Outlet, Route, Routes } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ShieldIcon, UserRoundIcon } from 'lucide-react'

import { authApi } from '@/api/auth'
import { UNAUTHORIZED_EVENT } from '@/api/client'
import type { User } from '@/api/types'
import { cn } from '@/lib/utils'
import { Toaster } from '@/components/ui/sonner'
import { Login } from '@/components/Login'
import { MyPage } from '@/pages/portal/MyPage'
import { SpacePicker } from '@/pages/portal/SpacePicker'
import { SpaceShell } from '@/pages/portal/SpaceShell'
import { PortalSessionProvider } from '@/pages/portal/portal-context'
import { TrainingList } from '@/pages/portal/TrainingList'
import { TrainingDetail } from '@/pages/portal/TrainingDetail'
import { PracticeList } from '@/pages/portal/PracticeList'
import { PracticeSolve } from '@/pages/portal/PracticeSolve'
import { PracticeRecordPage } from '@/pages/portal/PracticeRecordPage'
import { QuizList } from '@/pages/portal/QuizList'
import { QuizSolve } from '@/pages/portal/QuizSolve'
import { WrongBookPage } from '@/pages/portal/WrongBookPage'
import { RankPage } from '@/pages/portal/RankPage'
import { AdminLayout } from '@/pages/admin/AdminLayout'

// 重量级页面懒加载，不进首屏主包（做题页 + 管理区各页）。
const ProblemSolvePage = lazy(() => import('@/pages/oj/ProblemSolvePage').then((m) => ({ default: m.ProblemSolvePage })))
const AdminProblemsWorkspace = lazy(() => import('@/pages/admin/ProblemsWorkspace').then((m) => ({ default: m.ProblemsWorkspace })))
const AdminDomainAdmin = lazy(() => import('@/pages/admin/DomainAdmin').then((m) => ({ default: m.DomainAdmin })))
const AdminSpaceAdmin = lazy(() => import('@/pages/admin/SpaceAdmin').then((m) => ({ default: m.SpaceAdmin })))
const AdminUsers = lazy(() => import('@/pages/admin/UsersAdmin').then((m) => ({ default: m.UsersAdmin })))

function PageFallback() {
  return (
    <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
      加载中…
    </div>
  )
}

const queryClient = new QueryClient({
  defaultOptions: {
    queries: { retry: 1, staleTime: 0 },
  },
})

export default function App() {
  const [authed, setAuthed] = useState<boolean | null>(null)
  const [user, setUser] = useState<User | null>(null)

  useEffect(() => {
    authApi.me().then((d) => {
      setAuthed(d.authenticated)
      setUser(d.user ?? null)
    }).catch(() => setAuthed(false))
    const on401 = () => {
      queryClient.clear() // 会话失效：清空缓存，防换账号残留上一账号的空间/排行数据
      setAuthed(false)
      setUser(null)
      window.location.href = '/login'
    }
    window.addEventListener(UNAUTHORIZED_EVENT, on401)
    return () => window.removeEventListener(UNAUTHORIZED_EVENT, on401)
  }, [])

  if (authed === null) {
    return <div className="flex h-dvh items-center justify-center text-sm text-muted-foreground">正在连接刷题服务…</div>
  }

  const onLogout = () =>
    void authApi.logout().finally(() => {
      queryClient.clear()
      setAuthed(false)
      window.location.href = '/login'
    })

  return (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        {authed && user ? (
          <Suspense fallback={<PageFallback />}>
            <PortalSessionProvider value={{ user, onLogout }}>
              <Routes>
              <Route path="/login" element={<Navigate to="/" replace />} />

              {/* 非空间页：顶部壳（品牌 + 我的 [+ 管理]） */}
              <Route path="/" element={<TopShell user={user} onLogout={onLogout} />}>
                <Route index element={<SpacePicker user={user} />} />
                <Route path="mine" element={<MyPage />} />
                <Route path="problem/:problemId" element={<ProblemSolvePage />} />
              </Route>

              {/* 空间壳：列表页在壳内（顶栏 tab 切换）；详情/作答为独立全屏页 */}
              <Route path="/s/:spaceId" element={<SpaceShell user={user} onLogout={onLogout} />}>
                <Route index element={<Navigate to="training" replace />} />
                <Route path="training" element={<TrainingList />} />
                <Route path="practice" element={<PracticeList />} />
                <Route path="quiz" element={<QuizList />} />
                <Route path="rank" element={<RankPage />} />
              </Route>

              {/* 独立全屏页（点击具体训练/练习/刷题后进入）：顶栏左上返回列表 */}
              <Route path="/s/:spaceId/training/:trainingId" element={<TrainingDetail />} />
              <Route path="/s/:spaceId/training/:trainingId/q/:no" element={<TrainingDetail />} />
              <Route path="/s/:spaceId/practice/:practiceId" element={<PracticeSolve />} />
              <Route path="/s/:spaceId/practice/:practiceId/record/:submissionId" element={<PracticeRecordPage />} />
              <Route path="/s/:spaceId/quiz/:quizId" element={<QuizSolve />} />
              <Route path="/s/:spaceId/wrong-book" element={<WrongBookPage />} />

              {/* 管理区：仅管理员（global_admin/domain_admin），member 重定向回门户 */}
              <Route path="/admin" element={<RequireAdmin user={user}><AdminLayout user={user} onLogout={onLogout} /></RequireAdmin>}>
                <Route index element={<Navigate to="problems" replace />} />
                <Route path="problems" element={<AdminProblemsWorkspace />} />
                <Route path="users" element={<AdminUsers />} />
                <Route path="spaces" element={<AdminSpaceAdmin />} />
                <Route path="domains" element={<AdminDomainAdmin />} />
              </Route>

              <Route path="*" element={<Navigate to="/" replace />} />
              </Routes>
            </PortalSessionProvider>
          </Suspense>
        ) : (
          <Login onSuccess={(u) => { setUser(u); setAuthed(true) }} />
        )}
      </BrowserRouter>
      <Toaster position="bottom-right" richColors closeButton />
    </QueryClientProvider>
  )
}

export type ShellContext = { user: User; onLogout: () => void }

// /admin 角色守卫：仅 global_admin / domain_admin 可进入；member 访问重定向回门户首页。
function RequireAdmin({ user, children }: { user: User; children: React.ReactNode }) {
  if (user.role === 'member') return <Navigate to="/" replace />
  return <>{children}</>
}

// 非空间页的轻量顶壳：品牌 → 我的（个人入口）；管理员额外展示「管理」入口。
// 「我的」页含账号与退出。
function TopShell({ user, onLogout }: { user: User; onLogout: () => void }) {
  const isAdmin = user.role !== 'member'
  return (
    <div className="flex h-dvh flex-col overflow-hidden">
      <header className="shrink-0 border-b bg-background">
        <div className="mx-auto flex h-11 w-full max-w-6xl items-center gap-4 px-4 lg:px-6">
          <NavLink to="/" className="flex shrink-0 items-center gap-2 text-base font-semibold">
            <img src="/favicon.png" alt="OrangeOJ" className="size-7 rounded-lg" />
            OrangeOJ
          </NavLink>
          {isAdmin && (
            <NavLink
              to="/admin"
              className={({ isActive }) =>
                cn(
                  'flex shrink-0 items-center gap-1.5 rounded-lg px-2.5 py-1.5 text-xs transition-colors',
                  isActive
                    ? 'bg-primary/10 font-medium text-primary'
                    : 'border border-border text-muted-foreground hover:border-primary/40 hover:text-foreground',
                )
              }
            >
              <ShieldIcon className="size-3.5" />
              管理
            </NavLink>
          )}
          <div className="flex-1" />
          <NavLink
            to="/mine"
            className={({ isActive }) =>
              cn(
                'flex shrink-0 items-center gap-2 rounded-full border py-1 pl-1 pr-3 text-sm transition-colors',
                isActive
                  ? 'border-primary/40 bg-primary/10 font-medium text-primary'
                  : 'border-border text-muted-foreground hover:border-primary/40 hover:text-foreground',
              )
            }
          >
            <span className="flex size-6 items-center justify-center rounded-full bg-primary/15 text-xs font-semibold text-primary">
              {user.username.slice(0, 1).toUpperCase()}
            </span>
            <span className="hidden max-w-28 truncate sm:inline">{user.username}</span>
            {!isAdmin ? null : (
              <span className="rounded bg-muted px-1 py-0.5 text-[10px]">
                {user.role === 'domain_admin' ? '域管理' : '系统管理'}
              </span>
            )}
            <UserRoundIcon className="size-4 lg:hidden" />
          </NavLink>
        </div>
      </header>
      <main className="min-h-0 flex-1 overflow-y-auto">
        <Outlet context={{ user, onLogout }} />
      </main>
    </div>
  )
}
