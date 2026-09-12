// 「开放」开关行：训练/练习/刷题 的可见性设置（可见性浮窗内）共用。
// 语义：**开放** + **已分配** 两个条件同时满足时，成员才可见该项目。
import { Switch } from '@/components/ui/switch'

export function PublicToggleRow(props: {
  checked: boolean
  disabled?: boolean
  onCheckedChange: (v: boolean) => void
}) {
  return (
    <div className="flex items-center justify-between gap-3 rounded-lg border bg-background px-3 py-2.5">
      <div className="min-w-0">
        <p className="text-sm font-medium">开放</p>
        <p className="text-xs leading-relaxed text-muted-foreground">
          成员需同时满足「已开放」且「在分配名单中」才可见；未开放时仅管理员可见
        </p>
      </div>
      <Switch
        checked={props.checked}
        disabled={props.disabled}
        onCheckedChange={props.onCheckedChange}
        className="shrink-0"
      />
    </div>
  )
}
