// 可拖拽左右分栏（桌面 lg+）：左题面区 flex-1 + 拖拽分隔条 + 右编辑器区（宽度=rightPct%）。
// 移动端（<lg）自动纵向堆叠：题面在上、编辑器在下。
// 两栏内容自行负责滚动；外层高度由父级 flex 约束。
import { useCallback, useEffect, useRef, useState } from 'react'
import { cn } from '@/lib/utils'

export function SplitPane({
  left,
  right,
  initialRightPct = 50,
  minRightPct = 28,
  maxRightPct = 72,
}: {
  left: React.ReactNode
  right: React.ReactNode
  initialRightPct?: number
  minRightPct?: number
  maxRightPct?: number
}) {
  const rootRef = useRef<HTMLDivElement>(null)
  const [rightPct, setRightPct] = useState(initialRightPct)
  const dragging = useRef(false)
  const [active, setActive] = useState(false)
  // 单实例：按断点只挂载一份 right（桌面拖拽分栏 / 移动端整宽堆叠），
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

  useEffect(() => {
    const onMove = (e: MouseEvent) => {
      if (!dragging.current || !rootRef.current) return
      const rect = rootRef.current.getBoundingClientRect()
      if (rect.width <= 0) return
      const rw = rect.right - e.clientX
      setRightPct(clamp((rw / rect.width) * 100))
      e.preventDefault()
    }
    const onUp = () => {
      dragging.current = false
      setActive(false)
      document.body.style.cursor = ''
      document.body.style.userSelect = ''
    }
    document.addEventListener('mousemove', onMove)
    document.addEventListener('mouseup', onUp)
    return () => {
      document.removeEventListener('mousemove', onMove)
      document.removeEventListener('mouseup', onUp)
    }
  }, [clamp])

  return (
    <div ref={rootRef} className="flex h-full min-h-0 min-w-0 flex-col overflow-hidden lg:flex-row">
      {/* 左：题面（占剩余宽度；自身滚动） */}
      <div className="min-h-0 w-full flex-1 overflow-y-auto lg:border-r">{left}</div>

      {/* 拖拽分隔条（仅桌面） */}
      <div
        role="separator"
        aria-orientation="vertical"
        title="拖动调整左右宽度（双击复位）"
        onMouseDown={(e) => {
          dragging.current = true
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

      {/* 右：编辑器（桌面=宽度百分比分栏；移动端=整宽堆叠）——仅按断点渲染一份 */}
      {isDesktop ? (
        <div className="flex min-h-0 flex-col lg:h-full lg:shrink-0" style={{ width: `${rightPct}%` }}>
          {right}
        </div>
      ) : (
        <div className="flex min-h-0 w-full flex-col">{right}</div>
      )}
    </div>
  )
}
