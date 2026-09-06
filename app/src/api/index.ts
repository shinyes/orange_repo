// 单前端应用 API 汇总导出：调用方统一从 '@/api' 引用，屏蔽具体文件拆分。
//  - api：认证+门户+管理 兼容门面（方法名沿用迁移前两前端 api.* 命名，功能等价零改动）；
//  - authApi：认证命名空间（登录/登出/改密/会话，门户与管理端共用同一会话）；
//  - portalApi：门户命名空间（同 api 中的 portal*/oj* 方法，便于后续收敛调用点）；
//  - adminApi：管理端命名空间（同 api 中的 problems/tags/domains/spaces/... 方法）。
export { ApiError, req, json, UNAUTHORIZED_EVENT } from './client'
export { authApi } from './auth'
export { portalApi } from './portal'
export { adminApi, setDomain, getDomain, filterQuery } from './admin'
export type * from './types'

import { authApi } from './auth'
import { portalApi } from './portal'
import { adminApi } from './admin'

// 兼容门面：认证 + 门户 + 管理合并为单对象（方法名与迁移前一致，门户与
// 迁入的管理页调用点均直接使用；命名空间版本供后续收敛）。
export const api = {
  ...authApi,
  ...portalApi,
  ...adminApi,
}
