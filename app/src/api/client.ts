// API 传输层（统一 fetch 封装，门户与管理端共用）：
//  - 同源携带 cookie（credentials: 'same-origin'），与后端单进程 internal/app（/api 全合并、单 cookie orange_session）对接；
//  - 401（非 /api/auth/* 路径）视为会话失效：派发全局事件，由 App 根组件清缓存并回登录页。
import type { User } from './types'

export class ApiError extends Error {
  status: number
  constructor(status: number, message: string) {
    super(message)
    this.status = status
  }
}

// 会话失效事件名：src/app/App.tsx 监听（queryClient.clear() → 回登录页）。
export const UNAUTHORIZED_EVENT = 'app:unauthorized'

export async function req<T>(path: string, init?: RequestInit): Promise<T> {
  const resp = await fetch(path, {
    credentials: 'same-origin',
    ...init,
  })
  if (!resp.ok) {
    if (resp.status === 401 && !path.startsWith('/api/auth/')) {
      window.dispatchEvent(new Event(UNAUTHORIZED_EVENT))
    }
    let msg = `HTTP ${resp.status}`
    try {
      const data = (await resp.json()) as { error?: string }
      if (data.error) msg = data.error
    } catch {
      // 忽略非 JSON 错误体
    }
    throw new ApiError(resp.status, msg)
  }
  if (resp.status === 204) return undefined as T
  const ct = resp.headers.get('Content-Type') ?? ''
  if (ct.includes('json')) return (await resp.json()) as T
  return undefined as T
}

export function json(init?: RequestInit): RequestInit {
  return { ...init, headers: { 'Content-Type': 'application/json', ...init?.headers } }
}

// 导出供类型推导（/api/auth/me 返回值等）。
export type { User }
