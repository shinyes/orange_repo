package store

// nonNilSlice 空结果返回空切片而非 nil——nil 切片 JSON 序列化为 null，
// 前端对数组字段做 map/filter/length 时会崩溃（管理 API 与门户消费同一批结构）。
func nonNilSlice[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
