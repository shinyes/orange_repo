// 与后端 API 契约一一对应（见 docs/aegis/specs §5）。

export type ProblemType = 'programming' | 'single_choice' | 'true_false'

export interface Solution {
  language: string
  code: string
  markdown: string
}

export interface ProgrammingCase {
  input: string
  output: string
}

export interface ProgrammingBody {
  inputFormat?: string
  outputFormat?: string
  samples?: ProgrammingCase[]
  testCases?: ProgrammingCase[]
}

export interface ChoiceBody {
  options?: string[]
}

export interface ProblemSummary {
  id: number
  type: ProblemType
  title: string
  tags: string[]
  timeLimitMs: number
  memoryLimitMiB: number
  createdAt: string
}

export interface Problem {
  id: number
  type: ProblemType
  title: string
  tags: string[]
  statementMd: string
  // OrangeOJ 原始 JSON 结构，按题型解释
  bodyJson: Record<string, unknown>
  answerJson: Record<string, unknown>
  solutions: Solution[]
  timeLimitMs: number
  memoryLimitMiB: number
  createdAt: string
}

export interface ProblemPayload {
  type: ProblemType
  title: string
  tags: string[]
  statementMd: string
  bodyJson: Record<string, unknown>
  answerJson: Record<string, unknown>
  solutions?: Solution[]
  timeLimitMs?: number
  memoryLimitMiB?: number
}

// 标签树节点：tag 为完整路径（如 数学/几何），label 为最后一段。
// 虚拟父节点由服务端分面接口直接给出计数。
export interface TagNode {
  tag: string
  label: string
  count: number
  children: TagNode[]
}

export interface TagCount {
  tag: string
  count: number
}

export interface Item {
  id: number
  chapterId: number
  problemId: number
  orderNo: number
  problemTitle?: string
  problemType?: string
}

export interface Chapter {
  id: number
  trainingId: number
  title: string
  orderNo: number
  items: Item[]
}

export interface Training {
  id: number
  title: string
  description: string
  tags: string[]
  folderId?: number | null
  problemCount: number
  createdAt: string
}

// 题册目录节点（扁平列表，parentId 为 null 表示根目录）。
export interface BookletDirectory {
  id: number
  name: string
  parentId: number | null
  orderNo: number
}

export interface PracticeItem {
  id: number
  practiceId: number
  problemId: number
  orderNo: number
  problemTitle?: string
  problemType?: string
}

export interface Practice {
  id: number
  title: string
  description: string
  tags: string[]
  folderId?: number | null
  problemCount: number
  createdAt: string
}

export interface ProblemFilterState {
  q: string
  tags: string[]
  type: ProblemType | ''
}

// ---------- 域 / 空间（OJ 重构：仓库页 = 域仓库管理） ----------

export type UserRole = 'global_admin' | 'domain_admin' | 'member'

/** /api/auth/me 返回的当前登录用户。 */
export interface MeUser {
  id: number
  username: string
  role: UserRole
  /** domain_admin 的归属域；member/global_admin 无（或 null）。 */
  domainId?: number | null
}

export interface MeResult {
  authenticated: boolean
  user?: MeUser
}

/** 域管理员账号视图（GET /api/admin/domains/:id/admins）。 */
export interface DomainAdminUser {
  id: number
  username: string
  role?: UserRole
  domainId?: number | null
}

/** 普通成员账号视图（GET /api/admin/users）。 */
export interface MemberUser {
  id: number
  username: string
}

export interface Domain {
  id: number
  name: string
  createdAt: string
}

export interface Space {
  id: number
  domainId: number
  name: string
  createdAt: string
}

export interface SpaceMember {
  userId: number
  username: string
}

// ---------- 空间内容（结构管理） ----------

export interface SpaceTraining {
  id: number
  spaceId: number
  title: string
  description: string
  tags: string[]
  maxAttempts: number
  problemCount: number
}

export interface SpaceChapterItem {
  id: number
  chapterId: number
  problemId: number
  orderNo: number
  problemTitle?: string
  problemType?: string
  problemUuid?: string
}

export interface SpaceChapter {
  id: number
  trainingId: number
  title: string
  orderNo: number
  items: SpaceChapterItem[]
}

export interface SpacePractice {
  id: number
  spaceId: number
  title: string
  description: string
  tags: string[]
  problemCount: number
}

export interface SpacePracticeItem {
  id: number
  practiceId: number
  problemId: number
  orderNo: number
  problemTitle?: string
  problemType?: string
  problemUuid?: string
}

export interface SpaceQuiz {
  id: number
  spaceId: number
  title: string
  tags: string[]
  sourceType: string
  repoKind?: string
  repoId?: number
  problemCount: number
}
