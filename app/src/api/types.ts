// 与后端 API 契约一一对应（合并前：OrangeOJ quizserver；合服后：单进程 internal/app /api 全合并，契约不变）。
// 角色模型：global_admin（系统管理员）/ domain_admin（域管理员，带 domainId）/ member（空间成员=学生）。

export type Role = 'global_admin' | 'domain_admin' | 'member'

export interface User {
  id: number
  username: string
  role: Role
  /** 仅 domain_admin 有意义（其归属域 id） */
  domainId?: number
}

export type PortalProblemType = 'programming' | 'single_choice' | 'true_false'

export type ObjectiveType = 'single_choice' | 'true_false'

// ---------- 空间 ----------

export interface PortalSpace {
  id: number
  domainId: number
  domainName?: string
  name: string
}

export interface SpaceHome {
  trainings: TrainingBrief[]
  practices: PracticeBrief[]
  quizzes: QuizBrief[]
}

// 首页三区卡片与空间内列表共用同一简报结构（home.trainings / space/:id/quizzes 等）。

export interface TrainingBrief {
  id: number
  uuid?: string
  spaceId: number
  title: string
  description: string
  tags: string[]
  maxAttempts: number
  problemCount: number
}

export interface PracticeBrief {
  id: number
  uuid?: string
  spaceId: number
  title: string
  description: string
  tags: string[]
  problemCount: number
}

export interface QuizBrief {
  id: number
  uuid?: string
  spaceId: number
  title: string
  tags: string[]
  sourceType: 'tags' | 'repo'
  repoKind?: string
  repoId?: number
  problemCount: number
}

// ---------- 空间训练（章节化，客观题限次作答） ----------

export interface TrainingItemView {
  /** 条目 id（作答请求按题目 id，此 id 仅列表锚点） */
  id: number
  problemId: number
  orderNo: number
  problemType?: PortalProblemType
  problemUuid?: string
  problemTitle?: string
  solved: boolean
  attempts: number
  /** 达上限未对=红锁；已答对=绿锁 */
  locked: boolean
  /** 已通过/达限时服务端下发的正确答案（回顾标色用；未作答不下发） */
  correctAnswer?: CorrectAnswer
}

export interface ChapterView {
  id: number
  title: string
  items: TrainingItemView[]
}

export interface TrainingDetail {
  training: TrainingBrief & { spaceId: number }
  chapters: ChapterView[]
}

export interface TrainingAnswerResult {
  correct: boolean
  attempts: number
  solved: boolean
  locked: boolean
  correctAnswer: CorrectAnswer
}

export interface CorrectAnswer {
  answerIndex?: number
  answer?: boolean
}

// ---------- 空间练习（整卷交卷） ----------

export interface PracticeItemView {
  id: number
  practiceId?: number
  problemId: number
  orderNo: number
  problemTitle?: string
  problemType?: PortalProblemType
  problemUuid?: string
}

export interface PracticeDetail {
  practice: PracticeBrief & { spaceId: number }
  items: PracticeItemView[]
}

export interface PracticeResultItem {
  problemId: number
  correct: boolean
  type: string
  /** 答错时给出正确项。训练/刷题返回 {answerIndex|answer} 对象；
   *  练习交卷返回原始值（单选=索引 number，判断=boolean） */
  correctAnswer?: CorrectAnswer | number | boolean
}

export interface PracticeSubmitResult {
  submissionId: number
  results: PracticeResultItem[]
  objectiveCorrect: number
  objectiveTotal: number
}

export interface PracticeSubmission {
  id: number
  practiceId: number
  userId: number
  objectiveCorrect: number
  createdAt: string
}

/** 答题卡回看：一次交卷的逐题明细（练习全卷条目） */
export interface PracticeRecordItem {
  problemId: number
  no: number
  title?: string
  type: 'single_choice' | 'true_false' | 'programming'
  /** 客观题该次是否作答（编程题恒 false） */
  answered?: boolean
  /** 该次是否答对（仅作答客观题有意义） */
  correct: boolean
  /** 用户所选（客观题；单选=索引 number，判断=boolean） */
  answer?: number | boolean
  /** 答错时附正确项 */
  correctAnswer?: number | boolean
}

export interface PracticeRecordDetail {
  submissionId: number
  practiceId: number
  createdAt: string
  objectiveCorrect: number
  items: PracticeRecordItem[]
}

// ---------- 空间刷题（单题随机流） ----------

export interface QuizProblem {
  id: number
  type: ObjectiveType
  title: string
  statementMd: string
  bodyJson: { options?: string[] } & Record<string, unknown>
}

export interface QuizProblemResponse {
  problem: QuizProblem | null
  /** true = 本轮覆盖完成（无错题），可重新开始 */
  done: boolean
  /** 范围内没有可刷的客观题 */
  emptyRange?: boolean
  /** 开新批（错题复习优先） */
  newBatch?: boolean
  /** 当前批号 */
  batchNo?: number
  /** 会话中待纠正错题数 */
  wrongCnt?: number
}

export interface QuizAnswerResult {
  correct: boolean
  correctAnswer: CorrectAnswer
  /** 答对且此前未通过（首次通过 +1） */
  firstTime: boolean
  /** 会话中待纠正错题数 */
  wrongCnt?: number
}

// ---------- 排行榜 ----------

export interface RankRow {
  userId: number
  username: string
  solved: number
}

export interface RankView {
  rank: RankRow[]
  domainId: number
}

// =====================================================================
// 管理端（域仓库/域/空间）类型 —— 自原管理端前端类型文件合并
// （合服单进程 /api 契约不变：/api/problems、/api/admin/*、/api/space/* 等）。
// 与上方门户类型去重后统一命名；同义但用途不同的（仓库 Training vs 门户 TrainingBrief、
// 仓库 Space vs 门户 PortalSpace）按语义区分保留。
// =====================================================================

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

// 仓库题目列表行（/api/problems 列表）。
export interface ProblemSummary {
  id: number
  type: ProblemType
  title: string
  tags: string[]
  timeLimitMs: number
  memoryLimitMiB: number
  createdAt: string
}

// 仓库题目详情（/api/problems/:id）。
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
  /** 学生起始代码（python/cpp，可空串=空；编程题编辑器兜底用通用模板） */
  starterPy?: string
  starterCpp?: string
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
  starterPy?: string
  starterCpp?: string
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

// 仓库题册模板条目 / 章节（admin：题册模板管理）。
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
  uuid?: string
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
  uuid?: string
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

// ---------- 域 / 空间（管理 API） ----------

/** 域管理员账号视图（GET /api/admin/domains/:id/admins）。 */
export interface DomainAdminUser {
  id: number
  username: string
  role?: Role
  domainId?: number | null
}

/** 普通成员账号视图（GET /api/admin/users）。 */
export interface MemberUser {
  id: number
  username: string
}

/** 全账号视图（GET /api/admin/all-users：含角色/归属域与域名）。 */
export interface AllUser {
  id: number
  username: string
  role: Role
  domainId?: number | null
  domainName?: string
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

// ---------- 空间内容（管理端结构管理） ----------

export interface SpaceTraining {
  id: number
  uuid?: string
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
  uuid?: string
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
  uuid?: string
  spaceId: number
  title: string
  tags: string[]
  sourceType: string
  repoKind?: string
  repoId?: number
  problemCount: number
}

// ---------- 题目做题（/api/oj/problem/:id 保留） ----------

export type CodeLang = 'cpp' | 'python'

export interface OjProblem {
  id: number
  type: PortalProblemType
  title: string
  statementMd: string
  bodyJson: { options?: string[]; samples?: { input?: string; output?: string }[] } & Record<string, unknown>
  timeLimitMs: number
  memoryLimitMiB: number
  /** 题目级起始模板（学生做题页/训练卡兜底用，可空串）；无值=可空 */
  starterPy?: string
  starterCpp?: string
}

/** 云端草稿内容（GET /api/oj/problem/:id/draft）。 */
export interface OjDraft {
  code: string
  language: string
}

export type Verdict = 'PENDING' | 'OK' | 'AC' | 'WA' | 'CE' | 'RE' | 'TLE' | 'MLE'

export interface CaseDetail {
  caseNo: number
  verdict: Verdict
  input?: string
  output?: string
  expectedOutput?: string
  error?: string
  timeMs: number
  memoryKiB: number
}

export interface Submission {
  id: number
  problemId: number
  questionType: string
  language: string
  sourceCode?: string
  inputData?: string
  submitType: 'run' | 'test' | 'submit' | 'objective'
  status: 'queued' | 'running' | 'done' | 'failed'
  verdict: Verdict
  timeMs: number
  memoryKiB: number
  score: number
  stdout?: string
  stderr?: string
  caseDetails?: CaseDetail[]
  createdAt: string
  finishedAt?: string | null
}

export interface SubmissionPoll {
  submissionId: number
  status: 'queued' | 'running' | 'done' | 'failed'
  isFinal: boolean
  verdict: Verdict
  score: number
  timeMs: number
  memoryKiB: number
  stdout: string
  stderr: string
  caseDetails: CaseDetail[]
  pollAfterMs: number
}

// 客观题提交载荷统一形式（单选=number 索引，判断=boolean）。
export type ObjectiveAnswer = number | boolean
