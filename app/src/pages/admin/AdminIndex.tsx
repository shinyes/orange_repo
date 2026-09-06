// 管理区首页（占位）：第二阶段把原 web/（管理端）功能迁入 /admin 路由树。
// 现仅确认「管理区路由 + 顶栏入口 + 门户路由共存」骨架可用。
import { ConstructionIcon } from 'lucide-react'

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'

export function AdminIndex() {
  return (
    <div className="mx-auto w-full max-w-3xl px-4 py-10 lg:px-8">
      <h1 className="mb-1 text-lg font-semibold">管理区</h1>
      <p className="mb-5 text-xs text-muted-foreground">域名、空间、题目与成员等内容管理（第二阶段迁入）</p>
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-sm">
            <ConstructionIcon className="size-4 text-primary" />
            管理区（待迁移）
          </CardTitle>
          <CardDescription>
            原主站 web/ 管理端将在此合并（仓库/域/空间/题目/成员管理）；当前为占位页，仅验证 /admin 路由与门户共存。
          </CardDescription>
        </CardHeader>
        <CardContent className="text-xs text-muted-foreground">
          空间内容管理入口仍可通过做题空间顶栏跳转体验；正式管理功能见第二阶段。
        </CardContent>
      </Card>
    </div>
  )
}
