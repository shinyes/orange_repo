package server

import "log/slog"

// warnCleanup 删除主记录后的作答侧清理失败提示（孤儿数据不阻塞删除返回：
// 主记录已删时返回 500 会让客户端盲目重试且必然 404；残留数据可后续统一清扫）。
func warnCleanup(what string, err error) {
	slog.Warn("删除后作答侧清理失败（残留可后续清扫）", "scope", what, "err", err)
}
