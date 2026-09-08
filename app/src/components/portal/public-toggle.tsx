// 「公开」开关行：训练/练习/刷题 编辑与新建弹窗共用。
// 公开后空间内所有成员可见；关闭则仅分配给可见名单的成员可见。
import { Switch } from '@/components/ui/switch'

export function PublicToggleRow(props: {
  checked: boolean
  disabled?: boolean
  onCheckedChange: (v: boolean) => void
}) {
  return (
    <div className="flex items-center justify-between gap-3 rounded-lg border bg-background px-3 py-2.5">
      <div className="min-w-0">
        <p className="text-sm font-medium">公开</p>
        <p className="text-xs leading-relaxed text-muted-foreground">
          公开后空间内所有成员可见；关闭则仅分配给可见名单的成员可见
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
