// 与刷题服务 API 契约一一对应（见 docs/aegis/specs/2026-08-29-quiz-service-design.md §5）。

export type Role = 'admin' | 'student'

export interface User {
  id: number
  username: string
  role: Role
}

export interface CategoryBrief {
  id: number
  name: string
  orderNo: number
  questionCount: number
}

export interface SubjectBrief {
  id: number
  name: string
  orderNo: number
  categories: CategoryBrief[]
}

export type ProblemType = 'single_choice' | 'true_false'

export interface QuizProblem {
  id: number
  type: ProblemType
  title: string
  statementMd: string
  bodyJson: Record<string, unknown>
  hasExplanation: boolean
}

export interface Round {
  categoryId: number
  total: number
  problems: QuizProblem[]
}

export interface CorrectAnswer {
  answerIndex?: number
  answer?: boolean
}

export interface SubmitResult {
  correct: boolean
  correctAnswer: CorrectAnswer
  hasExplanation: boolean
  explanation: string
}

export interface WrongGroup {
  categoryId: number
  categoryName: string
  subjectName: string
  count: number
}

export interface WrongSummary {
  total: number
  groups: WrongGroup[]
}

export interface WrongRoundProblem extends QuizProblem {
  categoryId: number
}

export interface WrongRound {
  scope: 'all' | 'category'
  categoryId?: number | null
  problems: WrongRoundProblem[]
}

// ---- 管理员 ----

export interface AdminCategory {
  id: number
  subjectId: number
  name: string
  orderNo: number
  tags: string[]
  types: ProblemType[]
  questionCount: number
}

export interface AdminSubject {
  id: number
  name: string
  orderNo: number
  categories: AdminCategory[]
}

export interface AdminStudent {
  id: number
  username: string
  createdAt: string
  wrongCount: number
}

export interface Settings {
  roundSize: number
}

// ---- OrangeOJ：布置与做题 ----

export interface OjAssigned {
  trainings: OjTrainingBrief[]
  practices: OjPracticeBrief[]
}

export interface OjTrainingBrief {
  id: number
  title: string
  description: string
  tags: string[]
  problemCount: number
  accepted: number
  chapterCount: number
}

export interface OjPracticeBrief {
  id: number
  title: string
  description: string
  tags: string[]
  problemCount: number
  accepted: number
}

export interface OjItem {
  problemId: number
  orderNo: number
  title: string
  type: 'programming' | 'single_choice' | 'true_false'
  completed: boolean
}

export interface OjChapter {
  id: number
  title: string
  orderNo: number
  items: OjItem[]
}

export interface OjTrainingDetail {
  id: number
  title: string
  description: string
  tags: string[]
  chapters: OjChapter[]
  accepted: number
  total: number
  stale?: boolean
}

export interface OjPracticeDetail {
  id: number
  title: string
  description: string
  tags: string[]
  items: OjItem[]
  accepted: number
  total: number
  stale?: boolean
}

export type CodeLang = 'cpp' | 'python'

export interface OjProblem {
  id: number
  type: 'programming' | 'single_choice' | 'true_false'
  title: string
  statementMd: string
  bodyJson: Record<string, unknown>
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

// ---- 管理端：布置 ----

export interface RepoTraining {
  id: number
  title: string
  description: string
  tags: string[]
  problemCount: number
  chapterCount: number
}

export interface RepoPractice {
  id: number
  title: string
  description: string
  tags: string[]
  items: number[]
  problemCount: number
}

export interface AdminAssignment {
  id: number
  kind: 'training' | 'practice'
  repoId: number
  title: string
  description: string
  tags: string[]
  published: boolean
  assignedAll: boolean
  problemCount: number
  studentCount: number
  createdAt: string
}

export interface AssignmentStudents {
  assignedAll: boolean
  students: { userId: number; username: string }[]
}

export interface AssignmentStatsProblem {
  problemId: number
  title: string
  type: string
  accepted: number
  submissions: number
}

export interface AssignmentStats {
  title: string
  kind: 'training' | 'practice'
  totalStudents: number
  problems: AssignmentStatsProblem[]
}