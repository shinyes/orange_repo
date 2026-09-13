// scratchassets：把 Scratch 官方素材库（角色/造型/声音/背景）镜像到本地，供离线使用。
//
// 背景：scratch-gui 的素材库 JSON 只带 md5ext（资源 id），实际字节由 scratch-storage 从
// assets.scratch.mit.edu 拉取。要让 Scratch 空间完全离线可用，必须把这些素材抓到本站。
//
// 用法：
//
//	go run ./cmd/scratchassets -lib app/scratch/node_modules/@scratch/scratch-gui/src/lib/libraries -out data/scratch-assets
//
// 行为：扫描库 JSON 里所有 md5ext，逐个下载到 -out（已存在则跳过），失败重试并汇总。
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const assetBase = "https://assets.scratch.mit.edu/internalapi/asset/%s/get/"

var md5extRe = regexp.MustCompile(`^[0-9a-f]{32}\.[a-z0-9]{2,5}$`)

func main() {
	libDir := flag.String("lib", "app/scratch/node_modules/@scratch/scratch-gui/src/lib/libraries", "素材库 JSON 目录")
	outDir := flag.String("out", filepath.Join("data", "scratch-assets"), "素材落盘目录")
	workers := flag.Int("workers", 8, "并发下载数")
	limit := flag.Int("limit", 0, "只下载前 N 个（0=全部；用于冒烟测试）")
	flag.Parse()

	files, err := os.ReadDir(*libDir)
	if err != nil {
		fmt.Println("读取素材库目录失败:", err)
		os.Exit(1)
	}
	set := map[string]bool{}
	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(*libDir, f.Name()))
		if err != nil {
			fmt.Println("读取失败", f.Name(), err)
			continue
		}
		var v any
		if err := json.Unmarshal(raw, &v); err != nil {
			fmt.Println("解析失败", f.Name(), err)
			continue
		}
		collect(v, set)
	}
	ids := make([]string, 0, len(set))
	for id := range set {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	if *limit > 0 && *limit < len(ids) {
		ids = ids[:*limit]
	}
	fmt.Printf("素材库引用资源 %d 个（去重后）\n", len(ids))

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fmt.Println("创建输出目录失败:", err)
		os.Exit(1)
	}

	var done, skipped, failed int64
	var bytes int64
	client := &http.Client{Timeout: 60 * time.Second}
	sem := make(chan struct{}, *workers)
	var wg sync.WaitGroup
	var failMu sync.Mutex
	var failures []string

	for i, id := range ids {
		dst := filepath.Join(*outDir, id)
		if st, err := os.Stat(dst); err == nil && st.Size() > 0 {
			atomic.AddInt64(&skipped, 1)
			atomic.AddInt64(&bytes, st.Size())
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(id, dst string) {
			defer wg.Done()
			defer func() { <-sem }()
			data, err := fetch(client, id)
			if err != nil {
				atomic.AddInt64(&failed, 1)
				failMu.Lock()
				if len(failures) < 40 {
					failures = append(failures, id+": "+err.Error())
				}
				failMu.Unlock()
				return
			}
			tmp := dst + ".part"
			if err := os.WriteFile(tmp, data, 0o644); err != nil {
				atomic.AddInt64(&failed, 1)
				return
			}
			if err := os.Rename(tmp, dst); err != nil {
				atomic.AddInt64(&failed, 1)
				_ = os.Remove(tmp)
				return
			}
			atomic.AddInt64(&done, 1)
			atomic.AddInt64(&bytes, int64(len(data)))
		}(id, dst)
		if (i+1)%200 == 0 {
			fmt.Printf("  进度 %d/%d（新下载 %d，跳过 %d，失败 %d，%.1f MB）\n",
				i+1, len(ids), atomic.LoadInt64(&done), atomic.LoadInt64(&skipped), atomic.LoadInt64(&failed),
				float64(atomic.LoadInt64(&bytes))/1024/1024)
		}
	}
	wg.Wait()

	fmt.Printf("完成：新下载 %d，已存在跳过 %d，失败 %d，共 %.1f MB → %s\n",
		done, skipped, failed, float64(bytes)/1024/1024, *outDir)
	for _, f := range failures {
		fmt.Println("  失败:", f)
	}
	if failed > 0 {
		os.Exit(2) // 便于重跑补齐
	}
}

// collect 递归收集任意层级对象里的 md5ext 字段。
func collect(v any, out map[string]bool) {
	switch t := v.(type) {
	case map[string]any:
		if s, ok := t["md5ext"].(string); ok && md5extRe.MatchString(s) {
			out[s] = true
		}
		for _, child := range t {
			collect(child, out)
		}
	case []any:
		for _, child := range t {
			collect(child, out)
		}
	}
}

// fetch 下载单个素材（带 3 次重试）。
func fetch(client *http.Client, id string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 700 * time.Millisecond)
		}
		resp, err := client.Get(fmt.Sprintf(assetBase, id))
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode != http.StatusOK {
			_ = resp.Body.Close()
			lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
			continue
		}
		data, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}
		if len(data) == 0 {
			lastErr = fmt.Errorf("空响应")
			continue
		}
		return data, nil
	}
	return nil, lastErr
}
