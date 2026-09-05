import type {
  BookletDirectory,
  Chapter,
  Domain,
  DomainAdminUser,
  MemberUser,
  MeResult,
  Practice,
  PracticeItem,
  Problem,
  ProblemFilterState,
  ProblemPayload,
  ProblemSummary,
  Space,
  SpaceChapter,
  SpaceMember,
  SpacePractice,
  SpacePracticeItem,
  SpaceQuiz,
  SpaceTraining,
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

// 当前域上下文（global_admin 在域下拉选中后由 DomainProvider 调用 setDomain 设置；
// domain_admin 登录时即设置为归属域）。所有带域数据的请求 URL 由 dq() 附加 domainId=。
let currentDomainId: number | null = null

/** 设置当前域（null=全局/未选域）。影响此后全部带域请求的 domainId 附加。 */
export function setDomain(domainId: number | null) {
  currentDomainId = domainId
}

/** 获取当前域（供调用方在需要显式传递时读取）。 */
export function getDomain(): number | null {
  return currentDomainId
}

/** 为带域数据的路径附加当前域 query：未选域时原样返回，避免漏 domainId 的裸调用。 */
function dq(path: string): string {
  if (currentDomainId == null) return path
  const sep = path.includes('?') ? '&' : '?'
  return `${path}${sep}domainId=${currentDomainId}`
}

async function req<T>(path: string, init?: RequestInit): Promise<T> {
  const resp = await fetch(path, {
    credentials: 'same-origin',
    ...init,
  })
  if (!resp.ok) {
    if (resp.status === 401 && !path.startsWith('/api/auth/')) {
      window.dispatchEvent(new Event('OrangeOJ:unauthorized'))
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
  me: () => req<MeResult>('/api/auth/me'),
  login: (username: string, password: string) =>
    req<void>('/api/auth/login', json({ method: 'POST', body: JSON.stringify({ username, password }) })),
  logout: () => req<void>('/api/auth/logout', { method: 'POST' }),
  changePassword: (oldPassword: string, newPassword: string) =>
    req<void>('/api/auth/password', json({ method: 'PUT', body: JSON.stringify({ oldPassword, newPassword }) })),

  // ---- 题目 ----
  problems: (f: ProblemFilterState) =>
    req<{ problems: ProblemSummary[] }>(dq(`/api/problems${filterQuery(f)}`)),
  createProblem: (payload: ProblemPayload) =>
    req<{ problem: Problem }>(dq('/api/problems'), json({ method: 'POST', body: JSON.stringify(payload) })),
  getProblem: (id: number) => req<{ problem: Problem }>(`/api/problems/${id}`),
  updateProblem: (id: number, payload: ProblemPayload) =>
    req<{ problem: Problem }>(`/api/problems/${id}`, json({ method: 'PUT', body: JSON.stringify(payload) })),
  deleteProblem: (id: number) => req<void>(`/api/problems/${id}`, { method: 'DELETE' }),

  // ---- 标签（动态 facet 计数随过滤上下文联动；子树整体重命名/删除） ----
  tags: (f?: ProblemFilterState) =>
    req<{ tags: TagCount[]; total: number }>(dq(`/api/tags${f ? filterQuery(f) : ''}`)),
  renameTag: (from: string, to: string) =>
    req<{ updated: number }>(dq('/api/tags'), json({ method: 'PATCH', body: JSON.stringify({ from, to }) })),
  deleteTag: (tag: string) =>
    req<{ updated: number }>(dq(`/api/tags?tag=${encodeURIComponent(tag)}`), { method: 'DELETE' }),
  getTagOrder: () => req<{ order: Record<string, string[]> }>(dq('/api/tag-order')),
  setTagOrder: (order: Record<string, string[]>) =>
    req<void>(dq('/api/tag-order'), json({ method: 'PUT', body: JSON.stringify({ order }) })),

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
    return req(dq(`/api/import?mode=${mode}${folder}`), { method: 'POST', body })
  },
  exportProblemsUrl: (f: ProblemFilterState, ids?: number[]) => {
    if (ids && ids.length) return dq(`/api/export/problems${filterQuery({ ...f, q: '', tags: [], type: '' }, { ids: ids.join(',') })}`)
    return dq(`/api/export/problems${filterQuery(f, { name: '题库导出' })}`)
  },
  exportTrainingUrl: (id: number) => dq(`/api/export/trainings/${id}`),
  exportPracticeUrl: (id: number) => dq(`/api/export/practices/${id}`),
  // 全库备份/迁移（全库级，不带域）
  exportBackupUrl: () => `/api/export/backup`,
  importBackup: async (file: File): Promise<{ imported: number; trainings: number; practices: number }> => {
    const body = new FormData()
    body.append('zip', file)
    return req('/api/import/backup', { method: 'POST', body })
  },

  // ---- 题册目录（可嵌套） ----
  bookletDirectories: () => req<{ directories: BookletDirectory[] }>(dq('/api/booklet-directories')),
  createBookletDirectory: (name: string, parentId: number | null = null) =>
    req<{ id: number }>(dq('/api/booklet-directories'), json({ method: 'POST', body: JSON.stringify({ name, parentId }) })),
  renameBookletDirectory: (id: number, name: string) =>
    req<void>(dq(`/api/booklet-directories/${id}`), json({ method: 'PATCH', body: JSON.stringify({ name }) })),
  deleteBookletDirectory: (id: number, deleteBooklets = false) =>
    req<void>(dq(`/api/booklet-directories/${id}${deleteBooklets ? '?deleteBooklets=true' : ''}`), { method: 'DELETE' }),
  setBookletDirectoryLayout: (directories: BookletDirectory[]) =>
    req<void>(dq('/api/booklet-directories/layout'), json({ method: 'PUT', body: JSON.stringify({ directories }) })),
  setTrainingFolder: (id: number, folderId: number | null) =>
    req<void>(`/api/trainings/${id}/folder`, json({ method: 'PUT', body: JSON.stringify({ folderId }) })),
  setPracticeFolder: (id: number, folderId: number | null) =>
    req<void>(`/api/practices/${id}/folder`, json({ method: 'PUT', body: JSON.stringify({ folderId }) })),

  // ---- 训练 ----
  trainings: () => req<{ trainings: Training[] }>(dq('/api/trainings')),
  createTraining: (title: string, description = '', tags: string[] = [], folderId: number | null = null) =>
    req<{ id: number }>(dq('/api/trainings'), json({ method: 'POST', body: JSON.stringify({ title, description, tags, folderId }) })),
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
  practices: () => req<{ practices: Practice[] }>(dq('/api/practices')),
  createPractice: (title: string, description = '', tags: string[] = [], folderId: number | null = null) =>
    req<{ id: number }>(dq('/api/practices'), json({ method: 'POST', body: JSON.stringify({ title, description, tags, folderId }) })),
  getPractice: (id: number) => req<{ practice: Practice; items: PracticeItem[] }>(`/api/practices/${id}`),
  updatePractice: (id: number, payload: { title: string; description: string; tags: string[] }) =>
    req<void>(`/api/practices/${id}`, json({ method: 'PUT', body: JSON.stringify(payload) })),
  deletePractice: (id: number) => req<void>(`/api/practices/${id}`, { method: 'DELETE' }),
  addPracticeItems: (practiceId: number, problemIds: number[]) =>
    req<{ itemIds: number[] }>(`/api/practices/${practiceId}/items`, json({ method: 'POST', body: JSON.stringify({ problemIds }) })),
  reorderPracticeItems: (practiceId: number, itemIds: number[]) =>
    req<void>(`/api/practices/${practiceId}/items`, json({ method: 'PUT', body: JSON.stringify({ itemIds }) })),
  deletePracticeItem: (id: number) => req<void>(`/api/practice-items/${id}`, { method: 'DELETE' }),

  // ---- 域管理（global_admin） ----
  domains: () => req<{ domains: Domain[] }>('/api/admin/domains'),
  createDomain: (name: string, adminUsername?: string, adminPassword?: string) =>
    req<{ id: number }>(
      '/api/admin/domains',
      json({ method: 'POST', body: JSON.stringify({ name, adminUsername, adminPassword }) }),
    ),
  renameDomain: (id: number, name: string) =>
    req<void>(`/api/admin/domains/${id}`, json({ method: 'PATCH', body: JSON.stringify({ name }) })),
  /** 删除域。域内有题目时后端返回 409；届时调用方应提示并带 deleteProblems=true 重试。 */
  deleteDomain: (id: number, deleteProblems = false) =>
    req<void>(`/api/admin/domains/${id}${deleteProblems ? '?deleteProblems=true' : ''}`, { method: 'DELETE' }),
  domainAdmins: (id: number) => req<{ admins: DomainAdminUser[] }>(`/api/admin/domains/${id}/admins`),
  /** 设域管理员：username + 可选 password（用户不存在且缺密码时报错）。 */
  setDomainAdmin: (id: number, username: string, password?: string) =>
    req<void>(`/api/admin/domains/${id}/admin`, json({ method: 'PUT', body: JSON.stringify({ username, password }) })),

  // ---- 空间管理（global_admin 带 domainId / domain_admin 自动本域） ----
  spaces: () => req<{ spaces: Space[] }>(dq('/api/admin/spaces')),
  createSpace: (name: string) => req<{ id: number }>(dq('/api/admin/spaces'), json({ method: 'POST', body: JSON.stringify({ name }) })),
  renameSpace: (id: number, name: string) =>
    req<void>(`/api/admin/spaces/${id}`, json({ method: 'PATCH', body: JSON.stringify({ name }) })),
  deleteSpace: (id: number) => req<void>(`/api/admin/spaces/${id}`, { method: 'DELETE' }),
  spaceMembers: (id: number) => req<{ members: SpaceMember[] }>(`/api/admin/spaces/${id}/members`),
  setSpaceMembers: (id: number, userIds: number[]) =>
    req<void>(`/api/admin/spaces/${id}/members`, json({ method: 'PUT', body: JSON.stringify({ userIds }) })),

  // ---- 空间成员账号（member 账号维护；账号池全局共享，不带域） ----
  users: () => req<{ users: MemberUser[] }>('/api/admin/users'),
  createUser: (username: string, password: string) =>
    req<{ id: number }>('/api/admin/users', json({ method: 'POST', body: JSON.stringify({ username, password }) })),
  deleteUser: (id: number) => req<void>(`/api/admin/users/${id}`, { method: 'DELETE' }),
  resetUserPassword: (id: number, password: string) =>
    req<void>(`/api/admin/users/${id}/password`, json({ method: 'PUT', body: JSON.stringify({ password }) })),

  // ---- 空间内容管理（管理员管理用；URL 以空间归属鉴权，不追加 domainId） ----
  spaceTrainings: (spaceId: number) => req<{ trainings: SpaceTraining[] }>(`/api/space/${spaceId}/trainings`),
  createSpaceTraining: (
    spaceId: number,
    body: { title: string; description?: string; tags?: string[]; maxAttempts?: number; fromRepo?: { kind: 'training' | 'practice'; id: number } },
  ) => req<{ id: number }>(`/api/space/${spaceId}/trainings`, json({ method: 'POST', body: JSON.stringify(body) })),
  getSpaceTraining: (spaceId: number, tid: number) =>
    req<{ training: SpaceTraining; chapters: SpaceChapter[] }>(`/api/space/${spaceId}/trainings/${tid}`),
  updateSpaceTraining: (spaceId: number, tid: number, body: { title: string; description?: string; tags?: string[]; maxAttempts?: number }) =>
    req<void>(`/api/space/${spaceId}/trainings/${tid}`, json({ method: 'PUT', body: JSON.stringify(body) })),
  deleteSpaceTraining: (spaceId: number, tid: number) => req<void>(`/api/space/${spaceId}/trainings/${tid}`, { method: 'DELETE' }),
  createSpaceChapter: (spaceId: number, tid: number, title: string) =>
    req<{ id: number }>(`/api/space/${spaceId}/trainings/${tid}/chapters`, json({ method: 'POST', body: JSON.stringify({ title }) })),
  addSpaceChapterItems: (spaceId: number, chapterId: number, problemIds: number[]) =>
    req<{ itemIds: number[] }>(`/api/space/${spaceId}/chapters/${chapterId}/items`, json({ method: 'POST', body: JSON.stringify({ problemIds }) })),
  spacePractices: (spaceId: number) => req<{ practices: SpacePractice[] }>(`/api/space/${spaceId}/practices`),
  createSpacePractice: (
    spaceId: number,
    body: { title: string; description?: string; tags?: string[]; fromRepo?: { kind: 'training' | 'practice'; id: number } },
  ) => req<{ id: number }>(`/api/space/${spaceId}/practices`, json({ method: 'POST', body: JSON.stringify(body) })),
  getSpacePractice: (spaceId: number, pid: number) =>
    req<{ practice: SpacePractice; items: SpacePracticeItem[] }>(`/api/space/${spaceId}/practices/${pid}`),
  updateSpacePractice: (spaceId: number, pid: number, body: { title: string; description?: string; tags?: string[] }) =>
    req<void>(`/api/space/${spaceId}/practices/${pid}`, json({ method: 'PUT', body: JSON.stringify(body) })),
  deleteSpacePractice: (spaceId: number, pid: number) => req<void>(`/api/space/${spaceId}/practices/${pid}`, { method: 'DELETE' }),
  addSpacePracticeItems: (spaceId: number, pid: number, problemIds: number[]) =>
    req<void>(`/api/space/${spaceId}/practices/${pid}/items`, json({ method: 'POST', body: JSON.stringify({ problemIds }) })),
  deleteSpaceItem: (itemId: number) => req<void>(`/api/space/space-items/${itemId}`, { method: 'DELETE' }),
  spaceQuizzes: (spaceId: number) => req<{ quizzes: SpaceQuiz[] }>(`/api/space/${spaceId}/quizzes`),
  createSpaceQuiz: (spaceId: number, body: { title: string; tags?: string[]; sourceType: string; repoKind?: string; repoId?: number }) =>
    req<{ id: number }>(`/api/space/${spaceId}/quizzes`, json({ method: 'POST', body: JSON.stringify(body) })),
  deleteSpaceQuiz: (spaceId: number, quizId: number) => req<void>(`/api/space/${spaceId}/quizzes/${quizId}`, { method: 'DELETE' }),
}
