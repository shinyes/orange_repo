// 控制台高度缩放共享逻辑（训练内嵌卡 / 独立做题页共用）：
// 默认约 5 行（112px），上下拖动句柄调整（60–360px），双击/按钮重置。
import { useRef, useState, type PointerEvent as ReactPointerEvent } from 'react'

export const CONSOLE_DEFAULT_H = 112
export const CONSOLE_MIN_H = 60
export const CONSOLE_MAX_H = 360

export function useConsoleResize(defaultH = CONSOLE_DEFAULT_H) {
  const [consoleH, setConsoleH] = useState(defaultH)
  const heightRef = useRef(defaultH)
  const dragRef = useRef<{ startY: number; startH: number } | null>(null)

  const setHeight = (h: number) => {
    const v = Math.min(CONSOLE_MAX_H, Math.max(CONSOLE_MIN_H, h))
    heightRef.current = v
    setConsoleH(v)
  }

  /** 句柄 onPointerDown：开始拖动（上拖=放大） */
  function startDrag(e: ReactPointerEvent) {
    if (e.button !== 0) return
    e.preventDefault()
    dragRef.current = { startY: e.clientY, startH: heightRef.current }
    document.body.style.userSelect = 'none'
    document.body.style.cursor = 'ns-resize'
    const onMove = (ev: PointerEvent) => {
      const d = dragRef.current
      if (!d) return
      setHeight(d.startH + (d.startY - ev.clientY))
    }
    const onUp = () => {
      dragRef.current = null
      document.body.style.userSelect = ''
      document.body.style.cursor = ''
      window.removeEventListener('pointermove', onMove)
      window.removeEventListener('pointerup', onUp)
    }
    window.addEventListener('pointermove', onMove)
    window.addEventListener('pointerup', onUp)
  }

  const reset = () => setHeight(defaultH)

  return { consoleH, startDrag, reset, setHeight }
}
