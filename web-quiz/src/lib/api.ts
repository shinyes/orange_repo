import type {
  AdminStudent,
  AdminSubject,
  ProblemType,
  Round,
  Settings,
  SubjectBrief,
  SubmitResult,
  User,
  WrongRound,
  WrongSummary,
} from './types'

export class ApiError extends Error {
  status: number
  constructor(status: number, message: string) {
    super(message)
    this.status = status
  }
}

async function req<T>(path: string, init?: RequestInit): Promise<T> {
  const resp = await fetch(path, {
    credentials: 'same-origin',
    ...init,
  })
  if (!resp.ok) {
    if (resp.status === 401 && !path.startsWith('/api/auth/')) {
      window.dispatchEvent(new Event('quiz:unauthorized'))
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

function json(init?: RequestInit): RequestInit {
  return { ...init, headers: { 'Content-Type': 'application/json', ...(init?.headers ?? {}) } }
}

export const api = {
  // ---- 认证 ----
  me: () => req<{ authenticated: boolean; user?: User }>('/api/auth/me'),
  login: (username: string, password: string) =>
    req<void>('/api/auth/login', json({ method: 'POST', body: JSON.stringify({ username, password }) })),
  logout: () => req<void>('/api/auth/logout', { method: 'POST' }),
  changePassword: (oldPassword: string, newPassword: string) =>
    req<void>('/api/auth/password', json({ method: 'PUT', body: JSON.stringify({ oldPassword, newPassword }) })),

  // ---- 学生端 ----
  subjects: () => req<{ subjects: SubjectBrief[] }>('/api/quiz/subjects'),
  startRound: (categoryId: number) =>
    req<Round>('/api/quiz/round', json({ method: 'POST', body: JSON.stringify({ categoryId }) })),
  submit: (payload: { problemId: number; categoryId: number; optionIndex?: number; answer?: boolean }) =>
    req<SubmitResult>('/api/quiz/submit', json({ method: 'POST', body: JSON.stringify(payload) })),
  wrongSummary: () => req<WrongSummary>('/api/quiz/wrong-summary'),
  startWrongRound: (categoryId?: number) =>
    req<WrongRound>('/api/quiz/wrong-round', json({ method: 'POST', body: JSON.stringify({ categoryId: categoryId ?? null }) })),

  // ---- 管理员：科目与分类 ----
  adminSubjects: () => req<{ subjects: AdminSubject[] }>('/api/admin/subjects'),
  createSubject: (name: string) =>
    req<{ id: number }>('/api/admin/subjects', json({ method: 'POST', body: JSON.stringify({ name }) })),
  renameSubject: (id: number, name: string) =>
    req<void>(`/api/admin/subjects/${id}`, json({ method: 'PATCH', body: JSON.stringify({ name }) })),
  deleteSubject: (id: number) => req<void>(`/api/admin/subjects/${id}`, { method: 'DELETE' }),
  setSubjectOrder: (ids: number[]) =>
    req<void>('/api/admin/subjects/order', json({ method: 'PUT', body: JSON.stringify({ ids }) })),
  createCategory: (payload: { subjectId: number; name: string; orderNo?: number; tags: string[]; types: ProblemType[] }) =>
    req<{ id: number }>('/api/admin/categories', json({ method: 'POST', body: JSON.stringify(payload) })),
  updateCategory: (
    id: number,
    payload: { name?: string; orderNo?: number; tags?: string[]; types?: ProblemType[] },
  ) => req<void>(`/api/admin/categories/${id}`, json({ method: 'PATCH', body: JSON.stringify(payload) })),
  deleteCategory: (id: number) => req<void>(`/api/admin/categories/${id}`, { method: 'DELETE' }),
  setCategoryOrder: (subjectId: number, ids: number[]) =>
    req<void>(`/api/admin/subjects/${subjectId}/categories/order`, json({ method: 'PUT', body: JSON.stringify({ ids }) })),
  problemsCount: (tags: string[], types: ProblemType[]) => {
    const p = new URLSearchParams()
    if (tags.length) p.set('tags', tags.join(','))
    if (types.length) p.set('types', types.join(','))
    const s = p.toString()
    return req<{ count: number }>(`/api/admin/problems-count${s ? `?${s}` : ''}`)
  },

  // ---- 管理员：学生与设置 ----
  students: () => req<{ students: AdminStudent[] }>('/api/admin/students'),
  createStudent: (username: string, password: string) =>
    req<{ id: number }>('/api/admin/students', json({ method: 'POST', body: JSON.stringify({ username, password }) })),
  resetStudentPassword: (id: number, password: string) =>
    req<void>(`/api/admin/students/${id}/password`, json({ method: 'PUT', body: JSON.stringify({ password }) })),
  deleteStudent: (id: number) => req<void>(`/api/admin/students/${id}`, { method: 'DELETE' }),
  admins: () => req<{ admins: User[] }>('/api/admin/admins'),
  resetAdminPassword: (id: number, password: string) =>
    req<void>(`/api/admin/admins/${id}/password`, json({ method: 'PUT', body: JSON.stringify({ password }) })),
  settings: () => req<Settings>('/api/admin/settings'),
  putSettings: (roundSize: number) =>
    req<void>('/api/admin/settings', json({ method: 'PUT', body: JSON.stringify({ roundSize }) })),
}