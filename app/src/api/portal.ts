// 门户 API 命名空间（src/api/portal.ts）：
// 对应后端合并后单进程 internal/app 的 /api/portal/*、/api/oj/problem/:id 保留路由。
// 方法名沿用迁移前门户前端 api 的 portal* / oj* 命名，功能等价、零调用点改动；
// 后续若需收敛为 portalApi.xxx 短名可整体重构（与 admin 并入时统一）。
// 类型契约见 ./types（与后端合服前的 OrangeOJ quizserver 一致）。
import { req, json } from './client'
import type {
  CodeLang,
  ObjectiveAnswer,
  OjDraft,
  OjProblem,
  PortalSpace,
  PracticeDetail,
  PracticeRecordDetail,
  PracticeSubmission,
  PracticeSubmitResult,
  QuizAnswerResult,
  QuizBrief,
  QuizProblemResponse,
  RankView,
  SpaceHome,
  Submission,
  SubmissionPoll,
  TrainingAnswerResult,
  TrainingDetail,
  Verdict,
} from './types'

export const portalApi = {
  // ---- 门户：空间 ----
  portalSpaces: () => req<{ spaces: PortalSpace[] }>('/api/portal/spaces'),
  portalSpaceHome: (spaceId: number | string) => req<SpaceHome>(`/api/portal/space/${spaceId}/home`),
  portalRank: (domainId: number | string) => req<RankView>(`/api/portal/rank?domainId=${domainId}`),

  // ---- 门户：空间训练 ----
  portalTraining: (spaceId: number | string, trainingId: number | string) =>
    req<TrainingDetail>(`/api/portal/space/${spaceId}/training/${trainingId}`),
  portalTrainingAnswer: (spaceId: number | string, trainingId: number | string, problemId: number, answer: ObjectiveAnswer) =>
    req<TrainingAnswerResult>(
      `/api/portal/space/${spaceId}/training/${trainingId}/answer`,
      json({ method: 'POST', body: JSON.stringify({ problemId, answer }) }),
    ),

  // ---- 门户：空间练习 ----
  portalPractice: (spaceId: number | string, practiceId: number | string) =>
    req<PracticeDetail>(`/api/portal/space/${spaceId}/practice/${practiceId}`),
  portalPracticeSubmit: (
    spaceId: number | string,
    practiceId: number | string,
    answers: { problemId: number; answer: ObjectiveAnswer; uuid?: string }[],
  ) =>
    req<PracticeSubmitResult>(
      `/api/portal/space/${spaceId}/practice/${practiceId}/submit`,
      json({ method: 'POST', body: JSON.stringify({ answers }) }),
    ),
  portalPracticeSubmissions: (spaceId: number | string, practiceId: number | string) =>
    req<{ submissions: PracticeSubmission[] }>(`/api/portal/space/${spaceId}/practice/${practiceId}/submissions`),
  portalPracticeSubmissionDetail: (spaceId: number | string, practiceId: number | string, submissionId: number | string) =>
    req<PracticeRecordDetail>(`/api/portal/space/${spaceId}/practice/${practiceId}/submissions/${submissionId}`),

  // ---- 门户：空间刷题 ----
  portalSpaceQuizzes: (spaceId: number | string) => req<{ quizzes: QuizBrief[] }>(`/api/portal/space/${spaceId}/quizzes`),
  portalQuizProblem: (quizId: number | string) => req<QuizProblemResponse>(`/api/portal/quiz/${quizId}/problem`),
  portalQuizAnswer: (quizId: number | string, problemId: number, answer: ObjectiveAnswer) =>
    req<QuizAnswerResult>(
      `/api/portal/quiz/${quizId}/answer`,
      json({ method: 'POST', body: JSON.stringify({ problemId, answer }) }),
    ),

  // ---- OrangeOJ：题目做题（/api/oj/problem/:id 保留——做题页/空间内跳转复用） ----
  ojProblem: (id: number) => req<OjProblem>(`/api/oj/problem/${id}`),
  ojDraft: (id: number, lang: CodeLang) => req<OjDraft>(`/api/oj/problem/${id}/draft?lang=${lang}`),
  ojSaveDraft: (id: number, lang: CodeLang, code: string) =>
    req<void>(`/api/oj/problem/${id}/draft`, json({ method: 'PUT', body: JSON.stringify({ language: lang, code }) })),
  ojRun: (id: number, language: CodeLang, sourceCode: string, inputData: string) =>
    req<{ submissionId: number; status: string }>(`/api/oj/problem/${id}/run`, json({ method: 'POST', body: JSON.stringify({ language, sourceCode, inputData }) })),
  ojTest: (id: number, language: CodeLang, sourceCode: string) =>
    req<{ submissionId: number; status: string }>(`/api/oj/problem/${id}/test`, json({ method: 'POST', body: JSON.stringify({ language, sourceCode }) })),
  /** 提交：trainingId/practiceId 可选——省略=全局（做题页）；>0=训练/练习内提交（服务端落对应上下文列）。 */
  ojSubmit: (id: number, language: CodeLang, sourceCode: string, trainingId?: number, practiceId?: number) =>
    req<{ submissionId: number; status: string }>(`/api/oj/problem/${id}/submit`, json({
      method: 'POST',
      body: JSON.stringify({ language, sourceCode, ...(trainingId ? { trainingId } : {}), ...(practiceId ? { practiceId } : {}) }),
    })),
  ojObjectiveSubmit: (id: number, answer: ObjectiveAnswer) =>
    req<{ submissionId: number; verdict: Verdict; score: number; correct: boolean; correctAnswer: { answerIndex?: number; answer?: boolean } }>(
      `/api/oj/problem/${id}/objective-submit`, json({ method: 'POST', body: JSON.stringify({ answer }) })),
  ojPoll: (submissionId: number, trainingId?: number) =>
    req<SubmissionPoll>(`/api/oj/submission/${submissionId}/poll${trainingId ? `?trainingId=${trainingId}` : ''}`),
  /** 提交历史：trainingId/practiceId 可选——省略=全局；>0=仅对应上下文内提交。 */
  ojSubmissions: (id: number, trainingId?: number, practiceId?: number) =>
    req<{ submissions: Submission[] }>(`/api/oj/problem/${id}/submissions${
      trainingId ? `?trainingId=${trainingId}` : practiceId ? `?practiceId=${practiceId}` : ''
    }`),
}
