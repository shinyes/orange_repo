import { lazy, Suspense, useEffect, useRef, useState } from 'react'
import { QueryClient, QueryClientProvider, useQuery } from '@tanstack/react-query'
import { Toaster } from '@/components/ui/sonner'
import { BookOpenIcon, ListChecksIcon, TagsIcon } from 'lucide-react'

import { api } from '@/lib/api'
import { AppStateProvider, useAppState } from '@/lib/app-context'
import { DomainProvider, useDomain } from '@/lib/domain-context'
import { Login } from '@/components/Login'
import { PasswordDialog } from '@/components/PasswordDialog'
import { ProblemListColumn, TagFilterColumn } from '@/components/Sidebar'

// 右侧详情/编辑区按需懒加载（ProblemPane 含编辑器与 KaTeX 渲染，体积大且仅点选题目后需要）。
const ProblemPane = lazy(() => import('@/components/ProblemPane').then((m) => ({ default: m.ProblemPane })))
const BookletColumn = lazy(() => import('@/components/BookletColumn').then((m) => ({ default: m.BookletColumn })))
const TrainingDetail = lazy(() => import('@/components/GroupsPane').then((m) => ({ default: m.TrainingDetail })))
const PracticeDetail = lazy(() => import('@/components/GroupsPane').then((m) => ({ default: m.PracticeDetail })))
// OJ 重构：全宽管理页（域管理 = 系统管理员；空间管理 = 域管理员 / 系统管理员选中域后）
const DomainAdmin = lazy(() => import('@/components/DomainAdmin').then((m) => ({ default: m.DomainAdmin })))
const SpaceAdmin = lazy(() => import('@/components/SpaceAdmin').then((m) => ({ default: m.SpaceAdmin })))

// 右栏懒加载占位。
function PaneFallback() {
  return <div className="flex h-full items-center justify-center text-sm text-muted-foreground">加载中…</div>
}

const queryClient = new QueryClient({
  defaultOptions: {
    queries: { retry: 1, staleTime: 5_000 },
  },
})

export default function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <DomainProvider>
        <Shell />
      </DomainProvider>
      <Toaster position="top-center" richColors closeButton />
    </QueryClientProvider>
  )
}

// 会话状态机：加载中（"正在连接"）→ 登录 / 主界面。
function Shell() {
  const { status, refresh } = useDomain()
  if (status === 'loading') {
    return <div className="flex h-screen items-center justify-center text-sm text-muted-foreground">正在连接…</div>
  }
  if (status === 'anon') {
    return <Login onSuccess={() => void refresh()} />
  }
  return (
    <AppStateProvider>
      <Main />
    </AppStateProvider>
  )
}

function Main() {
  const [pwOpen, setPwOpen] = useState(false)
  const [showBooklets, setShowBooklets] = useState(false)
  const { view, goHome } = useAppState()
  const { user, domainId, logout } = useDomain()
  const role = user?.role ?? null

  // 打开训练/练习时自动展开题册列
  useEffect(() => {
    if (view.kind === 'training' || view.kind === 'practice') setShowBooklets(true)
  }, [view.kind === 'training', view.kind === 'practice'])

  // 切换域后回到首页：旧域打开的题目/题册详情不再属于当前域
  const lastDomain = useRef<number | null>(null)
  useEffect(() => {
    if (lastDomain.current !== null && lastDomain.current !== domainId) {
      goHome()
      setShowBooklets(false)
    }
    lastDomain.current = domainId
  }, [domainId, goHome])

  // 全宽管理页：隐藏三栏工作区，仅渲染管理内容
  const adminPage = view.kind === 'domainadmin' || view.kind === 'spaceadmin'
  // 系统管理员尚未选域：仅第一栏（含「请选择域」+ 域下拉），不渲染数据栏/右栏避免跨域读取
  const needsDomainPick = role === 'global_admin' && domainId == null

  if (adminPage) {
    return (
      <div className="h-screen overflow-hidden">
        <main className="h-full overflow-y-auto">
          <Suspense fallback={<PaneFallback />}>
            {view.kind === 'domainadmin' && <DomainAdmin />}
            {view.kind === 'spaceadmin' && <SpaceAdmin />}
          </Suspense>
        </main>
        <PasswordDialog open={pwOpen} onOpenChange={setPwOpen} />
      </div>
    )
  }

  return (
    <div className="flex h-screen overflow-hidden">
      <aside className={needsDomainPick ? 'w-[min(420px,90vw)] shrink-0 border-r' : 'w-[270px] shrink-0 border-r'}>
        <TagFilterColumn onLogout={() => void logout()} onOpenSettings={() => setPwOpen(true)} />
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
        <img src="/favicon.png" alt="OrangeOJ" className="mx-auto mb-3 size-16 rounded-2xl" />
        <h1 className="text-2xl font-semibold">OrangeOJ</h1>
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
