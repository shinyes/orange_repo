package store

import (
	"encoding/json"
	"fmt"
	"testing"
)

// 性能基线：大量题目 + 多标签下 ListTagFacets 的耗时（标签树渲染核心路径）。
func BenchmarkListTagFacets(b *testing.B) {
	dir := b.TempDir()
	st, err := Open(dir)
	if err != nil {
		b.Fatal(err)
	}
	defer st.Close()

	// 构造 5000 题 × 每个 1-3 标签，标签形如 a/b/c 层级
	tagPool := make([]string, 0, 60)
	for i := 1; i <= 5; i++ {
		for j := 1; j <= 5; j++ {
			for k := 1; k <= 2; k++ {
				tagPool = append(tagPool, fmt.Sprintf("科%d/章%d/节%d", i, j, k))
			}
		}
	}
	for n := 0; n < 5000; n++ {
		tags := []string{tagPool[n%len(tagPool)]}
		if n%3 == 0 {
			tags = append(tags, tagPool[(n*7)%len(tagPool)])
		}
		tj, _ := json.Marshal(tags)
		if _, err := st.DB.Exec(`INSERT INTO problems(type,title,tags_json,statement_md,body_json,answer_json,solutions_json)
			VALUES('single_choice',?,'`+string(tj)+`','','{}','{}','[]')`, fmt.Sprintf("题%d", n)); err != nil {
			b.Fatal(err)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := st.ListTagFacets(ProblemFilter{}); err != nil {
			b.Fatal(err)
		}
	}
}
