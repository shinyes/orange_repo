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