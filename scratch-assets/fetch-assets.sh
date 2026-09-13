#!/usr/bin/env bash
# 批量下载 Scratch 素材库（Linux/macOS，aria2c 或 curl 均可）
#
# 用途：某些网络下 CI 抓不到 assets.scratch.mit.edu，可在能联网的机器上先抓一次。
# 清单来源（仓库根目录）：go run ./cmd/scratchassets -list scratch-assets/manifest.tsv
#
# 用法：
#   bash fetch-assets.sh                    # 下载到当前目录
#   OUT=/data/assets JOBS=16 bash fetch-assets.sh
#   RETRY=1 bash fetch-assets.sh            # 只补缺失项
set -euo pipefail

MANIFEST="${MANIFEST:-$(cd "$(dirname "$0")" && pwd)/manifest.tsv}"
OUT="${OUT:-$(cd "$(dirname "$0")" && pwd)}"
JOBS="${JOBS:-8}"
mkdir -p "$OUT"
[ -f "$MANIFEST" ] || { echo "找不到清单：$MANIFEST" >&2; exit 1; }

total=$(wc -l < "$MANIFEST")
echo "清单条目：$total；输出目录：$OUT"

# 只下缺失项（RETRY=1 时）
if [ "${RETRY:-0}" = "1" ]; then
  work=$(mktemp)
  while IFS=$'\t' read -r url name; do
    [ -s "$OUT/$name" ] || printf '%s\t%s\n' "$url" "$name" >> "$work"
  done < "$MANIFEST"
else
  work="$MANIFEST"
fi
echo "待下载：$(wc -l < "$work")"

if command -v aria2c >/dev/null 2>&1; then
  # aria2 支持自定义文件名：-Z 并发 + --out 逐项不便，这里用 xargs 调 curl 更直观
  echo "（检测到 aria2c，但为保证文件名精确，仍使用 curl 逐项下载）"
fi

export OUT
< "$work" xargs -P "$JOBS" -d '\n' -I{} bash -c '
  url="${1%%$'"'"'\t'"'"'*}"; name="${1##*$'"'"'\t'"'"'}"
  dst="$OUT/$name"
  [ -s "$dst" ] && exit 0
  curl -fsSL --retry 3 --connect-timeout 20 --max-time 120 "$url" -o "$dst.part" \
    && mv "$dst.part" "$dst" \
    || { echo "失败: $name"; rm -f "$dst.part"; exit 1; }
' _ {}

ok=$(find "$OUT" -type f ! -name '*.tsv' ! -name 'README.md' ! -name 'fetch-assets.*' | wc -l)
size=$(du -sh "$OUT" | cut -f1)
echo "完成：目录素材文件数 $ok，合计 $size"

cat <<'TIP'

接下来（二选一）：
  A) 把素材文件拷到仓库 scratch-assets/ 后构建：
     docker build --build-arg ASSET_MIRROR_REQUIRED=1 -f scratch/Dockerfile -t orangeoj-scratch .
  B) 打成素材包上传到 GitHub Release，再用 URL 构建（CI 也能用）：
     tar -czf scratch-assets.tar.gz -C "$OUT" .
     docker build --build-arg ASSET_BUNDLE_URL=<素材包URL> -f scratch/Dockerfile -t orangeoj-scratch .
TIP
