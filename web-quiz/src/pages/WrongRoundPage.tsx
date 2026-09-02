import { useCallback, useEffect, useRef, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { ArrowLeftIcon, RefreshCwIcon, TrophyIcon } from 'lucide-react'
import { toast } from 'sonner'

import { api } from '@/lib/api'
import type { WrongRound } from '@/lib/types'
import { Button } from '@/components/ui/button'
import { QuestionCard } from '@/components/QuestionCard'

// /wrong/:scope — 错题练习（scope = all 或分类 id）。答对自动从错题集移除；
// 全部答完后返回错题列表（列表重新拉取，已移除的题不再出现）。
export function WrongRoundPage() {
  const { scope } = useParams()
  const navigate = useNavigate()
  const [round, setRound] = useState<WrongRound | null>(null)
  const [idx, setIdx] = useState(0)
  const [correct, setCorrect] = useState(0)
  const [wrong, setWrong] = useState(0)
  const [loadError, setLoadError] = useState<string | null>(null)
  const startedRef = useRef(false)

  const startRound = useCallback(async () => {
    if (!scope) return
    setLoadError(null)
    try {
      const categoryId = scope === 'all' ? undefined : Number(scope)
      const r = await api.startWrongRound(categoryId)
      if (r.problems.length === 0) {
        setLoadError('错题集是空的')
        return
      }
      setRound(r)
      setIdx(0)
      setCorrect(0)
      setWrong(0)
    } catch (err) {
      setLoadError(err instanceof Error ? err.message : '加载错题失败')
    }
  }, [scope])

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
            <Link to="/wrong">
              <Button variant="ghost" className="text-muted-foreground">
                <ArrowLeftIcon className="size-4" /> 错题列表
              </Button>
            </Link>
          </div>
        </div>
      </div>
    )
  }

  if (round === null) {
    return <div className="flex h-full items-center justify-center text-sm text-muted-foreground">正在加载错题…</div>
  }

  const problem = round.problems[idx]
  const last = idx + 1 >= round.problems.length
  return (
    <div>
      <div className="mx-auto flex w-full max-w-2xl px-4 pt-4">
        <Link to="/wrong">
          <Button variant="ghost" size="sm" className="-ml-2 text-muted-foreground">
            <ArrowLeftIcon className="size-4" /> 错题列表
          </Button>
        </Link>
      </div>
      <QuestionCard
        key={problem.id}
        problem={problem}
        index={idx}
        total={round.problems.length}
        submit={(payload) => api.submit({ ...payload, problemId: problem.id, categoryId: problem.categoryId })}
        onNext={() => {
          if (!last) {
            setIdx(idx + 1)
          } else {
            navigate('/wrong')
          }
        }}
        onCorrect={() => {
          setCorrect((n) => n + 1)
          toast('已从错题集移除', { description: '答对啦，这道题已自动移出错题集' })
        }}
        onWrong={() => setWrong((n) => n + 1)}
      />
      {correct === round.problems.length && wrong === 0 && (
        <div className="mx-auto w-full max-w-2xl px-4 pb-6">
          <div className="flex items-center gap-2 rounded-xl bg-emerald-50 p-3 text-sm text-emerald-700">
            <TrophyIcon className="size-4" />
            本轮全部答对，错题已全部移出错题集
          </div>
        </div>
      )}
    </div>
  )
}