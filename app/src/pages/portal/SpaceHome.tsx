// 空间首页（/s/:spaceId）：训练/练习/刷题/排行榜四大入口卡片。
// 点击进入各自独立列表页（左上角返回本页）。
import { Link } from 'react-router-dom'
import { BookOpenIcon, ClipboardListIcon, FolderKanbanIcon, TrophyIcon } from 'lucide-react'

import { usePortalCtx } from './SpaceShell'
import { useSpaceHome } from './useSpaceHome'

export function SpaceHome() {
  const { space } = usePortalCtx()
  const home = useSpaceHome(space)
  const tr = home.data?.trainings?.length ?? 0
  const pr = home.data?.practices?.length ?? 0
  const qz = home.data?.quizzes?.length ?? 0

  const entries = [
    {
      to: `/s/${space.id}/training`,
      title: '训练',
      desc: '按章节组织的题单，客观题限次作答（答对绿勾 / 次数用尽标红），编程题不限次',
      icon: FolderKanbanIcon,
      count: tr,
      countLabel: tr > 0 ? `${tr} 个训练` : '暂无训练',
    },
    {
      to: `/s/${space.id}/practice`,
      title: '练习',
      desc: '整卷作答后统一交卷评分，可重做且每次作答留档',
      icon: ClipboardListIcon,
      count: pr,
      countLabel: pr > 0 ? `${pr} 套练习` : '暂无练习',
    },
    {
      to: `/s/${space.id}/quiz`,
      title: '刷题',
      desc: '随机单题即时反馈：答对记通过（uuid 去重），答错不限次数',
      icon: BookOpenIcon,
      count: qz,
      countLabel: qz > 0 ? `${qz} 个刷题项目` : '暂无刷题项目',
    },
    {
      to: `/s/${space.id}/rank`,
      title: '排行榜',
      desc: '本空间所属域的通过题数总榜（管理员不参与）',
      icon: TrophyIcon,
      count: -1,
      countLabel: '查看域内总榜',
    },
  ]

  return (
    <div className="mx-auto w-full max-w-3xl px-4 py-6 lg:px-8">
      <h1 className="mb-1 text-lg font-semibold">点击进入一个空间</h1>
      <p className="mb-4 text-xs text-muted-foreground">空间「{space.name}」——选择一项开始：</p>
      {home.isLoading && <p className="py-8 text-center text-sm text-muted-foreground">加载中…</p>}
      <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
        {entries.map((e) => (
          <Link
            key={e.to}
            to={e.to}
            className="flex flex-col gap-1.5 rounded-2xl border bg-card p-5 transition-colors hover:border-primary/50 hover:bg-primary/5"
          >
            <span className="flex items-center gap-2 font-medium">
              <e.icon className="size-4 shrink-0 text-primary" />
              {e.title}
            </span>
            <span className="line-clamp-2 text-xs text-muted-foreground">{e.desc}</span>
            <span className="mt-1 text-xs text-muted-foreground/70">{e.countLabel}</span>
          </Link>
        ))}
      </div>
    </div>
  )
}
