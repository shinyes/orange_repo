import { useEffect, useState } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { BookOpenIcon, ClipboardXIcon, SettingsIcon, UserRoundIcon } from 'lucide-react'

import { api } from '@/lib/api'
import { Toaster } from '@/components/ui/sonner'
import { Login } from '@/components/Login'
import { QuizPage } from '@/pages/QuizPage'
import { WrongPage } from '@/pages/WrongPage'
import { MyPage } from '@/pages/MyPage'
import { AdminPage } from '@/pages/AdminPage'
import type { User } from '@/lib/types'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: { retry: 1, staleTime: 0 },
  },
})

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
    return <div className="flex h-screen items-center justify-center text-sm text-muted-foreground">正在连接刷题服务…</div>
  }

  return (
    <QueryClientProvider client={queryClient}>
      {authed && user ? (
        <Main user={user} onLogout={() => void api.logout().finally(() => setAuthed(false))} />
      ) : (
        <Login onSuccess={(u) => { setUser(u); setAuthed(true) }} />
      )}
      <Toaster position="top-center" richColors closeButton />
    </QueryClientProvider>
  )
}

export type Tab = 'quiz' | 'wrong' | 'mine' | 'admin'

function Main({ user, onLogout }: { user: User; onLogout: () => void }) {
  const [tab, setTab] = useState<Tab>('quiz')
  const effectiveTab: Tab = tab === 'admin' && user.role !== 'admin' ? 'mine' : tab

  return (
    <div className="flex h-screen flex-col overflow-hidden">
      <main className="min-h-0 flex-1 overflow-y-auto">
        {effectiveTab === 'quiz' && <QuizPage />}
        {effectiveTab === 'wrong' && <WrongPage />}
        {effectiveTab === 'mine' && <MyPage user={user} onOpenAdmin={() => setTab('admin')} onLogout={onLogout} />}
        {effectiveTab === 'admin' && user.role === 'admin' && <AdminPage />}
      </main>
      <BottomNav tab={effectiveTab} user={user} onSelect={setTab} />
    </div>
  )
}

function BottomNav({ tab, user, onSelect }: { tab: Tab; user: User; onSelect: (t: Tab) => void }) {
  const items: { key: Tab; label: string; icon: typeof BookOpenIcon }[] = [
    { key: 'quiz', label: '刷题', icon: BookOpenIcon },
    { key: 'wrong', label: '错题', icon: ClipboardXIcon },
    { key: 'mine', label: '我的', icon: UserRoundIcon },
  ]
  if (user.role === 'admin') {
    items.push({ key: 'admin', label: '管理', icon: SettingsIcon })
  }
  return (
    <nav className="flex shrink-0 border-t bg-background px-4 pb-[env(safe-area-inset-bottom)]">
      {items.map((it) => (
        <button
          key={it.key}
          type="button"
          onClick={() => onSelect(it.key)}
          className={`flex flex-1 flex-col items-center gap-1 py-2.5 text-xs transition-colors ${
            tab === it.key ? 'text-primary' : 'text-muted-foreground hover:text-foreground'
          }`}
        >
          <it.icon className="size-5" />
          {it.label}
        </button>
      ))}
    </nav>
  )
}