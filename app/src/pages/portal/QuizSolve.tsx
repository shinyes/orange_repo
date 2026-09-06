import { useEffect, useState } from 'react'
import { useParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { BookOpenIcon, CheckCircle2Icon, Loader2Icon, PartyPopperIcon, RefreshCwIcon, SkipForwardIcon } from 'lucide-react'
import { toast } from 'sonner'

import { api } from '@/api'
import type { CorrectAnswer, ObjectiveAnswer, QuizProblemResponse } from '@/api/types'
import { useSpaceById } from './portal-context'
import { PageContainer, SpacePageShell } from './SpacePageShell'
import { ObjectiveQuestion } from '@/components/portal/objective'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

// 刷题单题流：进入即取一题（GET problem），作答（POST answer）即时反馈；
// 答对后「下一题」；答错可「再试一次」或「换一题」；done=true 显示完成页。
export function QuizSolve() {
  const { spaceId, quizId } = useParams()
  const sid = Number(spaceId)
  const qid = Number(quizId)
  const space = useSpaceById(sid)
  const quizNameQ = useQuery({ queryKey: ['portal-space-home', sid], queryFn: () => api.portalSpaceHome(sid) })
  const quizName = quizNameQ.data?.quizzes.find((q) => q.id === qid)?.title ?? `刷题 #${qid}`

  if (space === 'loading') return <Center text="加载中…" />
  if (space === null) return <Center text="空间不存在或无权访问" />

  return (
    <SpacePageShell spaceId={sid} backTo={`/s/${sid}/quiz`} backLabel="返回刷题列表">
      <QuizRound key={qid} qid={qid} quizName={quizName} spaceName={space.name} />
    </SpacePageShell>
  )
}

function Center({ text }: { text: string }) {
  return <div className="flex h-dvh items-center justify-center text-sm text-muted-foreground">{text}</div>
}

function QuizRound({ qid, quizName, spaceName }: { qid: number; quizName: string; spaceName: string }) {
  const [problem, setProblem] = useState<QuizProblemResponse['problem'] | null>(null)
  const [fetchError, setFetchError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const [done, setDone] = useState(false)
  const [busy, setBusy] = useState(false)
  const [selected, setSelected] = useState<ObjectiveAnswer | null>(null)
  const [feedback, setFeedback] = useState<{ correct: boolean; correctAnswer?: CorrectAnswer; firstTime?: boolean } | null>(null)

  async function fetchProblem() {
    setLoading(true)
    setFetchError(null)
    try {
      const r = await api.portalQuizProblem(qid)
      if (r.done || !r.problem) {
        setDone(true)
        setProblem(null)
      } else {
        setDone(false)
        setProblem(r.problem)
        setSelected(null)
        setFeedback(null)
      }
    } catch (err) {
      setFetchError(err instanceof Error ? err.message : '抽题失败')
    } finally {
      setLoading(false)
    }
  }

  // 首次挂载自动抽题
  useEffect(() => {
    void fetchProblem()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [qid])

  async function answer(a: ObjectiveAnswer) {
    if (!problem || busy || feedback) return
    setSelected(a)
    setBusy(true)
    try {
      const r = await api.portalQuizAnswer(qid, problem.id, a)
      setFeedback(r)
      if (r.correct) {
        toast.success(r.firstTime ? '回答正确 · 首次通过 +1' : '回答正确（此前已通过）')
      } else {
        toast.error('回答错误')
      }
    } catch (err) {
      toast.error(err instanceof Error ? err.message : '提交失败')
      setSelected(null)
    } finally {
      setBusy(false)
    }
  }

  function retry() {
    setFeedback(null)
    setSelected(null)
  }

  return (
    <PageContainer className="max-w-2xl lg:px-6">
      <div className="mt-3 rounded-2xl border bg-card p-5">
        {done ? (
          <DonePanel spaceName={spaceName} quizName={quizName} onRefresh={() => void fetchProblem()} />
        ) : fetchError ? (
          <div className="py-10 text-center">
            <p className="text-sm text-muted-foreground">{fetchError}</p>
            <Button variant="outline" className="mt-3" onClick={() => void fetchProblem()}>
              <RefreshCwIcon className="size-4" /> 重试
            </Button>
          </div>
        ) : loading || !problem ? (
          <div className="flex items-center justify-center gap-2 py-12 text-sm text-muted-foreground">
            <Loader2Icon className="size-4 animate-spin" /> 正在抽题…
          </div>
        ) : (
          <div>
            <div className="mb-3 flex items-center gap-2">
              <BookOpenIcon className="size-4 shrink-0 text-primary" />
              <span className="min-w-0 flex-1 truncate text-sm font-semibold">{quizName}</span>
            </div>
            <ObjectiveQuestion
              problem={problem}
              selected={selected}
              feedback={feedback}
              busy={busy}
              onSelect={(a) => void answer(a)}
            />
            {feedback && (
              <div className="mt-5 flex flex-wrap justify-end gap-2">
                {!feedback.correct && (
                  <Button variant="outline" onClick={retry}>
                    <RefreshCwIcon className="size-4" /> 再试一次
                  </Button>
                )}
                <Button onClick={() => void fetchProblem()}>
                  {feedback.correct ? '下一题' : '换一题'}
                  <SkipForwardIcon className="size-4" />
                </Button>
              </div>
            )}
          </div>
        )}
      </div>
    </PageContainer>
  )
}

function DonePanel({ spaceName, quizName, onRefresh }: { spaceName: string; quizName: string; onRefresh: () => void }) {
  return (
    <div className="py-10 text-center">
      <div className="mx-auto mb-3 flex size-14 items-center justify-center rounded-full bg-emerald-50">
        <PartyPopperIcon className="size-7 text-emerald-600" />
      </div>
      <h2 className="flex items-center justify-center gap-2 text-lg font-semibold">
        <CheckCircle2Icon className="size-5 text-emerald-600" /> 本组刷题完成！
      </h2>
      <p className="mt-2 text-sm text-muted-foreground">
        空间「{spaceName}」的「{quizName}」内题目已全部通过（按 uuid 去重记录）
      </p>
      <div className={cn('mt-5 flex justify-center gap-3')}>
        <Button variant="outline" onClick={onRefresh}>
          <RefreshCwIcon className="size-4" /> 刷新检查
        </Button>
      </div>
    </div>
  )
}
