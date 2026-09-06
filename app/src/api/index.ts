// 单前端应用 API 汇总导出：调用方统一从 '@/api' 引用，屏蔽具体文件拆分。
//  - api：门户+认证兼容门面（沿用迁移前 web-quiz/src/lib/api.ts 的 api.* 命名，功能等价零改动）；
//  - portalApi：门户命名空间（同 api 中的 portal*/oj* 方法，便于后续收敛调用点）；
//  - authApi：认证命名空间（登录/登出/改密/会话，门户与管理端共用同一会话）；
//  - adminApi：管理端命名空间（第二阶段并入 web/ 管理接口，本阶段仅占位）。
export { ApiError, req, json, UNAUTHORIZED_EVENT } from './client'
export { authApi } from './auth'
export { portalApi } from './portal'
export { adminApi } from './admin'
export type * from './types'

import { authApi } from './auth'
import { portalApi } from './portal'

// 兼容门面：认证 + 门户合并为单对象（方法名与迁移前一致；管理端并入后另行组织，不混入此门面）。
export const api = {
  ...authApi,
  ...portalApi,
}
