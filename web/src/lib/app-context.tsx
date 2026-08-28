// 全局应用状态：当前视图、题目过滤、勾选集。
import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from 'react'
import type { ProblemFilterState } from './types'

export type View =
  | { kind: 'empty' }
  | { kind: 'problem'; id: number }
  | { kind: 'training'; id: number }
  | { kind: 'practice'; id: number }

interface AppState {
  view: View
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

const Ctx = createContext<AppState | null>(null)

const DEFAULT_FILTER: ProblemFilterState = { q: '', tags: [], type: '' }

export function AppStateProvider({ children }: { children: ReactNode }) {
  const [view, setView] = useState<View>({ kind: 'empty' })
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

export function useAppState(): AppState {
  const v = useContext(Ctx)
  if (!v) throw new Error('useAppState must be used within AppStateProvider')
  return v
}
