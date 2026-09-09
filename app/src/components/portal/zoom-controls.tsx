// 题面文字缩放控制（放大/缩小/重置；作用于整块题面 zoom）——训练/做题页共用。
import { RotateCcwIcon, ZoomInIcon, ZoomOutIcon } from 'lucide-react'

export function ZoomControls({ scale, onChange }: { scale: number; onChange: (s: number) => void }) {
  const step = 0.1
  return (
    <span className="flex shrink-0 items-center gap-0.5 rounded-lg border bg-background px-1 py-0.5">
      <button
        type="button"
        title="缩小题目文字"
        className="flex size-5 items-center justify-center rounded text-muted-foreground transition-colors hover:bg-muted hover:text-foreground disabled:opacity-40"
        disabled={scale <= 0.7}
        onClick={() => onChange(Math.round((scale - step) * 100) / 100)}
      >
        <ZoomOutIcon className="size-3.5" />
      </button>
      <span className="w-9 text-center text-[10px] tabular-nums">{Math.round(scale * 100)}%</span>
      <button
        type="button"
        title="放大题目文字"
        className="flex size-5 items-center justify-center rounded text-muted-foreground transition-colors hover:bg-muted hover:text-foreground disabled:opacity-40"
        disabled={scale >= 2}
        onClick={() => onChange(Math.round((scale + step) * 100) / 100)}
      >
        <ZoomInIcon className="size-3.5" />
      </button>
      <button
        type="button"
        title="重置题目文字大小"
        className="flex size-5 items-center justify-center rounded text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
        onClick={() => onChange(1)}
      >
        <RotateCcwIcon className="size-3" />
      </button>
    </span>
  )
}
