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
  problemCount: number
  createdAt: string
}

export interface PracticeItem {
  id: number
  practiceId: number
  problemId: number
  orderNo: number
  score: number
  problemTitle?: string
  problemType?: string
}

export interface Practice {
  id: number
  title: string
  description: string
  tags: string[]
  problemCount: number
  createdAt: string
}

export interface ProblemFilterState {
  q: string
  tags: string[]
  type: ProblemType | ''
}
