// 可拖拽分栏：
//  桌面 lg+：左题面 + 竖直拖拽条 + 右编辑器（宽度=rightPct%）
//  移动 <lg：题面在上（topPct%）、中间水平拖拽条、编辑器在下——同样可调占比
// 两栏内容各自负责滚动；外层高度由父级 flex 约束。
import { useCallback, useEffect, useRef, useState } from 'react'
import { cn } from '@/lib/utils'

const MOBILE_TOP_MIN = 25
const MOBILE_TOP_MAX = 75

export function SplitPane({
  left,
  right,
  initialRightPct = 50,
  minRightPct = 28,
  maxRightPct = 72,
  initialTopPct = 45,
}: {
  left: React.ReactNode
  right: React.ReactNode
  initialRightPct?: number
  minRightPct?: number
  maxRightPct?: number
  /** 移动端题面初始占比 % */
  initialTopPct?: number
}) {
  const rootRef = useRef<HTMLDivElement>(null)
  const [rightPct, setRightPct] = useState(initialRightPct)
  const [topPct, setTopPct] = useState(initialTopPct)
  const dragging = useRef<{ startPct: number; latestX: number } | null>(null)
  const topDrag = useRef<{ startY: number; startPct: number; latestY: number } | null>(null)
  const [active, setActive] = useState(false)
  // 单实例：按断点只挂载一份 right（桌面分栏 / 移动端上下分栏），
  // 避免重型编辑器（Monaco）被同时实例化两份
  const [isDesktop, setIsDesktop] = useState(
    () => typeof window !== 'undefined' && window.matchMedia('(min-width: 1024px)').matches,
  )

  useEffect(() => {
    const mq = window.matchMedia('(min-width: 1024px)')
    const onChange = () => setIsDesktop(mq.matches)
    mq.addEventListener('change', onChange)
    return () => mq.removeEventListener('change', onChange)
  }, [])

  const clamp = useCallback(
    (pct: number) => Math.min(maxRightPct, Math.max(minRightPct, pct)),
    [minRightPct, maxRightPct],
  )
  const clampTop = (pct: number) => Math.min(MOBILE_TOP_MAX, Math.max(MOBILE_TOP_MIN, pct))

  // rAF 节流：拖动期间每个动画帧最多更新一次（避免 pointermove 高频 setState
  // 引发整树高频重渲染 → 在途请求被反复取消/重建，控制台出现 Canceled）
  const frame = useRef<number | null>(null)
  const scheduleUpdate = useCallback((fn: () => void) => {
    if (frame.current != null) return
    frame.current = requestAnimationFrame(() => {
      frame.current = null
      fn()
    })
  }, [])

  // 桌面左右拖拽（鼠标，rAF 节流）
  useEffect(() => {
    const onMove = (e: MouseEvent) => {
      if (!dragging.current || !rootRef.current) return
      dragging.current.latestX = e.clientX
      scheduleUpdate(() => {
        const d = dragging.current
        if (!d || !rootRef.current) return
        const rect = rootRef.current.getBoundingClientRect()
        if (rect.width <= 0) return
        const rw = rect.right - d.latestX
        setRightPct(clamp((rw / rect.width) * 100))
      })
      e.preventDefault()
    }
    const onUp = () => {
      dragging.current = null
      if (frame.current != null) cancelAnimationFrame(frame.current)
      frame.current = null
      setActive(false)
      document.body.style.cursor = ''
      document.body.style.userSelect = ''
    }
    document.addEventListener('mousemove', onMove)
    document.addEventListener('mouseup', onUp)
    return () => {
      if (frame.current != null) cancelAnimationFrame(frame.current)
      frame.current = null
      document.removeEventListener('mousemove', onMove)
      document.removeEventListener('mouseup', onUp)
    }
  }, [clamp, scheduleUpdate])

  // 移动端上下拖拽（pointer，同时覆盖触摸；rAF 节流）
  function startMobileDrag(e: React.PointerEvent) {
    if (e.button !== 0) return
    e.preventDefault()
    topDrag.current = { startY: e.clientY, startPct: topPct, latestY: e.clientY }
    document.body.style.userSelect = 'none'
    document.body.style.cursor = 'ns-resize'
    const onMove = (ev: PointerEvent) => {
      const d = topDrag.current
      if (!d || !rootRef.current) return
      // 记录最新指针位置，rAF 回调按最新值计算（节流但不丢增量）
      d.latestY = ev.clientY
      scheduleUpdate(() => {
        const dd = topDrag.current
        if (!dd || !rootRef.current) return
        const r = rootRef.current.getBoundingClientRect()
        if (r.height <= 0) return
        setTopPct(clampTop(dd.startPct + ((dd.latestY - dd.startY) / r.height) * 100))
      })
      ev.preventDefault()
    }
    const onUp = () => {
      topDrag.current = null
      if (frame.current != null) cancelAnimationFrame(frame.current)
      frame.current = null
      document.body.style.userSelect = ''
      document.body.style.cursor = ''
      window.removeEventListener('pointermove', onMove)
      window.removeEventListener('pointerup', onUp)
    }
    window.addEventListener('pointermove', onMove)
    window.addEventListener('pointerup', onUp)
  }

  return (
    <div ref={rootRef} className="flex h-full min-h-0 min-w-0 flex-col overflow-hidden lg:flex-row">
      {/* 上/左：题面（桌面占剩余宽度；移动端占 topPct%）——各自内部滚动 */}
      <div
        className="min-h-0 w-full overflow-y-auto lg:flex-1 lg:border-r"
        style={!isDesktop ? { height: `${topPct}%` } : undefined}
      >
        {left}
      </div>

      {/* 竖直拖拽条（仅桌面） */}
      <div
        role="separator"
        aria-orientation="vertical"
        title="拖动调整左右宽度（双击复位）"
        onMouseDown={(e) => {
          dragging.current = { startPct: rightPct, latestX: e.clientX }
          setActive(true)
          document.body.style.cursor = 'col-resize'
          document.body.style.userSelect = 'none'
          e.preventDefault()
        }}
        onDoubleClick={() => setRightPct(clamp(initialRightPct))}
        className={cn(
          'hidden w-1.5 shrink-0 cursor-col-resize touch-none select-none items-center justify-center bg-transparent transition-colors hover:bg-primary/25 lg:flex',
          active && 'bg-primary/40',
        )}
      >
        <span className="pointer-events-none h-14 w-0.5 rounded-full bg-muted-foreground/40" />
      </div>

      {/* 水平拖拽条（仅移动端：调整题面/编辑器上下占比） */}
      {!isDesktop && (
        <div
          role="separator"
          aria-orientation="horizontal"
          title="拖动调整上下占比（双击复位）"
          onPointerDown={startMobileDrag}
          onDoubleClick={() => setTopPct(clampTop(initialTopPct))}
          className="group flex h-3 shrink-0 cursor-ns-resize touch-none items-center justify-center border-y bg-muted/40"
        >
          <span className="pointer-events-none h-1 w-12 rounded-full bg-muted-foreground/25 transition-colors group-hover:bg-primary/50" />
        </div>
      )}

      {/* 下/右：编辑器（桌面=宽度百分比；移动端=剩余高度）——仅按断点渲染一份 */}
      {isDesktop ? (
        <div className="flex min-h-0 flex-col lg:h-full lg:shrink-0" style={{ width: `${rightPct}%` }}>
          {right}
        </div>
      ) : (
        <div className="min-h-0 w-full flex-1 overflow-y-auto">{right}</div>
      )}
    </div>
  )
}
