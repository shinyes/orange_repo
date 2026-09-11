// 编程工作区共享逻辑（做题页 ProblemSolvePage / 训练卡 TrainingProgrammingCard 共用）：
//  1. resolveStarter —— 起始代码解析：题目级模板(starterPy/starterCpp)非空优先，否则通用模板；
//  2. useCloudDraft —— 云端草稿加载（按登录用户隔离在服务端；同一题×语言语义，跨端/跨页互通）；
//  3. saveDraftDebounced —— 云端草稿 debounce 自动保存（fire-and-forget，失败静默——本地草稿已实时
//     写 localStorage 作离线缓存，页面自身的草稿写入逻辑保留）。
// 各页保留各自的本地草稿 key 与渲染细节，初始代码顺序统一为：
//     本地草稿 → 云端草稿（异步）→ 题目模板 → 通用模板。
import { useEffect, useState } from 'react'
import { api } from '@/api'
import type { CodeLang } from '@/api/types'

/** 云端保存 debounce 间隔。 */
const DRAFT_SAVE_DEBOUNCE_MS = 2000

/** 云端保存计时器（按 题目×语言 独立计时，跨组件实例共用同一 key 语义）。 */
const draftTimers = new Map<string, ReturnType<typeof setTimeout>>()

/** 无题目级模板时的兜底：空串（设计上不提供默认模板——留空即空白编辑器，学生自己写）。 */
export function genericStarter(_lang: CodeLang): string {
  return ''
}

/** 起始代码解析：题目级模板（starterCpp/starterPy）非空用之，否则空（无默认模板）。 */
export function resolveStarter(
  lang: CodeLang,
  problem?: { starterCpp?: string; starterPy?: string } | null,
): string {
  const t = problem ? (lang === 'cpp' ? problem.starterCpp : problem.starterPy) : undefined
  if (t != null && t.trim() !== '') return t
  return ''
}

/**
 * 云端草稿加载：挂载 / 语言切换后 GET 一次该 题×语言×上下文 的草稿。
 * ctxKind/ctxId：''=全局做题页 / training=训练 / practice=练习（草稿按上下文隔离）。
 * 返回 { cloudLoaded, initialCode, updatedAt }——updatedAt 为云端最后保存时间（毫秒时间戳，
 * 无草稿/解析失败为 0），由调用方与本地时间戳比较，避免用旧草稿覆盖新草稿。
 */
export function useCloudDraft(problemId: number, lang: CodeLang, ctxKind?: string, ctxId?: number): { cloudLoaded: boolean; initialCode: string; updatedAt: number } {
  const [state, setState] = useState<{ cloudLoaded: boolean; initialCode: string; updatedAt: number }>({ cloudLoaded: false, initialCode: '', updatedAt: 0 })
  useEffect(() => {
    let alive = true
    setState({ cloudLoaded: false, initialCode: '', updatedAt: 0 })
    if (!problemId) {
      setState({ cloudLoaded: true, initialCode: '', updatedAt: 0 })
      return
    }
    api
      .ojDraft(problemId, lang, ctxKind, ctxId)
      .then((d) => {
        const ts = d?.updatedAt ? Date.parse(d.updatedAt) : 0
        if (alive) {
          setState({ cloudLoaded: true, initialCode: d?.code ?? '', updatedAt: Number.isFinite(ts) ? ts : 0 })
        }
      })
      .catch(() => {
        // 取草稿失败按“无草稿”处理（本地草稿/模板仍兜底，不打断做题）
        if (alive) setState({ cloudLoaded: true, initialCode: '', updatedAt: 0 })
      })
    return () => {
      alive = false
    }
  }, [problemId, lang, ctxKind, ctxId])
  return state
}

// ---------- 本地草稿时间戳 ----------
// 本地草稿写入时记下时间（毫秒），与服务端 updatedAt 比较新旧。
// 键：草稿键 + ':ts'（与草稿内容同生命周期，清理草稿时一并删除）。

/** 记录本地草稿写入时间。 */
export function markLocalDraftTime(localKey: string): void {
  try {
    localStorage.setItem(localKey + ':ts', String(Date.now()))
  } catch {
    // localStorage 不可用（隐私模式）：跳过时间戳，退化为“本地优先”的旧行为
  }
}

/** 取本地草稿写入时间（无记录/不可用返回 0）。 */
export function localDraftTime(localKey: string): number {
  try {
    const raw = localStorage.getItem(localKey + ':ts')
    const n = raw ? Number(raw) : 0
    return Number.isFinite(n) ? n : 0
  } catch {
    return 0
  }
}

/** 云端草稿被采纳时，把本地时间戳同步为云端时间（保持两者一致）。 */
export function syncLocalDraftTime(localKey: string, cloudTs: number): void {
  if (!cloudTs) return
  try {
    localStorage.setItem(localKey + ':ts', String(cloudTs))
  } catch {
    // 忽略
  }
}

/**
 * 云端草稿是否应当覆盖本地：仅当云端的保存时间**严格新于**本地记录时间。
 * 本地时间戳缺失（0，如本次改动前的存量草稿）时返回 false——沿用“本地优先”，
 * 避免升级后把用户本机的草稿静默替换掉。
 */
export function cloudDraftIsNewer(cloudTs: number, localTs: number): boolean {
  return localTs > 0 && cloudTs > localTs
}

/**
 * 云端草稿 debounce 保存（PUT /api/oj/problem/:id/draft）。
 * ctxKind/ctxId：草稿上下文（训练/练习/全局隔离）。
 * 静默失败（本地草稿已实时写入 localStorage 作离线缓存）。
 */
export function saveDraftDebounced(problemId: number, lang: CodeLang, code: string, ctxKind?: string, ctxId?: number): void {
  const key = `${problemId}:${lang}:${ctxKind ?? ''}:${ctxId ?? 0}`
  const timer = draftTimers.get(key)
  if (timer) clearTimeout(timer)
  draftTimers.set(
    key,
    setTimeout(() => {
      draftTimers.delete(key)
      void api.ojSaveDraft(problemId, lang, code, ctxKind, ctxId).catch(() => {
        // 静默：本地已缓存，下次编辑会再次尝试
      })
    }, DRAFT_SAVE_DEBOUNCE_MS),
  )
}
