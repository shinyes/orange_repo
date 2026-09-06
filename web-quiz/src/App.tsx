import { lazy, Suspense, useEffect, useState } from 'react'
import { BrowserRouter, Navigate, NavLink, Outlet, Route, Routes } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { UserRoundIcon } from 'lucide-react'

import { api } from '@/lib/api'
import { Toaster } from '@/components/ui/sonner'
import { Login } from '@/components/Login'
import { MyPage } from '@/pages/MyPage'
import { SpacePicker } from '@/pages/portal/SpacePicker'
import { SpaceShell } from '@/pages/portal/SpaceShell'
import { TrainingList } from '@/pages/portal/TrainingList'
import { TrainingDetail } from '@/pages/portal/TrainingDetail'
import { PracticeList } from '@/pages/portal/PracticeList'
import { PracticeSolve } from '@/pages/portal/PracticeSolve'
import { QuizList } from '@/pages/portal/QuizList'
import { QuizSolve } from '@/pages/portal/QuizSolve'
import { RankPage } from '@/pages/portal/RankPage'
import type { User } from '@/lib/types'
import { cn } from '@/lib/utils'

// 做题页等重量级组件懒加载，不进首屏主包。
const ProblemSolvePage = lazy(() => import('@/pages/oj/ProblemSolvePage').then((m) => ({ default: m.ProblemSolvePage })))

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

// 空间化门户：登录 → 空间（单空间自动进入 / 多空间选择）→ 空间内 训练/练习/刷题/排行榜；
// 管理员（domain_admin/global_admin）同为做题界面 + 顶栏管理提示。
export default function App() {
  const [authed, setAuthed] = useState<boolean | null>(null)
  const [user, setUser] = useState<User | null>(null)

  useEffect(() => {
    api.me().then((d) => {
      setAuthed(d.authenticated)
      setUser(d.user ?? null)
    }).catch(() => setAuthed(false))
    const on401 = () => {
      queryClient.clear() // 会话失效：清空缓存，防换账号残留上一账号的空间/排行数据
      setAuthed(false)
      setUser(null)
      window.location.href = '/login'
    }
    window.addEventListener('quiz:unauthorized', on401)
    return () => window.removeEventListener('quiz:unauthorized', on401)
  }, [])

  if (authed === null) {
    return <div className="flex h-dvh items-center justify-center text-sm text-muted-foreground">正在连接刷题服务…</div>
  }

  const onLogout = () =>
    void api.logout().finally(() => {
      queryClient.clear()
      setAuthed(false)
      window.location.href = '/login'
    })

  return (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        {authed && user ? (
          <Suspense fallback={<PageFallback />}>
            <Routes>
              <Route path="/login" element={<Navigate to="/" replace />} />

              {/* 非空间页：顶部壳（品牌 + 我的） */}
              <Route path="/" element={<TopShell user={user} onLogout={onLogout} />}>
                <Route index element={<SpacePicker user={user} />} />
                <Route path="mine" element={<MyPage />} />
                <Route path="problem/:problemId" element={<ProblemSolvePage />} />
              </Route>

              {/* 空间壳：/s/:spaceId/* */}
              <Route path="/s/:spaceId" element={<SpaceShell user={user} onLogout={onLogout} />}>
                <Route index element={<Navigate to="training" replace />} />
                <Route path="training" element={<TrainingList />} />
                <Route path="training/:trainingId" element={<TrainingDetail />} />
                <Route path="practice" element={<PracticeList />} />
                <Route path="practice/:practiceId" element={<PracticeSolve />} />
                <Route path="quiz" element={<QuizList />} />
                <Route path="quiz/:quizId" element={<QuizSolve />} />
                <Route path="rank" element={<RankPage />} />
              </Route>

              <Route path="*" element={<Navigate to="/" replace />} />
            </Routes>
          </Suspense>
        ) : (
          <Login onSuccess={(u) => { setUser(u); setAuthed(true) }} />
        )}
      </BrowserRouter>
      <Toaster position="top-center" richColors closeButton />
    </QueryClientProvider>
  )
}

export type ShellContext = { user: User; onLogout: () => void }

// 非空间页的轻量顶壳：品牌 → 我的（个人入口）；「我的」页含账号与退出。
function TopShell({ user, onLogout }: { user: User; onLogout: () => void }) {
  return (
    <div className="flex h-dvh flex-col overflow-hidden">
      <header className="shrink-0 border-b bg-background">
        <div className="mx-auto flex h-14 w-full max-w-6xl items-center gap-4 px-4 lg:px-6">
          <NavLink to="/" className="flex shrink-0 items-center gap-2 text-base font-semibold">
            <img src="/favicon.png" alt="OrangeOJ" className="size-7 rounded-lg" />
            OrangeOJ
          </NavLink>
          <span className="hidden text-xs text-muted-foreground sm:inline">刷题门户</span>
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
            {user.role === 'member' ? null : (
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
