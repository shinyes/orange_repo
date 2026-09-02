import { useCallback, useEffect, useRef, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { ArrowLeftIcon, BookOpenIcon, RefreshCwIcon, TrophyIcon } from 'lucide-react'

import { api } from '@/lib/api'
import type { Round } from '@/lib/types'
import { Button } from '@/components/ui/button'
import { QuestionCard } from '@/components/QuestionCard'

// /quiz/:subjectId/:categoryId — 做题页：随机抽题逐题作答；全部答完展示本轮统计。
// 每轮数据在内存（刷新/离开即重新抽题），URL 保持分类上下文，返回键回到分类列表。
export function QuizRoundPage() {
  const { subjectId, categoryId } = useParams()
  const [round, setRound] = useState<Round | null>(null)
  const [idx, setIdx] = useState(0)
  const [correct, setCorrect] = useState(0)
  const [wrong, setWrong] = useState(0)
  const [done, setDone] = useState(false)
  const [loadError, setLoadError] = useState<string | null>(null)
  const startedRef = useRef(false)

  const startRound = useCallback(async () => {
    if (!categoryId) return
    setLoadError(null)
    try {
      const r = await api.startRound(Number(categoryId))
      if (r.problems.length === 0) {
        setLoadError('该分类暂无符合条件的题目')
        return
      }
      setRound(r)
      setIdx(0)
      setCorrect(0)
      setWrong(0)
      setDone(false)
    } catch (err) {
      setLoadError(err instanceof Error ? err.message : '抽题失败')
    }
  }, [categoryId])

  // 首次挂载自动抽题（进入路由即开始，返回再进入时重新抽）
  useEffect(() => {
    if (!startedRef.current) {
      startedRef.current = true
      void startRound()
    }
  }, [startRound])

  if (loadError) {
    return (
      <div className="mx-auto w-full max-w-2xl px-4 py-10 text-center">
        <div className="rounded-2xl border bg-card p-8">
          <p className="text-sm text-muted-foreground">{loadError}</p>
          <div className="mt-4 flex justify-center gap-3">
            <Button variant="outline" onClick={() => void startRound()}>
              <RefreshCwIcon className="size-4" /> 重试
            </Button>
            <Link to={`/quiz/${subjectId}`}>
              <Button variant="ghost" className="text-muted-foreground">
                <ArrowLeftIcon className="size-4" /> 返回分类
              </Button>
            </Link>
          </div>
        </div>
      </div>
    )
  }

  if (round === null) {
    return <div className="flex h-full items-center justify-center text-sm text-muted-foreground">正在抽题…</div>
  }

  if (done) {
    return (
      <div className="mx-auto w-full max-w-2xl px-4 py-10">
        <div className="rounded-2xl border bg-card p-8 text-center">
          <TrophyIcon className="mx-auto mb-3 size-10 text-primary" />
          <h2 className="text-xl font-semibold">本轮完成！</h2>
          <div className="mt-4 flex justify-center gap-6 text-sm">
            <div className="rounded-xl bg-emerald-50 px-6 py-3">
              <div className="text-2xl font-bold text-emerald-600">{correct}</div>
              <div className="text-xs text-muted-foreground">答对</div>
            </div>
            <div className="rounded-xl bg-red-50 px-6 py-3">
              <div className="text-2xl font-bold text-red-600">{wrong}</div>
              <div className="text-xs text-muted-foreground">答错</div>
            </div>
          </div>
          <div className="mt-6 flex justify-center gap-3">
            <Button onClick={() => void startRound()}>
              <RefreshCwIcon className="size-4" /> 再练一轮
            </Button>
            <Link to={`/quiz/${subjectId}`}>
              <Button variant="outline">
                <BookOpenIcon className="size-4" /> 返回分类
              </Button>
            </Link>
          </div>
        </div>
      </div>
    )
  }

  const problem = round.problems[idx]
  const last = idx + 1 >= round.problems.length
  return (
    <div>
      <div className="mx-auto flex w-full max-w-2xl px-4 pt-4">
        <Link to={`/quiz/${subjectId}`}>
          <Button variant="ghost" size="sm" className="-ml-2 text-muted-foreground">
            <ArrowLeftIcon className="size-4" /> 返回分类
          </Button>
        </Link>
      </div>
      <QuestionCard
        key={problem.id}
        problem={problem}
        index={idx}
        total={round.problems.length}
        submit={(payload) => api.submit({ ...payload, problemId: problem.id, categoryId: round.categoryId })}
        onNext={() => {
          if (!last) {
            setIdx(idx + 1)
          } else {
            setDone(true)
          }
        }}
        onCorrect={() => setCorrect((n) => n + 1)}
        onWrong={() => setWrong((n) => n + 1)}
      />
    </div>
  )
}