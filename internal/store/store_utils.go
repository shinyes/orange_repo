package store

import "strings"

// nonNilSlice 空结果返回空切片而非 nil——nil 切片 JSON 序列化为 null，
// 前端对数组字段做 map/filter/length 时会崩溃（管理 API 与门户消费同一批结构）。
func nonNilSlice[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// placeholders 生成 n 个 "?" 占位符（IN 查询用）。
func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

// anySlice int64 → []any（IN 参数展开用）。
func anySlice(ids []int64) []any {
	out := make([]any, 0, len(ids))
	for _, id := range ids {
		out = append(out, id)
	}
	return out
}

// boolInt bool → 0/1（SQLite 无原生 bool；is_public 存 INTEGER）。
func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
