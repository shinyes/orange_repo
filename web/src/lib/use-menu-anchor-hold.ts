import { useCallback, useEffect, useRef, useState } from 'react'

// 行尾 hover 菜单的锚点保持 hook：
// 菜单（portal 到 body）关闭动画期间，若触发器（定位锚点）立即 display:none，
// base-ui 的 positioner 会塌缩到页面左上角形成闪现。
// 返回 [open, setOpen, anchorVisible]：anchorVisible 在菜单关闭后仍保持 delay 毫秒，
// 确保关闭动画（约 100ms）期间锚点可见。
export function useMenuAnchorHold(delay = 400): [boolean, (v: boolean) => void, boolean] {
  const [open, setOpen] = useState(false)
  const [hold, setHold] = useState(false)
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  useEffect(() => {
    return () => {
      if (timerRef.current) clearTimeout(timerRef.current)
    }
  }, [])

  const setOpenSafe = useCallback(
    (v: boolean) => {
      setOpen(v)
      if (!v) {
        setHold(true)
        if (timerRef.current) clearTimeout(timerRef.current)
        timerRef.current = setTimeout(() => setHold(false), delay)
      }
    },
    [delay],
  )

  return [open, setOpenSafe, open || hold]
}