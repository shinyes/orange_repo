import { lazy, Suspense, useEffect, useState } from 'react'
import { BrowserRouter, Navigate, NavLink, Outlet, Route, Routes } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { BookOpenIcon, ClipboardXIcon, FolderKanbanIcon, ClipboardListIcon, UserRoundIcon } from 'lucide-react'

import { api } from '@/lib/api'
import { Toaster } from '@/components/ui/sonner'
import { Login } from '@/components/Login'
import { QuizSubjectsPage } from '@/pages/QuizSubjectsPage'
import { QuizCategoriesPage } from '@/pages/QuizCategoriesPage'
import { WrongListPage } from '@/pages/WrongListPage'
import { MyPage } from '@/pages/MyPage'
import { TrainingCards, PracticeCards } from '@/pages/oj/AssignmentCards'
import type { User } from '@/lib/types'
import { cn } from '@/lib/utils'

// 重量级页面按路由懒加载（做题/管理/错题轮等大组件不进首屏主包）。
const QuizRoundPage = lazy(() => import('@/pages/QuizRoundPage').then((m) => ({ default: m.QuizRoundPage })))
const WrongRoundPage = lazy(() => import('@/pages/WrongRoundPage').then((m) => ({ default: m.WrongRoundPage })))
const AdminPage = lazy(() => import('@/pages/AdminPage').then((m) => ({ default: m.AdminPage })))
const TrainingPage = lazy(() => import('@/pages/oj/TrainingPage').then((m) => ({ default: m.TrainingPage })))
const PracticePage = lazy(() => import('@/pages/oj/PracticePage').then((m) => ({ default: m.PracticePage })))
const ProblemSolvePage = lazy(() => import('@/pages/oj/ProblemSolvePage').then((m) => ({ default: m.ProblemSolvePage })))

// 路由切换时的轻量加载占位。
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

// 路由驱动的刷题应用：每层页面都有独立 URL，浏览器返回/前进键全程可用。
export default function App() {
  const [authed, setAuthed] = useState<boolean | null>(null)
  const [user, setUser] = useState<User | null>(null)

  useEffect(() => {
    api.me().then((d) => {
      setAuthed(d.authenticated)
      setUser(d.user ?? null)
    }).catch(() => setAuthed(false))
    const on401 = () => {
      setAuthed(false)
      setUser(null)
    }
    window.addEventListener('quiz:unauthorized', on401)
    return () => window.removeEventListener('quiz:unauthorized', on401)
  }, [])

  if (authed === null) {
    return <div className="flex h-dvh items-center justify-center text-sm text-muted-foreground">正在连接刷题服务…</div>
  }

  return (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        {authed && user ? (
          <Suspense fallback={<PageFallback />}>
            <Routes>
              <Route path="/login" element={<Navigate to="/quiz" replace />} />
              <Route element={<MainShell user={user} onLogout={() => void api.logout().finally(() => setAuthed(false))} />}>
                <Route path="/" element={<Navigate to="/quiz" replace />} />
                <Route path="/quiz" element={<QuizSubjectsPage />} />
                <Route path="/quiz/:subjectId" element={<QuizCategoriesPage />} />
                <Route path="/quiz/:subjectId/:categoryId" element={<QuizRoundPage />} />
                <Route path="/training" element={<TrainingHome />} />
                <Route path="/training/:id" element={<TrainingPage />} />
                <Route path="/practice" element={<PracticeHome />} />
                <Route path="/practice/:id" element={<PracticePage />} />
                <Route path="/problem/:problemId" element={<ProblemSolvePage />} />
                <Route path="/wrong" element={<WrongListPage />} />
                <Route path="/wrong/:scope" element={<WrongRoundPage />} />
                <Route path="/mine" element={<MyPage />} />
                <Route
                  path="/admin"
                  element={user.role === 'admin' ? <AdminPage /> : <Navigate to="/mine" replace />}
                />
                <Route path="*" element={<Navigate to="/quiz" replace />} />
              </Route>
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

// 训练 / 练习 首页（共用卡片列表，仅文案差异）。
function TrainingHome() {
  return (
    <div className="mx-auto w-full max-w-2xl px-4 py-6 lg:max-w-4xl lg:px-8 lg:py-8">
      <h1 className="mb-1 text-lg font-semibold">训练</h1>
      <p className="mb-4 text-xs text-muted-foreground">按章节组织的布置训练（编程题提交评测，客观题即答即判）</p>
      <TrainingCards />
    </div>
  )
}

function PracticeHome() {
  return (
    <div className="mx-auto w-full max-w-2xl px-4 py-6 lg:max-w-4xl lg:px-8 lg:py-8">
      <h1 className="mb-1 text-lg font-semibold">练习</h1>
      <p className="mb-4 text-xs text-muted-foreground">布置给本班的练习题单</p>
      <PracticeCards />
    </div>
  )
}

// 主壳：PC（lg+）顶部导航 + 内容区；移动端底部导航（同一套路由）。
function MainShell({ user, onLogout }: { user: User; onLogout: () => void }) {
  // 导航项（不含「我的」：PC 放最右个人入口，移动端固定在底部最右）
  const items: { to: string; label: string; icon: typeof BookOpenIcon }[] = [
    { to: '/quiz', label: '刷题', icon: BookOpenIcon },
    { to: '/training', label: '训练', icon: FolderKanbanIcon },
    { to: '/practice', label: '练习', icon: ClipboardListIcon },
    { to: '/wrong', label: '错题', icon: ClipboardXIcon },
  ]
  const mineItem = { to: '/mine', label: '我的', icon: UserRoundIcon }
  return (
    <div className="flex h-dvh flex-col overflow-hidden">
      {/* PC 顶部导航 */}
      <header className="hidden shrink-0 border-b bg-background lg:block">
        <div className="mx-auto flex h-14 w-full max-w-6xl items-center gap-6 px-6">
          <NavLink to="/quiz" className="flex shrink-0 items-center gap-2 text-base font-semibold">
            <img src="/favicon.png" alt="OrangeOJ" className="size-7 rounded-lg" />
            OrangeOJ
          </NavLink>
          <nav className="flex min-w-0 flex-1 items-center gap-1">
            {items.map((it) => (
              <NavLink
                key={it.to}
                to={it.to}
                className={({ isActive }) =>
                  cn(
                    'flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm transition-colors',
                    isActive
                      ? 'bg-primary/10 font-medium text-primary'
                      : 'text-muted-foreground hover:bg-muted hover:text-foreground',
                  )
                }
              >
                <it.icon className="size-4" />
                {it.label}
              </NavLink>
            ))}
          </nav>
          {/* 最右：我的（个人入口）；管理入口保留在「我的」页内 */}
          <NavLink
            to={mineItem.to}
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
            <span className="max-w-28 truncate">{user.username}</span>
            {user.role === 'admin' && <span className="rounded bg-muted px-1 py-0.5 text-[10px]">管理</span>}
          </NavLink>
        </div>
      </header>

      {/* 内容区 */}
      <main className="min-h-0 flex-1 overflow-y-auto">
        <Outlet context={{ user, onLogout }} />
      </main>

      {/* 移动端底部导航 */}
      <nav className="flex shrink-0 border-t bg-background px-1 pb-[env(safe-area-inset-bottom)] lg:hidden">
        {[...items, mineItem].map((it) => (
          <NavLink
            key={it.to}
            to={it.to}
            className={({ isActive }) =>
              cn(
                'flex flex-1 flex-col items-center gap-0.5 py-2 text-[10px] transition-colors sm:gap-1 sm:text-xs',
                isActive ? 'text-primary' : 'text-muted-foreground hover:text-foreground',
              )
            }
          >
            <it.icon className="size-5" />
            {it.label}
          </NavLink>
        ))}
      </nav>
    </div>
  )
}