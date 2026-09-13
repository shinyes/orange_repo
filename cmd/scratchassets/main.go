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
	"bufio"
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

// assetHosts 素材源主机（按顺序回退）：官方主站与 CDN 都是同一套 internalapi 路径。
// 某些网络/CI 出口对其中之一不可达，回退能显著提高抓取成功率。
var assetHosts = []string{
	"https://assets.scratch.mit.edu/internalapi/asset/%s/get/",
	"https://cdn.assets.scratch.mit.edu/internalapi/asset/%s/get/",
}

var md5extRe = regexp.MustCompile(`^[0-9a-f]{32}\.[a-z0-9]{2,5}$`)

func main() {
	libDir := flag.String("lib", "app/scratch/node_modules/@scratch/scratch-gui/src/lib/libraries", "素材库 JSON 目录")
	outDir := flag.String("out", filepath.Join("data", "scratch-assets"), "素材落盘目录")
	workers := flag.Int("workers", 8, "并发下载数")
	limit := flag.Int("limit", 0, "只下载前 N 个（0=全部；用于冒烟测试）")
	timeoutSec := flag.Int("timeout", 20, "单个素材的请求超时（秒）")
	listPath := flag.String("list", "", "只生成下载清单（TSV：URL<TAB>文件名）到该路径，不下载")
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

	// 只出清单：给"人工/第三方下载器"用的模式（某些网络下 CI 抓不到素材源）
	if strings.TrimSpace(*listPath) != "" {
		f, err := os.Create(*listPath)
		if err != nil {
			fmt.Println("创建清单失败:", err)
			os.Exit(1)
		}
		defer f.Close()
		bw := bufio.NewWriter(f)
		for _, id := range ids {
			fmt.Fprintf(bw, "%s\t%s\n", fmt.Sprintf(assetHosts[0], id), id)
		}
		if err := bw.Flush(); err != nil {
			fmt.Println("写入清单失败:", err)
			os.Exit(1)
		}
		fmt.Printf("清单已写入 %s（每行：URL<TAB>文件名；可用 scratch-assets/fetch-assets.ps1 批量下载）\n", *listPath)
		return
	}

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fmt.Println("创建输出目录失败:", err)
		os.Exit(1)
	}

	var done, skipped, failed int64
	var bytes int64
	var consecutiveFail int64 // 连续失败计数：网络整体不可达时快速退出，避免逐个大超时拖垮构建
	var lastErrMu sync.Mutex
	var lastErrMsg string
	client := &http.Client{Timeout: time.Duration(*timeoutSec) * time.Second}
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
				if atomic.AddInt64(&failed, 1) == 1 {
					// 首个失败单独提示，便于一眼看出是网络问题还是个别资源缺失
					fmt.Println("  首个失败:", id, err)
				}
				lastErrMu.Lock()
				lastErrMsg = id + ": " + err.Error()
				lastErrMu.Unlock()
				if atomic.AddInt64(&consecutiveFail, 1) >= 25 {
					lastErrMu.Lock()
					msg := lastErrMsg
					lastErrMu.Unlock()
					fmt.Printf("连续 %d 个素材下载失败，判定素材源不可达（最新错误：%s）\n", 25, msg)
					fmt.Println("提示：可在有网络的机器上抓取后放入仓库 scratch-assets/（见该目录 README），再重新构建")
					os.Exit(3)
				}
				failMu.Lock()
				if len(failures) < 40 {
					failures = append(failures, id+": "+err.Error())
				}
				failMu.Unlock()
				return
			}
			atomic.StoreInt64(&consecutiveFail, 0)
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

// fetch 下载单个素材：在多个素材源之间回退，每个源重试 2 次。
func fetch(client *http.Client, id string) ([]byte, error) {
	var lastErr error
	for _, host := range assetHosts {
		for attempt := 0; attempt < 2; attempt++ {
			if attempt > 0 {
				time.Sleep(time.Duration(attempt) * 700 * time.Millisecond)
			}
			resp, err := client.Get(fmt.Sprintf(host, id))
			if err != nil {
				lastErr = err
				continue
			}
			if resp.StatusCode != http.StatusOK {
				_ = resp.Body.Close()
				lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
				if resp.StatusCode == http.StatusNotFound {
					// 素材确实不存在（库 JSON 引用了不存在的资源）：换源也没用，直接返回
					return nil, lastErr
				}
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
	}
	return nil, lastErr
}
