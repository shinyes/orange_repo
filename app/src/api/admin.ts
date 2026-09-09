// 管理端 API 命名空间（src/api/admin.ts）：自原管理端前端 api 层合并（第二阶段）。
// 覆盖：题目/标签/标签排序/图片/题册目录/训练/练习/域/空间/成员账号/空间内容/导入导出/全库备份。
// 域上下文（currentDomainId）由 admin 域上下文（pages/admin/domain-context.tsx）驱动：
//   - domain_admin 登录即锁定归属域；
//   - global_admin 从 localStorage 'OrangeOJ:domain' 恢复/切换；
// 所有带域数据的请求 URL 经 dq() 附加 domainId=；不带域的（题目详情/训练详情/域管理/账号池）不附加。
import { req, json } from './client'
import type {
  AllUser,
  BookletDirectory,
  Chapter,
  Domain,
  DomainAdminUser,
  ImportTaskView,
  MemberUser,
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

/** 当前管理域（null=全局/未选域）。影响此后全部带域请求的 domainId 附加。 */
let currentDomainId: number | null = null

/** 设置当前管理域（null=全局/未选域）。 */
export function setDomain(domainId: number | null) {
  currentDomainId = domainId
}

/** 获取当前管理域（供调用方在需要显式传递时读取）。 */
export function getDomain(): number | null {
  return currentDomainId
}

/** 为带域数据的路径附加当前域 query：未选域时原样返回，避免漏 domainId 的裸调用。 */
function dq(path: string): string {
  if (currentDomainId == null) return path
  const sep = path.includes('?') ? '&' : '?'
  return `${path}${sep}domainId=${currentDomainId}`
}

/** 由过滤状态构造 query string（标签逐个 append——标签文本可含逗号，多值参数不会误拆）。 */
export function filterQuery(f: ProblemFilterState, extra?: Record<string, string>): string {
  const p = new URLSearchParams()
  if (f.q) p.set('q', f.q)
  for (const t of f.tags) p.append('tags', t)
  if (f.type) p.set('type', f.type)
  for (const [k, v] of Object.entries(extra ?? {})) p.set(k, v)
  const s = p.toString()
  return s ? `?${s}` : ''
}

export const adminApi = {
  // ---- 题目 ----
  problems: (f: ProblemFilterState) => req<{ problems: ProblemSummary[] }>(dq(`/api/problems${filterQuery(f)}`)),
  createProblem: (payload: ProblemPayload) =>
    req<{ problem: Problem }>(dq('/api/problems'), json({ method: 'POST', body: JSON.stringify(payload) })),
  getProblem: (id: number) => req<{ problem: Problem }>(`/api/problems/${id}`),
  updateProblem: (id: number, payload: ProblemPayload) =>
    req<{ problem: Problem }>(`/api/problems/${id}`, json({ method: 'PUT', body: JSON.stringify(payload) })),
  deleteProblem: (id: number) => req<void>(`/api/problems/${id}`, { method: 'DELETE' }),

  // ---- 标签（动态 facet 计数随过滤上下文联动；子树整体重命名/删除） ----
  tags: (f?: ProblemFilterState) => req<{ tags: TagCount[]; total: number }>(dq(`/api/tags${f ? filterQuery(f) : ''}`)),
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
  // 全库备份/迁移（域级：导出该域 / 导入到该域，domainId 为空时后端回退默认域）
  exportBackupUrl: (domainId?: number | null) =>
    `/api/export/backup${domainId != null ? `?domainId=${domainId}` : ''}`,
  /** 全量导入（异步）：上传后立即返回 taskId，用 importTask 轮询进度 */
  startImportBackup: async (file: File, domainId?: number | null): Promise<{ taskId: string }> => {
    const body = new FormData()
    body.append('zip', file)
    return req(`/api/import/backup${domainId != null ? `?domainId=${domainId}` : ''}`, { method: 'POST', body })
  },
  /** 导入任务进度轮询 */
  importTask: (taskId: string): Promise<ImportTaskView> =>
    req(`/api/import/backup/task/${taskId}`),
  /** 兼容旧调用：上传后轮询至完成（供仍用同步语义的调用方） */
  importBackup: async (file: File, domainId?: number | null): Promise<{ imported: number; trainings: number; practices: number }> => {
    const started = await adminApi.startImportBackup(file, domainId)
    for (;;) {
      const t = await adminApi.importTask(started.taskId)
      if (!t.done) {
        await new Promise((r) => setTimeout(r, 600))
        continue
      }
      if (!t.ok) throw new Error(t.error || '导入失败')
      return t.result as { imported: number; trainings: number; practices: number }
    }
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

  // ---- 训练（仓库题册模板） ----
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
  ) => req<{ chapters: Chapter[] }>(`/api/trainings/${id}/layout`, json({ method: 'PUT', body: JSON.stringify(payload) })),
  deleteItem: (id: number) => req<void>(`/api/items/${id}`, { method: 'DELETE' }),

  // ---- 练习（仓库题册模板） ----
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
    req<{ id: number }>('/api/admin/domains', json({ method: 'POST', body: JSON.stringify({ name, adminUsername, adminPassword }) })),
  /** 部分更新域设置（PATCH /api/admin/domains/:id）：仅请求中出现的字段被修改。 */
  updateDomainSettings: (id: number, payload: { name?: string; leaderboardPublic?: boolean }) =>
    req<void>(`/api/admin/domains/${id}`, json({ method: 'PATCH', body: JSON.stringify(payload) })),
  renameDomain: (id: number, name: string) => adminApi.updateDomainSettings(id, { name }),
  /** 删除域。域内有题目时后端返回 409；届时调用方应提示并带 deleteProblems=true 重试。 */
  deleteDomain: (id: number, deleteProblems = false) =>
    req<void>(`/api/admin/domains/${id}${deleteProblems ? '?deleteProblems=true' : ''}`, { method: 'DELETE' }),
  domainAdmins: (id: number) => req<{ admins: DomainAdminUser[] }>(`/api/admin/domains/${id}/admins`),
  /** 设域管理员：username + 可选 password（用户不存在且缺密码时报错）。 */
  setDomainAdmin: (id: number, username: string, password?: string) =>
    req<void>(`/api/admin/domains/${id}/admin`, json({ method: 'PUT', body: JSON.stringify({ username, password }) })),
  /** 移除域管理员（账号保留为普通成员）。 */
  removeDomainAdmin: (id: number, uid: number) => req<void>(`/api/admin/domains/${id}/admins/${uid}`, { method: 'DELETE' }),

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
  /** 全账号列表（仅系统管理员：含角色/归属域/域名） */
  allUsers: () => req<{ users: AllUser[] }>('/api/admin/all-users'),
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
    body: { title: string; description?: string; tags?: string[]; maxAttempts?: number; isPublic?: boolean; fromRepo?: { kind: 'training' | 'practice'; id: number } },
  ) => req<{ id: number }>(`/api/space/${spaceId}/trainings`, json({ method: 'POST', body: JSON.stringify(body) })),
  getSpaceTraining: (spaceId: number, tid: number) =>
    req<{ training: SpaceTraining; chapters: SpaceChapter[] }>(`/api/space/${spaceId}/trainings/${tid}`),
  /** 更新训练元信息（部分更新：仅发送的字段被修改；title 提供时不可为空） */
  updateSpaceTraining: (spaceId: number, tid: number, body: { title?: string; description?: string; tags?: string[]; maxAttempts?: number; isPublic?: boolean }) =>
    req<void>(`/api/space/${spaceId}/trainings/${tid}`, json({ method: 'PUT', body: JSON.stringify(body) })),
  deleteSpaceTraining: (spaceId: number, tid: number) => req<void>(`/api/space/${spaceId}/trainings/${tid}`, { method: 'DELETE' }),
  /** 可见成员名单（训练/练习/刷题；kind=training|practice|quiz） */
  visibleUsers: (kind: 'training' | 'practice' | 'quiz', spaceId: number, itemId: number) =>
    req<{ userIds: number[] }>(`/api/space/${spaceId}/${kind === 'training' ? 'trainings' : kind === 'practice' ? 'practices' : 'quizzes'}/${itemId}/visible`),
  /** 覆盖式设置可见成员（空=无成员可见） */
  setVisibleUsers: (kind: 'training' | 'practice' | 'quiz', spaceId: number, itemId: number, userIds: number[]) =>
    req<void>(`/api/space/${spaceId}/${kind === 'training' ? 'trainings' : kind === 'practice' ? 'practices' : 'quizzes'}/${itemId}/visible`, json({ method: 'PUT', body: JSON.stringify({ userIds }) })),
  createSpaceChapter: (spaceId: number, tid: number, title: string) =>
    req<{ id: number }>(`/api/space/${spaceId}/trainings/${tid}/chapters`, json({ method: 'POST', body: JSON.stringify({ title }) })),
  addSpaceChapterItems: (spaceId: number, chapterId: number, problemIds: number[]) =>
    req<{ itemIds: number[] }>(`/api/space/${spaceId}/chapters/${chapterId}/items`, json({ method: 'POST', body: JSON.stringify({ problemIds }) })),
  /** 章节重命名 */
  renameSpaceChapter: (chapterId: number, title: string) =>
    req<void>(`/api/space/chapters/${chapterId}`, json({ method: 'PUT', body: JSON.stringify({ title }) })),
  /** 删除章节（级联条目） */
  deleteSpaceChapter: (chapterId: number) => req<void>(`/api/space/chapters/${chapterId}`, { method: 'DELETE' }),
  /** 章节排序（chapterIds 全量顺序；按训练归属校验） */
  reorderSpaceChapters: (trainingId: number, chapterIds: number[]) =>
    req<void>(`/api/space/trainings/${trainingId}/chapters/order`, json({ method: 'PUT', body: JSON.stringify({ chapterIds }) })),
  /** 章节内题目排序（itemIds 全量顺序） */
  reorderSpaceChapterItems: (chapterId: number, itemIds: number[]) =>
    req<void>(`/api/space/chapters/${chapterId}/items/order`, json({ method: 'PUT', body: JSON.stringify({ itemIds }) })),
  spacePractices: (spaceId: number) => req<{ practices: SpacePractice[] }>(`/api/space/${spaceId}/practices`),
  createSpacePractice: (
    spaceId: number,
    body: { title: string; description?: string; tags?: string[]; isPublic?: boolean; fromRepo?: { kind: 'training' | 'practice'; id: number } },
  ) => req<{ id: number }>(`/api/space/${spaceId}/practices`, json({ method: 'POST', body: JSON.stringify(body) })),
  getSpacePractice: (spaceId: number, pid: number) =>
    req<{ practice: SpacePractice; items: SpacePracticeItem[] }>(`/api/space/${spaceId}/practices/${pid}`),
  updateSpacePractice: (spaceId: number, pid: number, body: { title: string; description?: string; tags?: string[]; isPublic?: boolean }) =>
    req<void>(`/api/space/${spaceId}/practices/${pid}`, json({ method: 'PUT', body: JSON.stringify(body) })),
  deleteSpacePractice: (spaceId: number, pid: number) => req<void>(`/api/space/${spaceId}/practices/${pid}`, { method: 'DELETE' }),
  addSpacePracticeItems: (spaceId: number, pid: number, problemIds: number[]) =>
    req<void>(`/api/space/${spaceId}/practices/${pid}/items`, json({ method: 'POST', body: JSON.stringify({ problemIds }) })),
  /** 空间练习条目全量排序（itemIds 须覆盖全部条目） */
  reorderSpacePracticeItems: (pid: number, itemIds: number[]) =>
    req<void>(`/api/space/practices/${pid}/items/order`, json({ method: 'PUT', body: JSON.stringify({ itemIds }) })),
  deleteSpaceItem: (itemId: number) => req<void>(`/api/space/space-items/${itemId}`, { method: 'DELETE' }),
  spaceQuizzes: (spaceId: number) => req<{ quizzes: SpaceQuiz[] }>(`/api/space/${spaceId}/quizzes`),
  createSpaceQuiz: (spaceId: number, body: { title: string; tags?: string[]; sourceType: string; repoKind?: string; repoId?: number; roundSize?: number; isPublic?: boolean }) =>
    req<{ id: number }>(`/api/space/${spaceId}/quizzes`, json({ method: 'POST', body: JSON.stringify(body) })),
  updateSpaceQuiz: (spaceId: number, quizId: number, body: { title: string; tags?: string[]; roundSize?: number; isPublic?: boolean }) =>
    req<void>(`/api/space/${spaceId}/quizzes/${quizId}`, json({ method: 'PUT', body: JSON.stringify(body) })),
  deleteSpaceQuiz: (spaceId: number, quizId: number) => req<void>(`/api/space/${spaceId}/quizzes/${quizId}`, { method: 'DELETE' }),
}
