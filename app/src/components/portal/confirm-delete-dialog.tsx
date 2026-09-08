// 通用删除确认弹窗（门户空间内容列表共用：训练/练习/刷题卡片删除）。
// 基于 AlertDialog，红色确认按钮；busy 时禁用取消/确认并显示「删除中…」，
// 避免请求进行中对话框被关闭。禁止 window.confirm 的替代实现。
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'

export function ConfirmDeleteDialog(props: {
  open: boolean
  onOpenChange: (v: boolean) => void
  title: string
  description?: string
  /** 点击确认后的删除动作（可为 async）；失败时应自行 toast 并保持 open 以便重试 */
  onConfirm: () => void | Promise<void>
  /** 删除请求进行中：禁用按钮并显示处理中 */
  busy?: boolean
  confirmLabel?: string
}) {
  return (
    <AlertDialog open={props.open} onOpenChange={props.onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{props.title}</AlertDialogTitle>
          {props.description && <AlertDialogDescription>{props.description}</AlertDialogDescription>}
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={props.busy}>取消</AlertDialogCancel>
          <AlertDialogAction
            variant="destructive"
            disabled={props.busy}
            onClick={() => {
              // AlertDialogAction 为普通按钮：不自动关闭，由调用方在 onConfirm 成功后关闭
              void props.onConfirm()
            }}
          >
            {props.busy ? '删除中…' : (props.confirmLabel ?? '确认删除')}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
