import { useEffect, useState } from 'react'
import { useParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { BookOpenIcon, CheckCircle2Icon, LayersIcon, Loader2Icon, PartyPopperIcon, RefreshCwIcon, RotateCcwIcon, SkipForwardIcon } from 'lucide-react'
import { toast } from 'sonner'

import { api } from '@/api'
import type { CorrectAnswer, ObjectiveAnswer, QuizProblemResponse } from '@/api/types'
import { useSpaceById } from './portal-context'
import { PageContainer, SpacePageShell } from './SpacePageShell'
import { ObjectiveQuestion } from '@/components/portal/objective'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

// 刷题（批式复习）单题流：
// 默认规则——同批不重复；做过（已通过）的题少抽；答错的题进错题袋，下一批优先复抽；
// 一轮（批）覆盖完且无错题 → 本轮完成，可重新开始；有错题 → 自动开新一轮继续复习。
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
      <QuizRound key={qid} qid={qid} quizName={quizName} />
    </SpacePageShell>
  )
}

function Center({ text }: { text: string }) {
  return <div className="flex h-dvh items-center justify-center text-sm text-muted-foreground">{text}</div>
}

function QuizRound({ qid, quizName }: { qid: number; quizName: string }) {
  const [problem, setProblem] = useState<QuizProblemResponse['problem'] | null>(null)
  const [fetchError, setFetchError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const [done, setDone] = useState(false)
  const [newBatch, setNewBatch] = useState(false) // 本轮开始时提示
  const [batchNo, setBatchNo] = useState(1)
  const [wrongCnt, setWrongCnt] = useState(0)
  const [busy, setBusy] = useState(false)
  const [selected, setSelected] = useState<ObjectiveAnswer | null>(null)
  const [feedback, setFeedback] = useState<{ correct: boolean; correctAnswer?: CorrectAnswer; firstTime?: boolean } | null>(null)

  async function fetchProblem(reset?: boolean) {
    setLoading(true)
    setFetchError(null)
    setDone(false)
    setNewBatch(false)
    try {
      if (reset) await api.portalQuizReset(qid)
      const r = await api.portalQuizProblem(qid)
      if (r.done || !r.problem) {
        setDone(true)
        setProblem(null)
      } else {
        setDone(false)
        setProblem(r.problem)
        setNewBatch(!!r.newBatch)
        setBatchNo(r.batchNo ?? 1)
        setWrongCnt(r.wrongCnt ?? 0)
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
      if (typeof r.wrongCnt === 'number') setWrongCnt(r.wrongCnt)
      if (r.correct) {
        toast.success(r.firstTime ? '回答正确 · 首次通过' : '回答正确')
      } else {
        toast.error('回答错误，已加入错题袋（下一轮优先复习）')
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
        {/* 批次/进度信息条 */}
        {!done && !fetchError && (
          <div className="mb-3 flex flex-wrap items-center gap-x-3 gap-y-1 text-[11px] text-muted-foreground">
            <span className="inline-flex items-center gap-1 font-medium text-primary">
              <LayersIcon className="size-3.5" /> 第 {batchNo} 轮
            </span>
            {wrongCnt > 0 && (
              <span className="inline-flex items-center gap-1 text-red-500">错题袋 {wrongCnt} 道（下一轮优先）</span>
            )}
            <span className="ml-auto truncate">范围内单选/判断循环 · 同轮不重复</span>
          </div>
        )}

        {done ? (
          <DonePanel wrongCnt={wrongCnt} quizName={quizName} onRestart={() => void fetchProblem(true)} onRefresh={() => void fetchProblem(false)} />
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
            {newBatch && (
              <div className="mb-3 rounded-lg border border-sky-200 bg-sky-50 px-3 py-2 text-xs text-sky-700">
                进入第 {batchNo} 轮：错题优先复习，同轮题目不重复
              </div>
            )}
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

function DonePanel({ wrongCnt, quizName, onRestart, onRefresh }: {
  wrongCnt: number
  quizName: string
  onRestart: () => void
  onRefresh: () => void
}) {
  const allClear = wrongCnt === 0
  return (
    <div className="py-10 text-center">
      <div className={cn('mx-auto mb-3 flex size-14 items-center justify-center rounded-full', allClear ? 'bg-emerald-50' : 'bg-sky-50')}>
        {allClear ? <PartyPopperIcon className="size-7 text-emerald-600" /> : <CheckCircle2Icon className="size-7 text-sky-600" />}
      </div>
      <h2 className="flex items-center justify-center gap-2 text-lg font-semibold">
        {allClear ? '本轮刷题完成！' : `本轮完成，错题袋 ${wrongCnt} 道`}
      </h2>
      <p className="mt-2 text-sm text-muted-foreground">
        {allClear
          ? <>「{quizName}」范围内的题已全部答对一轮（已通过记录保留），可重新开始一轮</>
          : <>「{quizName}」本轮题目已全部出现，{wrongCnt} 道错题将在下一轮优先复习</>}
      </p>
      <div className="mt-5 flex justify-center gap-3">
        {!allClear && (
          <Button variant="outline" onClick={onRefresh}>
            <RefreshCwIcon className="size-4" /> 继续下一轮（复习错题）
          </Button>
        )}
        {allClear && (
          <Button onClick={onRestart}>
            <RotateCcwIcon className="size-4" /> 重新开始一轮
          </Button>
        )}
      </div>
    </div>
  )
}
