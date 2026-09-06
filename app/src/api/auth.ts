// 认证命名空间：登录/登出/改密/会话。
// 门户与管理端共用同一会话（单 cookie orange_session），本命名空间两区复用。
import { req, json } from './client'
import type { User } from './types'

export const authApi = {
  me: () => req<{ authenticated: boolean; user?: User }>('/api/auth/me'),
  login: (username: string, password: string) =>
    req<void>('/api/auth/login', json({ method: 'POST', body: JSON.stringify({ username, password }) })),
  logout: () => req<void>('/api/auth/logout', { method: 'POST' }),
  changePassword: (oldPassword: string, newPassword: string) =>
    req<void>('/api/auth/password', json({ method: 'PUT', body: JSON.stringify({ oldPassword, newPassword }) })),
}
