import { useEffect, useState } from 'react'
import { BrowserRouter, Navigate, NavLink, Outlet, Route, Routes } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { BookOpenIcon, ClipboardXIcon, FolderKanbanIcon, ClipboardListIcon, UserRoundIcon } from 'lucide-react'

import { api } from '@/lib/api'
import { Toaster } from '@/components/ui/sonner'
import { Login } from '@/components/Login'
import { QuizSubjectsPage } from '@/pages/QuizSubjectsPage'
import { QuizCategoriesPage } from '@/pages/QuizCategoriesPage'
import { QuizRoundPage } from '@/pages/QuizRoundPage'
import { WrongListPage } from '@/pages/WrongListPage'
import { WrongRoundPage } from '@/pages/WrongRoundPage'
import { MyPage } from '@/pages/MyPage'
import { AdminPage } from '@/pages/AdminPage'
import { TrainingCards, PracticeCards } from '@/pages/oj/AssignmentCards'
import { TrainingPage } from '@/pages/oj/TrainingPage'
import { PracticePage } from '@/pages/oj/PracticePage'
import { ProblemSolvePage } from '@/pages/oj/ProblemSolvePage'
import type { User } from '@/lib/types'
import { cn } from '@/lib/utils'

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
    <div className="mx-auto w-full max-w-2xl px-4 py-6">
      <h1 className="mb-1 text-lg font-semibold">训练</h1>
      <p className="mb-4 text-xs text-muted-foreground">按章节组织的布置训练（编程题提交评测，客观题即答即判）</p>
      <TrainingCards />
    </div>
  )
}

function PracticeHome() {
  return (
    <div className="mx-auto w-full max-w-2xl px-4 py-6">
      <h1 className="mb-1 text-lg font-semibold">练习</h1>
      <p className="mb-4 text-xs text-muted-foreground">布置给本班的练习题单</p>
      <PracticeCards />
    </div>
  )
}

// 主壳：内容区（Outlet）+ 底部导航（刷题/训练/练习/错题/我的）。
function MainShell({ user, onLogout }: { user: User; onLogout: () => void }) {
  const items: { to: string; label: string; icon: typeof BookOpenIcon }[] = [
    { to: '/quiz', label: '刷题', icon: BookOpenIcon },
    { to: '/training', label: '训练', icon: FolderKanbanIcon },
    { to: '/practice', label: '练习', icon: ClipboardListIcon },
    { to: '/wrong', label: '错题', icon: ClipboardXIcon },
    { to: '/mine', label: '我的', icon: UserRoundIcon },
  ]
  return (
    <div className="flex h-dvh flex-col overflow-hidden">
      <main className="min-h-0 flex-1 overflow-y-auto">
        <Outlet context={{ user, onLogout }} />
      </main>
      <nav className="flex shrink-0 border-t bg-background px-1 pb-[env(safe-area-inset-bottom)] sm:px-4">
        {items.map((it) => (
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