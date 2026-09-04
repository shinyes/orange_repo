package server

import (
	"strings"
	"testing"
)

// TestNanoName 校验 nano 随机名：长度、字符集、唯一性与不同长度。
func TestNanoName(t *testing.T) {
	// 默认/指定长度
	if n, _ := NanoName(0); len(n) != 16 {
		t.Fatalf("NanoName(0) = %q len=%d, want 16", n, len(n))
	}
	if n, _ := NanoName(8); len(n) != 8 {
		t.Fatalf("NanoName(8) len=%d, want 8", len(n))
	}
	// 字符集：仅 URL-safe（A-Za-z0-9_-）
	for _, size := range []int{8, 16, 32} {
		n, err := NanoName(size)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range n {
			if !strings.ContainsRune("0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz_-", c) {
				t.Fatalf("NanoName 含非法字符 %q: %q", c, n)
			}
		}
	}
	// 唯一性（1000 次不碰撞）
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		n, _ := NanoName(16)
		if seen[n] {
			t.Fatalf("NanoName 碰撞: %q", n)
		}
		seen[n] = true
	}
}
