// 小恐龙游戏 React 适配层：把内联的上游实现（BSD-3-Clause，见 dino-runner.js）包成组件。
// 上游要求：容器存在于 DOM、页面里有 .icon-offline 元素、有 offline-resources-1x/2x 精灵图；
// 这里全部在组件内提供（素材经 Vite 打包为本地资源，离线可用、无外部请求）。
import { useEffect, useRef } from 'react'

import { createDinoRunner, dinoCrashed, dinoScore, disposeDinoRunner, type DinoRunner } from './dino-runner'
import sprite1x from './sprites-1x.png'
import sprite2x from './sprites-2x.png'
import './dino.css'

export function DinoGame({ onGameOver, gameKey = 0 }: {
  /** 一局结束（撞击）时回调，参数为该局分数 */
  onGameOver: (score: number) => void
  /** 变化时重建游戏（用于"重新开始/换游戏"） */
  gameKey?: number
}) {
  const hostRef = useRef<HTMLDivElement | null>(null)
  const overRef = useRef(onGameOver)
  overRef.current = onGameOver

  useEffect(() => {
    const host = hostRef.current
    if (!host) return
    let runner: DinoRunner | null = null
    let raf = 0
    let disposed = false
    let wasCrashed = false

    // 上游用选择器找容器：给它一个唯一 class
    const cls = `dino-host-${Math.random().toString(36).slice(2, 10)}`
    host.classList.add(cls)

    // 精灵图必须先加载完成：Runner 构造时读取 naturalWidth/Height 计算 sprite 坐标
    const images = Array.from(document.querySelectorAll<HTMLImageElement>('img[data-dino-sprite]'))
    const ready = images.length > 0 && images.every((img) => img.complete && img.naturalWidth > 0)
    const start = () => {
      if (disposed) return
      // 上游会把新建的 .runner-container/canvas **追加**到宿主节点；重开（gameKey 变化）时
      // 若不清空，就会在下面再叠一个游戏窗口。这里先清空，保证任何时刻只有一个。
      host.replaceChildren()
      runner = createDinoRunner(`.${cls}`)
      const tick = () => {
        if (disposed) return
        const crashed = dinoCrashed(runner)
        if (crashed && !wasCrashed) {
          wasCrashed = true
          overRef.current(dinoScore(runner))
        } else if (!crashed) {
          wasCrashed = false
        }
        raf = window.requestAnimationFrame(tick)
      }
      raf = window.requestAnimationFrame(tick)
    }
    if (ready) start()
    else window.addEventListener('load', start, { once: true })

    return () => {
      disposed = true
      window.removeEventListener('load', start)
      if (raf) window.cancelAnimationFrame(raf)
      disposeDinoRunner(runner)
      // 清掉上游插入的 canvas/容器，避免残留（配合 replaceChildren 双保险）
      try {
        host.replaceChildren()
      } catch { /* 忽略 */ }
      host.classList.remove(cls)
    }
  }, [gameKey])

  return (
    <div className="w-full">
      {/* 上游依赖的元素：隐藏的静态图标占位 + 精灵图（1x/2x） */}
      <span className="icon-offline hidden" aria-hidden="true" />
      <img data-dino-sprite id="offline-resources-1x" src={sprite1x} alt="" className="hidden" />
      <img data-dino-sprite id="offline-resources-2x" src={sprite2x} alt="" className="hidden" />
      <div ref={hostRef} className="dino-stage w-full max-w-[600px] px-0 py-2" />
    </div>
  )
}
