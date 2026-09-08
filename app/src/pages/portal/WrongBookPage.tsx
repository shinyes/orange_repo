// 全局错题集：按刷题项目分组展示；点「重刷」进入该组（或全部）错题重刷——
// 单题即时判分，答对即从错题集移除并自动下一题；答错可再试/换一题。
import { useEffect, useRef, useState } from 'react'
import { useParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { ArrowLeftIcon, BookOpenIcon , Loader2Icon, PartyPopperIcon, RefreshCwIcon, SkipForwardIcon } from 'lucide-react'
import { toast } from 'sonner'

import { api } from '@/api'
import type { CorrectAnswer, ObjectiveAnswer, QuizProblemResponse, WrongGroup } from '@/api/types'
import { useSpaceById } from '@/pages/portal/portal-context'
import { PageContainer, SpacePageShell } from '@/pages/portal/SpacePageShell'
import { ObjectiveQuestion } from '@/components/portal/objective'
import { Button } from '@/components/ui/button'

export function WrongBookPage() {
  const { spaceId } = useParams()
  const sid = Number(spaceId)
  const space = useSpaceById(sid)
  const [practice, setPractice] = useState<{ mode: 'all' } | { mode: 'quiz'; group: WrongGroup } | null>(null)

  if (space === 'loading') return <Center text="加载中…" />
  if (space === null) return <Center text="空间不存在或无权访问" />

  return (
    <SpacePageShell spaceId={sid} backTo={`/s/${sid}/quiz`} backLabel="返回刷题列表">
      {practice ? (
        <WrongPractice
          group={practice.mode === 'quiz' ? practice.group : null}
          onExit={() => setPractice(null)}
        />
      ) : (
        <WrongList onPractice={(g) => setPractice(g ? { mode: 'quiz', group: g } : { mode: 'all' })} />
      )}
    </SpacePageShell>
  )
}

function Center({ text }: { text: string }) {
  return <div className="flex h-dvh items-center justify-center text-sm text-muted-foreground">{text}</div>
}

// ---------- 错题集列表（按项目分组 + 全部重刷入口） ----------

function WrongList({ onPractice }: { onPractice: (group: WrongGroup | null) => void }) {
  const q = useQuery({ queryKey: ['wrong-book'], queryFn: () => api.portalWrongBook() })
  const data = q.data
  return (
    <PageContainer className="max-w-3xl">
      <div className="mt-3 flex flex-wrap items-center justify-between gap-2">
        <h1 className="flex items-center gap-2 text-lg font-semibold">
          <BookOpenIcon className="size-5 text-primary" /> 错题集
          {data && data.total > 0 && (
            <span className="rounded-full bg-red-50 px-2 py-0.5 text-xs font-semibold text-red-600">共 {data.total} 题</span>
          )}
        </h1>
        <Button variant="ghost" size="sm" className="text-muted-foreground" onClick={() => onPractice(null)}>
          全部错题重刷 <SkipForwardIcon className="size-3.5" />
        </Button>
      </div>
      <p className="mt-1 text-xs text-muted-foreground">
        刷题答错的题目自动收进错题集；在任何一处答对后自动移出（包括本页重刷）
      </p>
      {q.isLoading && (
        <div className="flex items-center justify-center gap-2 py-16 text-sm text-muted-foreground">
          <Loader2Icon className="size-4 animate-spin" /> 加载中…
        </div>
      )}
      {q.isError && (
        <div className="py-16 text-center text-sm text-red-500">
          加载失败{q.error instanceof Error ? `：${q.error.message}` : ''}
          <button type="button" className="ml-2 text-primary underline" onClick={() => void q.refetch()}>重试</button>
        </div>
      )}
      {!q.isLoading && !q.isError && (!data || data.groups.length === 0) && (
        <div className="mt-6 rounded-xl border border-dashed p-12 text-center text-sm text-muted-foreground">
          暂无错题 🎉 刷题答错后会自动收进这里
        </div>
      )}
      <div className="mt-4 space-y-2">
        {data?.groups.map((g) => (
          <button
            key={g.quizId}
            type="button"
            onClick={() => onPractice(g)}
            className="flex w-full items-center gap-3 rounded-xl border bg-card p-4 text-left transition-colors hover:border-primary/50"
          >
            <span className="flex size-9 shrink-0 items-center justify-center rounded-full bg-red-50 text-sm font-bold text-red-600">
              {g.count}
            </span>
            <span className="min-w-0 flex-1">
              <span className="block truncate text-sm font-semibold">{g.title}</span>
              <span className="block text-xs text-muted-foreground">
                {g.spaceName ? `空间「${g.spaceName}」` : ''} · 错题 {g.count} 道
              </span>
            </span>
            <span className="shrink-0 text-xs font-medium text-primary">重刷 →</span>
          </button>
        ))}
      </div>
    </PageContainer>
  )
}

// ---------- 错题重刷（单题即时判分） ----------

function WrongPractice({ group, onExit }: { group: WrongGroup | null; onExit: () => void }) {
  const [problem, setProblem] = useState<QuizProblemResponse['problem'] | null>(null)
  const [loading, setLoading] = useState(true)
  const [done, setDone] = useState(false)
  const [busy, setBusy] = useState(false)
  const [selected, setSelected] = useState<ObjectiveAnswer | null>(null)
  const [feedback, setFeedback] = useState<{ correct: boolean; correctAnswer?: CorrectAnswer } | null>(null)

  const label = group ? `「${group.title}」错题重刷` : '全部错题重刷'
  const aliveRef = useRef(true)
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  useEffect(() => {
    aliveRef.current = true
    return () => {
      aliveRef.current = false
      if (timerRef.current) clearTimeout(timerRef.current)
    }
  }, [])

  async function next() {
    if (!aliveRef.current) return
    setLoading(true)
    setFeedback(null)
    setSelected(null)
    setDone(false)
    try {
      const r = await api.portalWrongNext(group?.quizId)
      if (!aliveRef.current) return
      if (r.done || !r.problem) {
        setDone(true)
        setProblem(null)
      } else {
        setProblem(r.problem)
      }
    } catch (err) {
      if (aliveRef.current) toast.error(err instanceof Error ? err.message : '抽题失败')
    } finally {
      if (aliveRef.current) setLoading(false)
    }
  }

  useEffect(() => {
    void next()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [group?.quizId])

  async function answer(a: ObjectiveAnswer) {
    if (!problem || busy || feedback || !aliveRef.current) return
    setSelected(a)
    setBusy(true)
    try {
      const r = await api.portalWrongAnswer(problem.id, a)
      if (!aliveRef.current) return
      setFeedback(r)
      if (r.correct) {
        toast.success('答对，已从错题集移除')
        timerRef.current = setTimeout(() => void next(), 700)
      } else {
        toast.error('回答错误，错题保留')
      }
    } catch (err) {
      if (!aliveRef.current) return
      toast.error(err instanceof Error ? err.message : '提交失败')
      setSelected(null)
    } finally {
      if (aliveRef.current) setBusy(false)
    }
  }

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="shrink-0 border-b bg-background shadow-sm">
        <div className="mx-auto flex h-14 w-full max-w-2xl items-center gap-2 px-4">
          <Button variant="ghost" size="sm" className="text-muted-foreground" onClick={onExit}>
            <ArrowLeftIcon className="size-4" /> 错题集
          </Button>
          <span className="min-w-0 flex-1 truncate text-sm font-semibold">{label}</span>
        </div>
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto">
        <PageContainer className="max-w-2xl">
          <div className="mt-3 rounded-2xl border bg-card p-5">
            {done ? (
              <div className="py-12 text-center">
                <div className="mx-auto mb-3 flex size-14 items-center justify-center rounded-full bg-emerald-50">
                  <PartyPopperIcon className="size-7 text-emerald-600" />
                </div>
                <h2 className="text-lg font-semibold">错题已清空！</h2>
                <p className="mt-2 text-sm text-muted-foreground">所有错题都已答对并移出错题集</p>
                <Button className="mt-5" onClick={onExit}>返回错题集</Button>
              </div>
            ) : loading ? (
              <div className="flex items-center justify-center gap-2 py-12 text-sm text-muted-foreground">
                <Loader2Icon className="size-4 animate-spin" /> 正在取题…
              </div>
            ) : problem ? (
              <div>
                <ObjectiveQuestion
                  problem={problem}
                  selected={selected}
                  feedback={feedback}
                  busy={busy}
                  onSelect={(a) => void answer(a)}
                />
                {feedback && !feedback.correct && (
                  <div className="mt-5 flex flex-wrap justify-end gap-2">
                    <Button variant="outline" onClick={() => { setFeedback(null); setSelected(null) }}>
                      <RefreshCwIcon className="size-4" /> 再试一次
                    </Button>
                    <Button onClick={() => void next()}>
                      换一题 <SkipForwardIcon className="size-4" />
                    </Button>
                  </div>
                )}
              </div>
            ) : null}
          </div>
        </PageContainer>
      </div>
    </div>
  )
}
