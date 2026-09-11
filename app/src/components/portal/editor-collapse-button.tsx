// 折叠/展开「代码编辑器 + 控制台」的切换按钮（编程题做题页共用）：
// 折叠后题面占满整宽/整屏，再次点击恢复；编辑器仅隐藏不卸载（草稿与状态不丢）。
import { PanelRightCloseIcon, PanelRightOpenIcon } from 'lucide-react'
import { cn } from '@/lib/utils'

export function EditorCollapseButton({ collapsed, onToggle, className }: {
  collapsed: boolean
  onToggle: () => void
  className?: string
}) {
  const Icon = collapsed ? PanelRightOpenIcon : PanelRightCloseIcon
  return (
    <button
      type="button"
      title={collapsed ? '展开代码编辑器与控制台' : '折叠代码编辑器与控制台'}
      aria-label={collapsed ? '展开代码编辑器与控制台' : '折叠代码编辑器与控制台'}
      aria-pressed={collapsed}
      onClick={onToggle}
      className={cn(
        'inline-flex size-6 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-muted hover:text-primary',
        collapsed && 'bg-primary/10 text-primary',
        className,
      )}
    >
      <Icon className="size-3.5" />
    </button>
  )
}
