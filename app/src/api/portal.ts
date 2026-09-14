// 门户 API 命名空间（src/api/portal.ts）：
// 对应后端合并后单进程 internal/app 的 /api/portal/*、/api/oj/problem/:id 保留路由。
// 方法名沿用迁移前门户前端 api 的 portal* / oj* 命名，功能等价、零调用点改动；
// 后续若需收敛为 portalApi.xxx 短名可整体重构（与 admin 并入时统一）。
// 类型契约见 ./types（与后端合服前的 OrangeOJ quizserver 一致）。
import { req, json } from './client'
import type {
  CodeLang,
  GameRankView,
  ScratchFolder,
  ScratchProject,
  ObjectiveAnswer,
  OjDraft,
  OjProblem,
  PortalSpace,
  PracticeDetail,
  PracticeDraftView,
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
  WrongBookView,
} from './types'

export const portalApi = {
  // ---- 门户：空间 ----
  portalSpaces: () => req<{ spaces: PortalSpace[] }>('/api/portal/spaces'),
  portalSpaceHome: (spaceId: number | string) => req<SpaceHome>(`/api/portal/space/${spaceId}/home`),
  portalRank: (domainId: number | string) => req<RankView>(`/api/portal/rank?domainId=${domainId}`),

  // ---- 休息时间小游戏（榜单 game 维度，新增游戏无需改接口） ----
  /** 提交一局成绩（服务端只保留最高分） */
  gameScoreSubmit: (game: string, score: number, spaceId?: number) =>
    req<{ bestScore: number; isNewBest: boolean; score: number; domainId: number; rankDomain: number; rankAll: number }>(
      `/api/portal/game/${game}/score`,
      json({ method: 'POST', body: JSON.stringify({ score, spaceId: spaceId ?? 0 }) }),
    ),
  /** 榜单：scope=domain（本域榜单）| all（全域榜单） */
  gameRank: (game: string, scope: 'domain' | 'all', spaceId?: number) =>
    req<GameRankView>(
      `/api/portal/game/${game}/rank?scope=${scope}${spaceId ? `&spaceId=${spaceId}` : ''}`,
    ),

  // ---- 运行时配置（公开）----
  /** Scratch 编辑器地址：同源前缀（主站反代，如 /scratch-app）或子域绝对地址；空=未部署 */
  appConfig: () => req<{ scratchUrl: string; scratchProtocol: number }>('/api/config'),

  // ---- 书包（Scratch 工程库；服务端存 .sb3，含素材） ----
  scratchFolders: () =>
    req<{ folders: ScratchFolder[]; usage: { usedBytes: number; quotaBytes: number; maxProjectBytes: number; maxFolders: number } }>(
      '/api/portal/scratch/folders',
    ),
  createScratchFolder: (name: string, parentId?: number | null) =>
    req<{ id: number }>('/api/portal/scratch/folders', json({ method: 'POST', body: JSON.stringify({ name, parentId: parentId ?? null }) })),
  updateScratchFolder: (id: number, patch: { name?: string; parentId?: number }) =>
    req<void>(`/api/portal/scratch/folders/${id}`, json({ method: 'PATCH', body: JSON.stringify(patch) })),
  deleteScratchFolder: (id: number) => req<void>(`/api/portal/scratch/folders/${id}`, { method: 'DELETE' }),
  /** folderId 省略=全部；0=根目录 */
  scratchProjects: (folderId?: number) =>
    req<{ projects: ScratchProject[] }>(
      `/api/portal/scratch/projects${folderId === undefined ? '' : `?folderId=${folderId}`}`,
    ),
  /** 上传工程（.sb3 原始字节；服务端校验 PK 魔数与配额） */
  uploadScratchProject: (name: string, bytes: ArrayBuffer, folderId?: number | null) =>
    req<{ id: number; uuid: string; size: number }>(
      `/api/portal/scratch/projects?name=${encodeURIComponent(name)}${folderId ? `&folderId=${folderId}` : ''}`,
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/octet-stream' },
        body: bytes,
      },
    ),
  /** 下载工程原始字节（用于载入编辑器 / 另存到本机） */
  scratchProjectBytes: async (id: number): Promise<ArrayBuffer> => {
    const res = await fetch(`/api/portal/scratch/projects/${id}/raw`, { credentials: 'include' })
    if (!res.ok) throw new Error(`读取工程失败（${res.status}）`)
    return res.arrayBuffer()
  },
  // 覆盖保存（实时暂存用）：同一作品写回内容，不新增记录
  updateScratchProjectContent: (id: number, bytes: ArrayBuffer) =>
    req<{ id: number; size: number }>(`/api/portal/scratch/projects/${id}/content`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/octet-stream' },
      body: bytes,
    }),  updateScratchProject: (id: number, patch: { name?: string; folderId?: number }) =>
    req<void>(`/api/portal/scratch/projects/${id}`, json({ method: 'PATCH', body: JSON.stringify(patch) })),
  deleteScratchProject: (id: number) => req<void>(`/api/portal/scratch/projects/${id}`, { method: 'DELETE' }),

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
  /** 练习交卷记录；scope=all 时（仅管理员生效）返回全部成员，含 userName */
  portalPracticeSubmissions: (spaceId: number | string, practiceId: number | string, scope?: 'all') =>
    req<{ submissions: PracticeSubmission[]; scope?: string }>(
      `/api/portal/space/${spaceId}/practice/${practiceId}/submissions${scope === 'all' ? '?scope=all' : ''}`,
    ),
  portalPracticeSubmissionDetail: (spaceId: number | string, practiceId: number | string, submissionId: number | string) =>
    req<PracticeRecordDetail>(`/api/portal/space/${spaceId}/practice/${practiceId}/submissions/${submissionId}`),
  // 整卷作答云端草稿（换设备续答）
  portalPracticeDraft: (spaceId: number | string, practiceId: number | string) =>
    req<PracticeDraftView>(`/api/portal/space/${spaceId}/practice/${practiceId}/draft`),
  portalSavePracticeDraft: (spaceId: number | string, practiceId: number | string, answers: Record<number, ObjectiveAnswer>) =>
    req<void>(
      `/api/portal/space/${spaceId}/practice/${practiceId}/draft`,
      json({ method: 'PUT', body: JSON.stringify({ answers }) }),
    ),

  // ---- 门户：空间刷题 ----
  portalSpaceQuizzes: (spaceId: number | string) => req<{ quizzes: QuizBrief[] }>(`/api/portal/space/${spaceId}/quizzes`),
  portalQuizProblem: (quizId: number | string, fresh?: boolean) =>
    req<QuizProblemResponse>(`/api/portal/quiz/${quizId}/problem${fresh ? '?fresh=1' : ''}`),
  portalQuizAnswer: (quizId: number | string, problemId: number, answer: ObjectiveAnswer) =>
    req<QuizAnswerResult>(
      `/api/portal/quiz/${quizId}/answer`,
      json({ method: 'POST', body: JSON.stringify({ problemId, answer }) }),
    ),
  portalQuizReset: (quizId: number | string) =>
    req<void>(`/api/portal/quiz/${quizId}/reset`, { method: 'POST' }),
  // 全局错题集
  portalWrongBook: () => req<WrongBookView>(`/api/portal/wrong-book`),
  portalWrongNext: (quizId?: number) =>
    req<QuizProblemResponse>(`/api/portal/wrong-book/next${quizId ? `?quizId=${quizId}` : ''}`),
  portalWrongAnswer: (problemId: number, answer: ObjectiveAnswer) =>
    req<QuizAnswerResult>(
      `/api/portal/wrong-book/answer`,
      json({ method: 'POST', body: JSON.stringify({ problemId, answer }) }),
    ),

  // ---- OrangeOJ：题目做题（/api/oj/problem/:id 保留——做题页/空间内跳转复用） ----
  ojProblem: (id: number) => req<OjProblem>(`/api/oj/problem/${id}`),
  ojDraft: (id: number, lang: CodeLang, ctxKind?: string, ctxId?: number) =>
    req<OjDraft>(`/api/oj/problem/${id}/draft?lang=${lang}${ctxKind ? `&ctxKind=${ctxKind}&ctxId=${ctxId ?? 0}` : ''}`),
  ojSaveDraft: (id: number, lang: CodeLang, code: string, ctxKind?: string, ctxId?: number) =>
    req<void>(`/api/oj/problem/${id}/draft`, json({ method: 'PUT', body: JSON.stringify({ language: lang, code, ...(ctxKind ? { ctxKind, ctxId: ctxId ?? 0 } : {}) }) })),
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
  ojSubmissions: (id: number, trainingId?: number, practiceId?: number, scope?: 'all') =>
    req<{ submissions: Submission[]; scope?: string }>(
      `/api/oj/problem/${id}/submissions?${[
        trainingId ? `trainingId=${trainingId}` : practiceId ? `practiceId=${practiceId}` : '',
        scope === 'all' ? 'scope=all' : '',
      ].filter(Boolean).join('&')}`,
    ),
}
