// 小游戏注册表：休息时间的游戏列表。
// 以后新增游戏只需在此追加一条（id 同时是榜单的 game 维度，后端按 game 存分，无需改接口）。
import { lazy, type ComponentType, type LazyExoticComponent } from 'react'

export interface GameProps {
  /** 一局结束时回调（分数 = 该游戏自己的计分口径） */
  onGameOver: (score: number) => void
  /** 重新开局（变化即重建游戏） */
  gameKey: number
}

export interface GameDef {
  /** 榜单维度 key：小写字母/数字/下划线/短横线（后端校验） */
  id: string
  /** 显示名 */
  title: string
  /** 一句话说明 */
  desc: string
  /** 操作说明 */
  controls: string[]
  /** 计分单位（榜单里显示） */
  scoreUnit: string
  /** 懒加载的游戏组件 */
  component: LazyExoticComponent<ComponentType<GameProps>>
  /** 该游戏的分数上限（与服务端校验一致） */
  maxScore: number
}

export const GAMES: GameDef[] = [
  {
    id: 'dino',
    title: '小恐龙',
    desc: '谷歌浏览器离线小恐龙：跳过仙人掌，越跑越快。',
    controls: ['空格 / ↑ / 点击：跳跃', '↓：下蹲', '撞到障碍即结束'],
    scoreUnit: '分',
    maxScore: 100000,
    component: lazy(() => import('./dino/DinoGame').then((m) => ({ default: m.DinoGame }))),
  },
]

export function gameById(id: string | undefined | null): GameDef {
  return GAMES.find((g) => g.id === id) ?? GAMES[0]
}

/** 与后端一致的 game id 校验：^[a-z0-9_-]{1,32}$ */
export function isValidGameId(id: string): boolean {
  return /^[a-z0-9_-]{1,32}$/.test(id)
}
