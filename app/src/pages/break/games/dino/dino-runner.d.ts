// 内联的 wayou/t-rex-runner（BSD-3-Clause）适配层类型声明。
// 源码为上游逐字节拷贝 + 文件末尾适配段，故此处只声明我们用到的接口。

/** 小恐龙实例（仅列出本页用到/覆盖的字段）。 */
export interface DinoRunner {
  /** 是否已撞击结束 */
  crashed?: boolean
  /** 是否正在游戏中 */
  playing?: boolean
  /** 已跑距离（内部单位） */
  distanceRan?: number
  distanceMeter?: { getActualDistance: (distance: number) => number }
  /** 停止当前回合 */
  stop?: () => void
  /** 覆盖：禁用音效加载 */
  loadSounds?: () => void
  /** 覆盖：禁用街机（全屏缩放）模式 */
  setArcadeMode?: () => void
  setArcadeModeContainerScale?: () => void
}

/** 在 containerSelector 指定的容器内创建游戏（容器需已存在于 DOM）。 */
export function createDinoRunner(containerSelector: string): DinoRunner

/** 当前分数（与画面显示同口径）。 */
export function dinoScore(runner: DinoRunner | null): number

/** 是否处于撞击结束状态。 */
export function dinoCrashed(runner: DinoRunner | null): boolean

/** 释放单例引用（组件卸载时必须调用，否则重挂载会复用旧实例）。 */
export function disposeDinoRunner(runner: DinoRunner | null): void
