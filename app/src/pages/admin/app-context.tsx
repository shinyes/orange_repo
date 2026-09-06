// 管理工作区上下文（src/pages/admin/app-context.tsx）：迁自原管理端前端 app-context 并路由化适配。
// 差异：
//  - web 的 View 含 domainadmin/spaceadmin（全宽管理页由 view.kind 切换）——现由 URL 路由承担
//    （/admin/domains、/admin/spaces），不再进入本上下文；
//  - 工作区内右栏详情选择（题目/训练/练习/空）保留为上下文状态（URL 不承载高频点选），
//    方法名/行为与 web 一致（openProblem/openTraining/openPractice/goHome/filter/checked），迁移组件零改动可用。
import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from 'react'
import type { ProblemFilterState } from '@/api/types'

export type WorkspaceDetail =
  | { kind: 'empty' }
  | { kind: 'problem'; id: number }
  | { kind: 'training'; id: number }
  | { kind: 'practice'; id: number }

interface WorkspaceState {
  view: WorkspaceDetail
  openProblem: (id: number) => void
  openTraining: (id: number) => void
  openPractice: (id: number) => void
  goHome: () => void
  filter: ProblemFilterState
  patchFilter: (patch: Partial<ProblemFilterState>) => void
  checked: number[]
  toggleChecked: (id: number) => void
  setChecked: (ids: number[]) => void
  clearChecked: () => void
}

const Ctx = createContext<WorkspaceState | null>(null)

const DEFAULT_FILTER: ProblemFilterState = { q: '', tags: [], type: '' }

export function AppStateProvider({ children }: { children: ReactNode }) {
  const [view, setView] = useState<WorkspaceDetail>({ kind: 'empty' })
  const [filter, setFilter] = useState<ProblemFilterState>(DEFAULT_FILTER)
  const [checked, setChecked] = useState<number[]>([])

  const patchFilter = useCallback((patch: Partial<ProblemFilterState>) => {
    setFilter((prev) => ({ ...prev, ...patch }))
  }, [])
  const openProblem = useCallback((id: number) => setView({ kind: 'problem', id }), [])
  const openTraining = useCallback((id: number) => setView({ kind: 'training', id }), [])
  const openPractice = useCallback((id: number) => setView({ kind: 'practice', id }), [])
  const goHome = useCallback(() => setView({ kind: 'empty' }), [])
  const toggleChecked = useCallback((id: number) => {
    setChecked((prev) => (prev.includes(id) ? prev.filter((x) => x !== id) : [...prev, id]))
  }, [])
  const setCheckedIds = useCallback((ids: number[]) => setChecked(ids), [])
  const clearChecked = useCallback(() => setChecked([]), [])

  const value = useMemo(
    () => ({ view, openProblem, openTraining, openPractice, goHome, filter, patchFilter, checked, toggleChecked, setChecked: setCheckedIds, clearChecked }),
    [view, openProblem, openTraining, openPractice, goHome, filter, patchFilter, checked, toggleChecked, setCheckedIds, clearChecked],
  )
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>
}

export function useAppState(): WorkspaceState {
  const v = useContext(Ctx)
  if (!v) throw new Error('useAppState must be used within AppStateProvider')
  return v
}
