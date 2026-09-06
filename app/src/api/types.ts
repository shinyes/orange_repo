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
  spaceId: number
  title: string
  description: string
  tags: string[]
  maxAttempts: number
  problemCount: number
}

export interface PracticeBrief {
  id: number
  spaceId: number
  title: string
  description: string
  tags: string[]
  problemCount: number
}

export interface QuizBrief {
  id: number
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
  /** true = 本组题已全部通过，无题可刷 */
  done: boolean
}

export interface QuizAnswerResult {
  correct: boolean
  correctAnswer: CorrectAnswer
  /** 答对且此前未通过（首次通过 +1） */
  firstTime: boolean
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
