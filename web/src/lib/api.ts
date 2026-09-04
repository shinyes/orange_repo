import type {
  BookletDirectory,
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
  // 标签逐个 append（tags=a&tags=b）：标签文本本身可含逗号，多值参数不会误拆
  for (const t of f.tags) p.append('tags', t)
  if (f.type) p.set('type', f.type)
  for (const [k, v] of Object.entries(extra ?? {})) p.set(k, v)
  const s = p.toString()
  return s ? `?${s}` : ''
}

export const api = {
  // ---- 认证 ----
  me: () => req<{ authenticated: boolean }>('/api/auth/me'),
  login: (username: string, password: string) =>
    req<void>('/api/auth/login', json({ method: 'POST', body: JSON.stringify({ username, password }) })),
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
  getTagOrder: () => req<{ order: Record<string, string[]> }>('/api/tag-order'),
  setTagOrder: (order: Record<string, string[]>) =>
    req<void>('/api/tag-order', json({ method: 'PUT', body: JSON.stringify({ order }) })),

  // ---- 图片 ----
  uploadImage: async (file: File): Promise<{ url: string }> => {
    const body = new FormData()
    body.append('file', file)
    return req<{ url: string }>('/api/images', { method: 'POST', body })
  },
  scanOrphanImages: () => req<{ orphaned: number; total: number }>('/api/uploads/cleanup?dryRun=true'),
  cleanupOrphanImages: () => req<{ removed: number }>('/api/uploads/cleanup', { method: 'POST' }),

  // ---- 导入导出 ----
  import: async (file: File, mode: 'problems' | 'training' | 'practice' | 'auto', folderId?: number | null): Promise<Record<string, unknown>> => {
    const body = new FormData()
    body.append('zip', file)
    const folder = folderId ? `&folderId=${folderId}` : ''
    return req(`/api/import?mode=${mode}${folder}`, { method: 'POST', body })
  },
  exportProblemsUrl: (f: ProblemFilterState, ids?: number[]) => {
    if (ids && ids.length) return `/api/export/problems${filterQuery({ ...f, q: '', tags: [], type: '' }, { ids: ids.join(',') })}`
    return `/api/export/problems${filterQuery(f, { name: '题库导出' })}`
  },
  exportTrainingUrl: (id: number) => `/api/export/trainings/${id}`,
  exportPracticeUrl: (id: number) => `/api/export/practices/${id}`,

  // ---- 题册目录（可嵌套） ----
  bookletDirectories: () => req<{ directories: BookletDirectory[] }>('/api/booklet-directories'),
  createBookletDirectory: (name: string, parentId: number | null = null) =>
    req<{ id: number }>('/api/booklet-directories', json({ method: 'POST', body: JSON.stringify({ name, parentId }) })),
  renameBookletDirectory: (id: number, name: string) =>
    req<void>(`/api/booklet-directories/${id}`, json({ method: 'PATCH', body: JSON.stringify({ name }) })),
  deleteBookletDirectory: (id: number, deleteBooklets = false) =>
    req<void>(`/api/booklet-directories/${id}${deleteBooklets ? '?deleteBooklets=true' : ''}`, { method: 'DELETE' }),
  setBookletDirectoryLayout: (directories: BookletDirectory[]) =>
    req<void>('/api/booklet-directories/layout', json({ method: 'PUT', body: JSON.stringify({ directories }) })),
  setTrainingFolder: (id: number, folderId: number | null) =>
    req<void>(`/api/trainings/${id}/folder`, json({ method: 'PUT', body: JSON.stringify({ folderId }) })),
  setPracticeFolder: (id: number, folderId: number | null) =>
    req<void>(`/api/practices/${id}/folder`, json({ method: 'PUT', body: JSON.stringify({ folderId }) })),

  // ---- 训练 ----
  trainings: () => req<{ trainings: Training[] }>('/api/trainings'),
  createTraining: (title: string, description = '', tags: string[] = [], folderId: number | null = null) =>
    req<{ id: number }>('/api/trainings', json({ method: 'POST', body: JSON.stringify({ title, description, tags, folderId }) })),
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
  updateTrainingLayout: (
    id: number,
    payload: { chapterIds: number[]; chapters: { chapterId: number; itemIds: number[] }[] },
  ) =>
    req<{ chapters: Chapter[] }>(`/api/trainings/${id}/layout`, json({ method: 'PUT', body: JSON.stringify(payload) })),
  deleteItem: (id: number) => req<void>(`/api/items/${id}`, { method: 'DELETE' }),

  // ---- 练习 ----
  practices: () => req<{ practices: Practice[] }>('/api/practices'),
  createPractice: (title: string, description = '', tags: string[] = [], folderId: number | null = null) =>
    req<{ id: number }>('/api/practices', json({ method: 'POST', body: JSON.stringify({ title, description, tags, folderId }) })),
  getPractice: (id: number) => req<{ practice: Practice; items: PracticeItem[] }>(`/api/practices/${id}`),
  updatePractice: (id: number, payload: { title: string; description: string; tags: string[] }) =>
    req<void>(`/api/practices/${id}`, json({ method: 'PUT', body: JSON.stringify(payload) })),
  deletePractice: (id: number) => req<void>(`/api/practices/${id}`, { method: 'DELETE' }),
  addPracticeItems: (practiceId: number, problemIds: number[]) =>
    req<{ itemIds: number[] }>(`/api/practices/${practiceId}/items`, json({ method: 'POST', body: JSON.stringify({ problemIds }) })),
  reorderPracticeItems: (practiceId: number, itemIds: number[]) =>
    req<void>(`/api/practices/${practiceId}/items`, json({ method: 'PUT', body: JSON.stringify({ itemIds }) })),
  deletePracticeItem: (id: number) => req<void>(`/api/practice-items/${id}`, { method: 'DELETE' }),
}
