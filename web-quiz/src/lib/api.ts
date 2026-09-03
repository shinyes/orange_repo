import type {
  AdminStudent,
  AdminSubject,
  AdminAssignment,
  AssignmentStats,
  AssignmentStudents,
  CodeLang,
  OjAssigned,
  OjPracticeDetail,
  OjProblem,
  OjTrainingDetail,
  ProblemType,
  RepoPractice,
  RepoTraining,
  Round,
  Settings,
  SubjectBrief,
  Submission,
  SubmissionPoll,
  SubmitResult,
  User,
  Verdict,
  WrongRound,
  WrongSummary,
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
      window.dispatchEvent(new Event('quiz:unauthorized'))
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

export const api = {
  // ---- 认证 ----
  me: () => req<{ authenticated: boolean; user?: User }>('/api/auth/me'),
  login: (username: string, password: string) =>
    req<void>('/api/auth/login', json({ method: 'POST', body: JSON.stringify({ username, password }) })),
  logout: () => req<void>('/api/auth/logout', { method: 'POST' }),
  changePassword: (oldPassword: string, newPassword: string) =>
    req<void>('/api/auth/password', json({ method: 'PUT', body: JSON.stringify({ oldPassword, newPassword }) })),

  // ---- 学生端 ----
  subjects: () => req<{ subjects: SubjectBrief[] }>('/api/quiz/subjects'),
  startRound: (categoryId: number) =>
    req<Round>('/api/quiz/round', json({ method: 'POST', body: JSON.stringify({ categoryId }) })),
  submit: (payload: { problemId: number; categoryId: number; optionIndex?: number; answer?: boolean }) =>
    req<SubmitResult>('/api/quiz/submit', json({ method: 'POST', body: JSON.stringify(payload) })),
  wrongSummary: () => req<WrongSummary>('/api/quiz/wrong-summary'),
  startWrongRound: (categoryId?: number) =>
    req<WrongRound>('/api/quiz/wrong-round', json({ method: 'POST', body: JSON.stringify({ categoryId: categoryId ?? null }) })),

  // ---- 管理员：科目与分类 ----
  adminSubjects: () => req<{ subjects: AdminSubject[] }>('/api/admin/subjects'),
  createSubject: (name: string) =>
    req<{ id: number }>('/api/admin/subjects', json({ method: 'POST', body: JSON.stringify({ name }) })),
  renameSubject: (id: number, name: string) =>
    req<void>(`/api/admin/subjects/${id}`, json({ method: 'PATCH', body: JSON.stringify({ name }) })),
  deleteSubject: (id: number) => req<void>(`/api/admin/subjects/${id}`, { method: 'DELETE' }),
  setSubjectOrder: (ids: number[]) =>
    req<void>('/api/admin/subjects/order', json({ method: 'PUT', body: JSON.stringify({ ids }) })),
  createCategory: (payload: { subjectId: number; name: string; orderNo?: number; tags: string[]; types: ProblemType[] }) =>
    req<{ id: number }>('/api/admin/categories', json({ method: 'POST', body: JSON.stringify(payload) })),
  updateCategory: (
    id: number,
    payload: { name?: string; orderNo?: number; tags?: string[]; types?: ProblemType[] },
  ) => req<void>(`/api/admin/categories/${id}`, json({ method: 'PATCH', body: JSON.stringify(payload) })),
  deleteCategory: (id: number) => req<void>(`/api/admin/categories/${id}`, { method: 'DELETE' }),
  setCategoryOrder: (subjectId: number, ids: number[]) =>
    req<void>(`/api/admin/subjects/${subjectId}/categories/order`, json({ method: 'PUT', body: JSON.stringify({ ids }) })),
  problemsCount: (tags: string[], types: ProblemType[]) => {
    const p = new URLSearchParams()
    // 标签逐个 append（可含逗号）；types 维持逗号分隔
    for (const t of tags) p.append('tags', t)
    if (types.length) p.set('types', types.join(','))
    const s = p.toString()
    return req<{ count: number }>(`/api/admin/problems-count${s ? `?${s}` : ''}`)
  },

  // ---- 管理员：学生与设置 ----
  students: () => req<{ students: AdminStudent[] }>('/api/admin/students'),
  createStudent: (username: string, password: string) =>
    req<{ id: number }>('/api/admin/students', json({ method: 'POST', body: JSON.stringify({ username, password }) })),
  resetStudentPassword: (id: number, password: string) =>
    req<void>(`/api/admin/students/${id}/password`, json({ method: 'PUT', body: JSON.stringify({ password }) })),
  deleteStudent: (id: number) => req<void>(`/api/admin/students/${id}`, { method: 'DELETE' }),
  admins: () => req<{ admins: User[] }>('/api/admin/admins'),
  resetAdminPassword: (id: number, password: string) =>
    req<void>(`/api/admin/admins/${id}/password`, json({ method: 'PUT', body: JSON.stringify({ password }) })),
  settings: () => req<Settings>('/api/admin/settings'),
  putSettings: (roundSize: number) =>
    req<void>('/api/admin/settings', json({ method: 'PUT', body: JSON.stringify({ roundSize }) })),

  // ---- OrangeOJ：学生端（训练/练习/做题） ----
  ojAssigned: () => req<OjAssigned>('/api/oj/assigned'),
  ojTraining: (id: number) => req<OjTrainingDetail>(`/api/oj/training/${id}`),
  ojPractice: (id: number) => req<OjPracticeDetail>(`/api/oj/practice/${id}`),
  ojProblem: (id: number) => req<OjProblem>(`/api/oj/problem/${id}`),
  ojRun: (id: number, language: CodeLang, sourceCode: string, inputData: string) =>
    req<{ submissionId: number; status: string }>(`/api/oj/problem/${id}/run`, json({ method: 'POST', body: JSON.stringify({ language, sourceCode, inputData }) })),
  ojTest: (id: number, language: CodeLang, sourceCode: string) =>
    req<{ submissionId: number; status: string }>(`/api/oj/problem/${id}/test`, json({ method: 'POST', body: JSON.stringify({ language, sourceCode }) })),
  ojSubmit: (id: number, language: CodeLang, sourceCode: string) =>
    req<{ submissionId: number; status: string }>(`/api/oj/problem/${id}/submit`, json({ method: 'POST', body: JSON.stringify({ language, sourceCode }) })),
  ojObjectiveSubmit: (id: number, answer: number | boolean) =>
    req<{ submissionId: number; verdict: Verdict; score: number; correct: boolean; correctAnswer: { answerIndex?: number; answer?: boolean } }>(
      `/api/oj/problem/${id}/objective-submit`, json({ method: 'POST', body: JSON.stringify({ answer }) })),
  ojPoll: (submissionId: number) => req<SubmissionPoll>(`/api/oj/submission/${submissionId}/poll`),
  ojSubmissions: (id: number) => req<{ submissions: Submission[] }>(`/api/oj/problem/${id}/submissions`),

  // ---- OrangeOJ：管理端布置 ----
  repoTrainings: () => req<{ trainings: RepoTraining[] }>('/api/admin/repo-trainings'),
  repoPractices: () => req<{ practices: RepoPractice[] }>('/api/admin/repo-practices'),
  repoTraining: (id: number) => req<OjTrainingDetail & { id: number }>(`/api/admin/repo-trainings/${id}`),
  repoPractice: (id: number) => req<RepoPractice>(`/api/admin/repo-practices/${id}`),
  adminAssignments: (kind?: 'training' | 'practice') =>
    req<{ assignments: AdminAssignment[] }>(`/api/admin/assignments${kind ? `?kind=${kind}` : ''}`),
  createAssignment: (payload: { kind: string; repoId: number; title?: string; description?: string; assignedAll: boolean; published?: boolean; studentIds: number[] }) =>
    req<{ id: number }>('/api/admin/assignments', json({ method: 'POST', body: JSON.stringify(payload) })),
  updateAssignment: (id: number, payload: { title?: string; description?: string; published?: boolean }) =>
    req<void>(`/api/admin/assignments/${id}`, json({ method: 'PATCH', body: JSON.stringify(payload) })),
  setAssignmentStudents: (id: number, payload: { assignedAll: boolean; studentIds: number[] }) =>
    req<void>(`/api/admin/assignments/${id}/students`, json({ method: 'PUT', body: JSON.stringify(payload) })),
  deleteAssignment: (id: number) => req<void>(`/api/admin/assignments/${id}`, { method: 'DELETE' }),
  assignmentStudents: (id: number) => req<AssignmentStudents>(`/api/admin/assignments/${id}/students`),
  assignmentStats: (id: number) => req<AssignmentStats>(`/api/admin/assignments/${id}/stats`),
}