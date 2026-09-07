// 可拖拽左右分栏（桌面 lg+）：左题面区 + 拖拽分隔条 + 右编辑器区（可拖动调宽）。
// 移动端（<lg）自动纵向堆叠：题面在上、编辑器在下。
// 两栏内容自行负责滚动（外层高度由父级 flex 约束）。
import { useCallback, useEffect, useRef, useState } from 'react'
import { cn } from '@/lib/utils'

export function SplitPane({
  left,
  right,
  initialRightPct = 42,
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

  const clamp = useCallback(
    (pct: number) => Math.min(maxRightPct, Math.max(minRightPct, pct)),
    [minRightPct, maxRightPct],
  )

  useEffect(() => {
    const onMove = (e: MouseEvent) => {
      if (!dragging.current || !rootRef.current) return
      const rect = rootRef.current.getBoundingClientRect()
      if (rect.width <= 0) return
      // 右边缘到光标距离 = 右侧宽度
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
    <div ref={rootRef} className="flex h-full min-h-0 min-w-0 flex-col lg:flex-row">
      {/* 左：题面（占剩余宽度；自身内部滚动） */}
      <div className="min-h-0 flex-1 overflow-y-auto lg:border-r">{left}</div>

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

      {/* 右：编辑器（桌面宽度=百分比；移动端纵向在下） */}
      <div
        className={cn('flex min-h-0 flex-col lg:shrink-0 lg:h-full', active && 'lg:pointer-events-none')}
        style={{ width: '100%', ['--split-right' as string]: `${rightPct}%` }}
      >
        {/* 桌面显示：固定百分比宽，内部滚动 */}
        <div className="hidden h-full min-h-0 min-w-0 overflow-y-auto lg:block" style={{ width: 'var(--split-right)' }}>
          {right}
        </div>
        {/* 移动端显示 */}
        <div className="min-h-0 lg:hidden">{right}</div>
      </div>
    </div>
  )
}
