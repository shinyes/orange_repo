import type {
  Chapter,
  Practice,
  PracticeItem,
  Problem,
  ProblemFilterState,
  ProblemPayload,
  ProblemSummary,
  TagCount,
  Training,
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
      window.dispatchEvent(new Event('orangerepo:unauthorized'))
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

export function filterQuery(f: ProblemFilterState, extra?: Record<string, string>): string {
  const p = new URLSearchParams()
  if (f.q) p.set('q', f.q)
  if (f.tags.length) p.set('tags', f.tags.join(','))
  if (f.type) p.set('type', f.type)
  for (const [k, v] of Object.entries(extra ?? {})) p.set(k, v)
  const s = p.toString()
  return s ? `?${s}` : ''
}

export const api = {
  // ---- 认证 ----
  me: () => req<{ authenticated: boolean }>('/api/auth/me'),
  login: (password: string) => req<void>('/api/auth/login', json({ method: 'POST', body: JSON.stringify({ password }) })),
  logout: () => req<void>('/api/auth/logout', { method: 'POST' }),
  changePassword: (oldPassword: string, newPassword: string) =>
    req<void>('/api/auth/password', json({ method: 'PUT', body: JSON.stringify({ oldPassword, newPassword }) })),

  // ---- 题目 ----
  problems: (f: ProblemFilterState) =>
    req<{ problems: ProblemSummary[] }>(`/api/problems${filterQuery(f)}`),
  createProblem: (payload: ProblemPayload) =>
    req<{ problem: Problem }>('/api/problems', json({ method: 'POST', body: JSON.stringify(payload) })),
  getProblem: (id: number) => req<{ problem: Problem }>(`/api/problems/${id}`),
  updateProblem: (id: number, payload: ProblemPayload) =>
    req<{ problem: Problem }>(`/api/problems/${id}`, json({ method: 'PUT', body: JSON.stringify(payload) })),
  deleteProblem: (id: number) => req<void>(`/api/problems/${id}`, { method: 'DELETE' }),

  // ---- 标签（动态 facet 计数随过滤上下文联动；子树整体重命名/删除） ----
  tags: (f?: ProblemFilterState) =>
    req<{ tags: TagCount[]; total: number }>(`/api/tags${f ? filterQuery(f) : ''}`),
  renameTag: (from: string, to: string) =>
    req<{ updated: number }>('/api/tags', json({ method: 'PATCH', body: JSON.stringify({ from, to }) })),
  deleteTag: (tag: string) =>
    req<{ updated: number }>(`/api/tags?tag=${encodeURIComponent(tag)}`, { method: 'DELETE' }),

  // ---- 图片 ----
  uploadImage: async (file: File): Promise<{ url: string }> => {
    const body = new FormData()
    body.append('file', file)
    return req<{ url: string }>('/api/images', { method: 'POST', body })
  },

  // ---- 导入导出 ----
  import: async (file: File, mode: 'problems' | 'training' | 'practice'): Promise<Record<string, unknown>> => {
    const body = new FormData()
    body.append('zip', file)
    return req(`/api/import?mode=${mode}`, { method: 'POST', body })
  },
  exportProblemsUrl: (f: ProblemFilterState, ids?: number[]) => {
    if (ids && ids.length) return `/api/export/problems${filterQuery({ ...f, q: '', tags: [], type: '' }, { ids: ids.join(',') })}`
    return `/api/export/problems${filterQuery(f, { name: '题库导出' })}`
  },
  exportTrainingUrl: (id: number) => `/api/export/trainings/${id}`,
  exportPracticeUrl: (id: number) => `/api/export/practices/${id}`,

  // ---- 训练 ----
  trainings: () => req<{ trainings: Training[] }>('/api/trainings'),
  createTraining: (title: string, description = '', tags: string[] = []) =>
    req<{ id: number }>('/api/trainings', json({ method: 'POST', body: JSON.stringify({ title, description, tags }) })),
  getTraining: (id: number) => req<{ training: Training; chapters: Chapter[] }>(`/api/trainings/${id}`),
  updateTraining: (id: number, payload: { title: string; description: string; tags: string[] }) =>
    req<void>(`/api/trainings/${id}`, json({ method: 'PUT', body: JSON.stringify(payload) })),
  deleteTraining: (id: number) => req<void>(`/api/trainings/${id}`, { method: 'DELETE' }),
  createChapter: (trainingId: number, title: string) =>
    req<{ id: number }>(`/api/trainings/${trainingId}/chapters`, json({ method: 'POST', body: JSON.stringify({ title }) })),
  updateChapter: (id: number, title: string, orderNo: number) =>
    req<void>(`/api/chapters/${id}`, json({ method: 'PUT', body: JSON.stringify({ title, orderNo }) })),
  deleteChapter: (id: number) => req<void>(`/api/chapters/${id}`, { method: 'DELETE' }),
  addChapterItems: (chapterId: number, problemIds: number[]) =>
    req<{ itemIds: number[] }>(`/api/chapters/${chapterId}/items`, json({ method: 'POST', body: JSON.stringify({ problemIds }) })),
  reorderChapterItems: (chapterId: number, itemIds: number[]) =>
    req<void>(`/api/chapters/${chapterId}/items`, json({ method: 'PUT', body: JSON.stringify({ itemIds }) })),
  deleteItem: (id: number) => req<void>(`/api/items/${id}`, { method: 'DELETE' }),

  // ---- 练习 ----
  practices: () => req<{ practices: Practice[] }>('/api/practices'),
  createPractice: (title: string, description = '', tags: string[] = []) =>
    req<{ id: number }>('/api/practices', json({ method: 'POST', body: JSON.stringify({ title, description, tags }) })),
  getPractice: (id: number) => req<{ practice: Practice; items: PracticeItem[] }>(`/api/practices/${id}`),
  updatePractice: (id: number, payload: { title: string; description: string; tags: string[] }) =>
    req<void>(`/api/practices/${id}`, json({ method: 'PUT', body: JSON.stringify(payload) })),
  deletePractice: (id: number) => req<void>(`/api/practices/${id}`, { method: 'DELETE' }),
  addPracticeItems: (practiceId: number, problemIds: number[], score = 100) =>
    req<{ itemIds: number[] }>(`/api/practices/${practiceId}/items`, json({ method: 'POST', body: JSON.stringify({ problemIds, score }) })),
  reorderPracticeItems: (practiceId: number, itemIds: number[]) =>
    req<void>(`/api/practices/${practiceId}/items`, json({ method: 'PUT', body: JSON.stringify({ itemIds }) })),
  updatePracticeItem: (id: number, score: number) =>
    req<void>(`/api/practice-items/${id}`, json({ method: 'PUT', body: JSON.stringify({ score }) })),
  deletePracticeItem: (id: number) => req<void>(`/api/practice-items/${id}`, { method: 'DELETE' }),
}
