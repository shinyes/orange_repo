// 休息时间：小游戏 + 榜单（本域榜单 / 全域榜单）。
// 设计：游戏区与榜单并排，玩的时候榜单常驻可见；窄屏时榜单折叠到游戏下方。
// 样式刻意贴近小恐龙的像素风（直角、2px 描边、等宽字体），与站内卡片风格区分开。
import { Suspense, useCallback, useMemo, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useOutletContext } from 'react-router-dom'
import { toast } from 'sonner'
import { ArrowLeftIcon, GamepadIcon, Loader2Icon, RotateCcwIcon, TrophyIcon } from 'lucide-react'

import { api } from '@/api'
import type { GameRankRow } from '@/api/types'
import { savedSpaceId } from '@/api/space'
import { GAMES, gameById, type GameProps } from '@/pages/break/games/registry'

type ShellContext = { user: { id: number; username: string; role: string } }

export function BreakPage() {
  const { user } = useOutletContext<ShellContext>()
  const [gameId, setGameId] = useState(GAMES[0].id)
  const game = useMemo(() => gameById(gameId), [gameId])
  const [scope, setScope] = useState<'domain' | 'all'>('domain')
  const [gameKey, setGameKey] = useState(0)
  const [lastResult, setLastResult] = useState<{ score: number; isNewBest: boolean; rankDomain: number; rankAll: number } | null>(null)
  const qc = useQueryClient()
  const spaceId = savedSpaceId() ?? undefined

  const rankQ = useQuery({
    queryKey: ['game-rank', game.id, scope, spaceId ?? 0],
    queryFn: () => api.gameRank(game.id, scope, spaceId),
    refetchInterval: 20000, // 玩的时候榜单自动刷新，能看到别人刚刷的分
  })

  const submit = useCallback(async (score: number) => {
    // 0 分不提交（刚开局就撞：没有记录价值）
    if (score <= 0) return
    if (score > game.maxScore) {
      toast.error(`分数异常（上限 ${game.maxScore}），本局不计入榜单`)
      return
    }
    try {
      const res = await api.gameScoreSubmit(game.id, score, spaceId)
      setLastResult(res)
      if (res.isNewBest) toast.success(`新纪录 ${score} ${game.scoreUnit}！`)
      else toast(`本局 ${score} ${game.scoreUnit}（最高 ${res.bestScore}）`)
      void qc.invalidateQueries({ queryKey: ['game-rank'] })
    } catch (e) {
      toast.error(e instanceof Error ? e.message : '成绩提交失败')
    }
  }, [game.id, game.maxScore, game.scoreUnit, spaceId, qc])

  const rows = rankQ.data?.rows ?? []
  const myRank = rankQ.data?.myRank
  const myBest = rankQ.data?.myBest ?? 0

  return (
    <div className="mx-auto w-full max-w-5xl px-4 py-6 lg:px-8">
      {/* 顶部：标题 + 返回（像素风标题条） */}
      <div className="mb-4 flex items-center gap-3">
        <Link to="/mine" className="pixel-mono inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground">
          <ArrowLeftIcon className="size-3.5" /> 我的
        </Link>
        <h1 className="pixel-mono flex items-center gap-2 text-base font-semibold tracking-wide">
          <GamepadIcon className="size-4" /> 休息时间
        </h1>
        <span className="pixel-mono text-xs text-muted-foreground">放松一下，成绩计入排行榜</span>
      </div>

      {/* 游戏选择（目前只有小恐龙；注册表新增游戏后这里自动出现） */}
      {GAMES.length > 1 && (
        <div className="mb-3 flex flex-wrap gap-1.5">
          {GAMES.map((g) => (
            <button
              key={g.id}
              type="button"
              onClick={() => { setGameId(g.id); setGameKey((k) => k + 1); setLastResult(null) }}
              className={`pixel-mono border-2 px-2.5 py-1 text-xs ${g.id === game.id ? 'border-[#2b2b2b] bg-[#2b2b2b] text-white' : 'border-[#2b2b2b]/40 hover:border-[#2b2b2b]'}`}
            >
              {g.title}
            </button>
          ))}
        </div>
      )}

      <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_320px]">
        {/* 游戏区 */}
        <section className="pixel-frame bg-[#f7f7f7] p-3">
          <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
            <div className="pixel-mono text-xs text-[#2b2b2b]">
              {game.title} · 我的最高 {myBest} {game.scoreUnit}
              {typeof myRank === 'number' && myRank > 0 ? ` · 排名 #${myRank}` : ''}
              {/* 上一局结果紧接着「我的最高」显示（原来在游戏区下方单独一行） */}
              {lastResult && (
                <>
                  {' · '}上一局 {lastResult.score} {game.scoreUnit}
                  {lastResult.isNewBest ? '（新纪录！）' : ''}
                  {lastResult.rankDomain > 0 ? ` · 本域 #${lastResult.rankDomain}` : ''}
                  {lastResult.rankAll > 0 ? ` · 全域 #${lastResult.rankAll}` : ''}
                </>
              )}
            </div>
            <button
              type="button"
              className="pixel-mono inline-flex items-center gap-1 border-2 border-[#2b2b2b] px-2 py-0.5 text-xs hover:bg-[#2b2b2b] hover:text-white"
              onClick={() => { setGameKey((k) => k + 1); setLastResult(null) }}
            >
              <RotateCcwIcon className="size-3" /> 重开
            </button>
          </div>

          <Suspense fallback={<div className="flex h-[150px] items-center justify-center text-xs text-muted-foreground"><Loader2Icon className="mr-2 size-4 animate-spin" /> 游戏加载中…</div>}>
            <GameHost game={game} gameKey={gameKey} onGameOver={submit} />
          </Suspense>

          <div className="pixel-mono mt-2 space-y-0.5 text-[11px] text-[#2b2b2b]/80">
            {game.controls.map((c) => <p key={c}>· {c}</p>)}
            <p>· 一局结束后自动上传成绩（只保留最高分）</p>
          </div>
        </section>

        {/* 榜单（常驻，玩的时候也能看到） */}
        <section className="pixel-frame flex flex-col bg-white">
          <div className="pixel-scanlines flex items-center gap-2 border-b-2 border-[#2b2b2b] px-3 py-2">
            <TrophyIcon className="size-3.5 text-[#2b2b2b]" />
            <span className="pixel-mono text-xs font-semibold">排行榜</span>
            <div className="ml-auto flex gap-1">
              {(['domain', 'all'] as const).map((s) => (
                <button
                  key={s}
                  type="button"
                  onClick={() => setScope(s)}
                  className={`pixel-mono border-2 px-1.5 py-0.5 text-[11px] ${scope === s ? 'border-[#2b2b2b] bg-[#2b2b2b] text-white' : 'border-[#2b2b2b]/40 hover:border-[#2b2b2b]'}`}
                >
                  {s === 'domain' ? '本域榜单' : '全域榜单'}
                </button>
              ))}
            </div>
          </div>

          <div className="pixel-mono border-b border-dashed border-[#2b2b2b]/30 px-3 py-1 text-[10px] text-[#2b2b2b]/70">
            {scope === 'domain'
              ? (rankQ.data?.noDomain
                ? '你还没有加入任何空间，暂时看不到本域榜单'
                : rankQ.data?.domainName ? `范围：${rankQ.data.domainName}` : '范围：当前空间所属域')
              : '范围：全部域'}
          </div>

          <div className="max-h-[360px] min-h-[180px] flex-1 overflow-y-auto">
            {rankQ.isLoading ? (
              <div className="flex h-[180px] items-center justify-center text-xs text-muted-foreground">
                <Loader2Icon className="mr-2 size-3.5 animate-spin" /> 加载中…
              </div>
            ) : rows.length === 0 ? (
              <div className="pixel-mono flex h-[180px] flex-col items-center justify-center gap-1 px-3 text-center text-[11px] text-[#2b2b2b]/60">
                <span>{rankQ.data?.noDomain ? '你还没有加入任何空间' : '还没有成绩'}</span>
                <span>{rankQ.data?.noDomain ? '可切换到全域榜单查看' : '玩一局就能上榜'}</span>
              </div>
            ) : (
              <ol>
                {rows.map((r) => <RankItem key={`${r.userId}`} row={r} me={r.userId === user.id} unit={game.scoreUnit} />)}
              </ol>
            )}
          </div>

          {/* 我的排名（不在榜内也常驻显示） */}
          <div className="pixel-mono border-t-2 border-[#2b2b2b] px-3 py-2 text-[11px]">
            <div className="flex items-center justify-between">
              <span>我：{user.username}</span>
              <span>{myBest} {game.scoreUnit}{typeof myRank === 'number' && myRank > 0 ? ` · #${myRank}` : ''}</span>
            </div>
            {rankQ.data?.myPlays ? <div className="mt-0.5 text-[10px] text-[#2b2b2b]/60">已玩 {rankQ.data.myPlays} 局</div> : null}
          </div>
        </section>
      </div>
    </div>
  )
}

/** 按注册表懒加载具体游戏组件（新增游戏无需改本页）。 */
function GameHost({ game, gameKey, onGameOver }: { game: ReturnType<typeof gameById>; gameKey: number; onGameOver: GameProps['onGameOver'] }) {
  const Game = game.component
  return <Game gameKey={gameKey} onGameOver={onGameOver} />
}

/** 榜单行：像素风 + 自己高亮。 */
function RankItem({ row, me, unit }: { row: GameRankRow; me: boolean; unit: string }) {
  const medal = row.rank === 1 ? '#d4a017' : row.rank === 2 ? '#9aa0a6' : row.rank === 3 ? '#b06a2c' : null
  return (
    <li className={`flex items-center gap-2 border-b border-dashed border-[#2b2b2b]/20 px-3 py-1.5 text-xs ${me ? 'bg-[#fff3bf]' : ''}`}>
      <span className="pixel-mono w-7 shrink-0 tabular-nums text-[#2b2b2b]/70">
        {medal ? <span style={{ color: medal }}>#{row.rank}</span> : `#${row.rank}`}
      </span>
      <span className="min-w-0 flex-1 truncate">
        {row.userName || `用户 #${row.userId}`}
        {me && <span className="ml-1 text-[10px] text-[#2b2b2b]/60">（我）</span>}
      </span>
      <span className="pixel-mono shrink-0 tabular-nums">{row.bestScore} {unit}</span>
    </li>
  )
}
