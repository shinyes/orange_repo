import { useEffect, useState } from 'react'
import { QueryClient, QueryClientProvider, useQuery } from '@tanstack/react-query'
import { Toaster } from '@/components/ui/sonner'
import { BookOpenIcon, ListChecksIcon, TagsIcon } from 'lucide-react'

import { api } from '@/lib/api'
import { AppStateProvider, useAppState } from '@/lib/app-context'
import { Login } from '@/components/Login'
import { PasswordDialog } from '@/components/PasswordDialog'
import { Sidebar } from '@/components/Sidebar'
import { ProblemPane } from '@/components/ProblemPane'
import { PracticeDetail, TrainingDetail } from '@/components/GroupsPane'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: { retry: 1, staleTime: 5_000 },
  },
})

export default function App() {
  const [authed, setAuthed] = useState<boolean | null>(null)

  useEffect(() => {
    api.me().then((d) => setAuthed(d.authenticated)).catch(() => setAuthed(false))
    const on401 = () => setAuthed(false)
    window.addEventListener('orangerepo:unauthorized', on401)
    return () => window.removeEventListener('orangerepo:unauthorized', on401)
  }, [])

  if (authed === null) {
    return <div className="flex h-screen items-center justify-center text-sm text-muted-foreground">正在连接题库…</div>
  }

  return (
    <QueryClientProvider client={queryClient}>
      {authed ? (
        <AppStateProvider>
          <Main onLogout={() => void api.logout().finally(() => setAuthed(false))} />
        </AppStateProvider>
      ) : (
        <Login onSuccess={() => setAuthed(true)} />
      )}
      <Toaster position="top-center" richColors closeButton />
    </QueryClientProvider>
  )
}

function Main({ onLogout }: { onLogout: () => void }) {
  const [pwOpen, setPwOpen] = useState(false)
  return (
    <div className="flex h-screen overflow-hidden">
      <aside className="w-[340px] shrink-0 border-r">
        <Sidebar onLogout={onLogout} onOpenSettings={() => setPwOpen(true)} />
      </aside>
      <main className="min-w-0 flex-1 overflow-y-auto">
        <RightPane />
      </main>
      <PasswordDialog open={pwOpen} onOpenChange={setPwOpen} />
    </div>
  )
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

// 空态首页：统计 + 快速指引。
function EmptyState() {
  const problems = useQuery({ queryKey: ['problems', 'all'], queryFn: () => api.problems({ q: '', tags: [], type: '' }) })
  const tags = useQuery({ queryKey: ['tags'], queryFn: () => api.tags() })
  const trainings = useQuery({ queryKey: ['trainings'], queryFn: api.trainings })
  const practices = useQuery({ queryKey: ['practices'], queryFn: api.practices })

  const stats = [
    { icon: BookOpenIcon, label: '题目', value: problems.data?.problems.length ?? '…' },
    { icon: TagsIcon, label: '标签节点', value: tags.data?.tags.length ?? '…' },
    {
      icon: ListChecksIcon,
      label: '训练 / 练习',
      value: trainings.data && practices.data ? `${trainings.data.trainings.length} / ${practices.data.practices.length}` : '…',
    },
  ]

  return (
    <div className="mx-auto max-w-3xl px-6 py-12">
      <div className="mb-8 text-center">
        <div className="mb-3 text-5xl">🍊</div>
        <h1 className="text-2xl font-semibold">OrangeRepo 题库</h1>
        <p className="mt-2 text-sm text-muted-foreground">嵌套标签管理 · 标签树筛选 · OrangeOJ 格式双向兼容</p>
      </div>

      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        {stats.map((s) => (
          <div key={s.label} className="rounded-xl border p-4 text-center">
            <s.icon className="mx-auto mb-2 size-5 text-primary" />
            <div className="text-xl font-semibold">{s.value}</div>
            <div className="text-xs text-muted-foreground">{s.label}</div>
          </div>
        ))}
      </div>

      <div className="mt-10 rounded-xl border bg-muted/30 p-5">
        <h2 className="mb-3 text-sm font-medium">快速上手</h2>
        <ol className="space-y-2.5 text-sm text-muted-foreground">
          <li className="flex gap-2.5">
            <Step n={1} /> 左栏「新建题目」或上传按钮导入 OrangeOJ ZIP 题包；标签支持斜杠层级（如 数学/几何）
          </li>
          <li className="flex gap-2.5">
            <Step n={2} /> 点击题目查看题面（支持 KaTeX 公式）、答案与题解，在「编辑」页修改
          </li>
          <li className="flex gap-2.5">
            <Step n={3} /> 勾选多道题目，通过「加入训练 / 加入练习」编制计划并导出 ZIP
          </li>
          <li className="flex gap-2.5">
            <Step n={4} /> 用标签树、搜索框与类型过滤快速定位题目；导出结果可直接导入 OrangeOJ
          </li>
        </ol>
      </div>
    </div>
  )
}

function Step({ n }: { n: number }) {
  return (
    <span className="flex size-5 shrink-0 items-center justify-center rounded-full bg-primary text-xs font-medium text-primary-foreground">
      {n}
    </span>
  )
}
