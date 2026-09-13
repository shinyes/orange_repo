# OrangeOJ Scratch 编辑器容器
#
# 为什么独立成容器：
#   1) 许可证边界清晰——scratch-gui 是 AGPL-3.0，独立镜像/独立服务让源码提供义务只落在这个组件上
#   2) 发布解耦——Scratch 子构建很重（700+ npm 包 + 素材库数百 MB），主镜像保持精简
#   3) 可选部署——不需要 Scratch 的部署不启动本容器即可
#
# 镜像内容：
#   · scratch-gui 官方 standalone 产物（未修改上游代码）
#   · 本站宿主页 host.js / index.html（我们把编辑器挂起来并暴露受控消息通道）
#   · 积木素材（scratch-blocks/media）
#   · **完整素材库镜像**（角色/造型/声音/背景，构建期从 assets.scratch.mit.edu 抓取）
#
# 构建（在仓库根目录执行）：
#   docker build -f scratch/Dockerfile -t orangeoj-scratch .

# ---------- 1) 取 scratch-gui 产物 + 宿主页 ----------
# 版本对齐主 Dockerfile：node 24（与本地验证环境一致）+ npm install 用 lockfile 保证可复现
FROM node:24.15-bookworm-slim AS web
WORKDIR /build
# 只用宿主页与外层依赖清单；--global=false 必须显式加，避免宿主 npm 配置把包装到全局
COPY app/scratch/package.json app/scratch/package-lock.json ./
RUN npm install --global=false --no-audit --no-fund --legacy-peer-deps
COPY app/scratch/host ./host
RUN set -eux; \
    mkdir -p /out/static/blocks-media /out/chunks; \
    cp node_modules/@scratch/scratch-gui/dist/scratch-gui-standalone.js /out/; \
    cp node_modules/@scratch/scratch-gui/dist/extension-worker.js /out/; \
    cp -r node_modules/@scratch/scratch-gui/dist/chunks/. /out/chunks/; \
    cp host/index.html host/host.js /out/; \
    cp -r node_modules/scratch-blocks/media/. /out/static/blocks-media/

# ---------- 2) 构建期抓取完整素材库（角色/造型/声音/背景）----------
# 需要构建机能访问 https://assets.scratch.mit.edu（GitHub Actions 等常规外网环境可以）。
# 若构建环境访问不到（内网/镜像被墙），构建仍会成功但素材为空——此时有两种补救：
#   a) 在能联网的机器上执行：go run ./cmd/scratchassets -out ./scratch-assets
#      然后把该目录作为构建上下文的一部分（--build-arg 或直接 COPY）打进镜像；
#   b) 把该目录挂载到运行中的容器：-v /path/scratch-assets:/usr/share/nginx/html/static/scratch-assets
# 可用 --build-arg ASSET_MIRROR_REQUIRED=1 让"素材为空"直接构建失败（CI 建议开启）。
# 版本必须与 go.mod 的 go 指令一致（当前 go 1.25.5）——用 1.24 会报
# "go.mod requires go >= 1.25.5 (running go 1.24.x; GOTOOLCHAIN=local)" 直接构建失败。
# 说明：该阶段只编译 cmd/scratchassets（纯标准库，无第三方依赖），但要能访问 HTTPS
# 下载素材，故用带 CA 证书的 bookworm 变体。
FROM golang:1.25-bookworm AS assets
WORKDIR /src
ARG ASSET_MIRROR_REQUIRED=0
# 可选：素材包 URL（tar.gz）。CI 出口访问不到官方 CDN 时，可把一次抓好的素材包挂到
# GitHub Release（或任意可下载地址），用 --build-arg ASSET_BUNDLE_URL=... 让构建直接离线供料。
# 例：ASSET_BUNDLE_URL=https://github.com/<owner>/<repo>/releases/download/scratch-assets-v1/scratch-assets.tar.gz
ARG ASSET_BUNDLE_URL=
COPY go.mod go.sum ./
COPY cmd ./cmd
RUN go build -o /usr/local/bin/scratchassets ./cmd/scratchassets
COPY --from=web /build/node_modules/@scratch/scratch-gui/src/lib/libraries /lib
# 可选：本地已抓取的素材（scratch-assets/ 目录，默认只有 README）——
# 先铺进目标目录，工具会把已存在的文件当"已下载"跳过，只联网补齐缺失部分。
COPY scratch-assets /local-assets
RUN set -eux; \
    mkdir -p /scratch-assets; \
    if [ -d /local-assets ]; then \
      find /local-assets -maxdepth 1 -type f \
        \( -name '*.svg' -o -name '*.png' -o -name '*.wav' -o -name '*.mp3' \
           -o -name '*.jpg' -o -name '*.jpeg' -o -name '*.gif' -o -name '*.webp' \) \
        -exec cp -n {} /scratch-assets/ \; ; \
    fi; \
    if [ -n "$ASSET_BUNDLE_URL" ]; then \
      echo "下载素材包: $ASSET_BUNDLE_URL"; \
      curl -fsSL --retry 3 --connect-timeout 20 "$ASSET_BUNDLE_URL" -o /tmp/assets.tar.gz \
        && tar xzf /tmp/assets.tar.gz -C /scratch-assets \
        || echo "警告：素材包下载/解包失败，继续走联网抓取"; \
    fi; \
    local_count=$(find /scratch-assets -type f \( -name '*.svg' -o -name '*.png' -o -name '*.wav' -o -name '*.mp3' -o -name '*.jpg' -o -name '*.jpeg' -o -name '*.gif' -o -name '*.webp' \) | wc -l); \
    echo "预置素材文件数（仓库 scratch-assets/ + 素材包）: $local_count"; \
    /usr/local/bin/scratchassets -lib /lib -out /scratch-assets -workers 12 \
      || /usr/local/bin/scratchassets -lib /lib -out /scratch-assets -workers 4 \
      || true; \
    count=$(find /scratch-assets -type f \( -name '*.svg' -o -name '*.png' -o -name '*.wav' -o -name '*.mp3' -o -name '*.jpg' -o -name '*.jpeg' -o -name '*.gif' -o -name '*.webp' \) | wc -l); \
    echo "素材库镜像文件数: $count（期望 1000+；为 0 表示既没有预置素材、也访问不到素材源）"; \
    echo "$count" > /scratch-assets/asset-count.txt; \
    if [ "$count" -eq 0 ]; then \
      echo "警告：素材库为空——编辑器核心功能可用，但角色/造型/声音素材库将缺失。"; \
      echo "     解决：在有网络的机器上执行 go run ./cmd/scratchassets -out ./scratch-assets，" \
           "把结果放进仓库 scratch-assets/ 目录；或用 --build-arg ASSET_BUNDLE_URL=<素材包URL>。" >&2; \
    fi; \
    if [ "$ASSET_MIRROR_REQUIRED" = "1" ] && [ "$count" -lt 100 ]; then \
      echo "错误：素材库镜像不完整（$count 个文件）——仓库 scratch-assets/ 未包含素材，" >&2; \
      echo "     且构建机无法访问素材源。请先预置素材（见 scratch-assets/README.md）" >&2; \
      exit 1; \
    fi

# ---------- 3) 运行时：静态服务 ----------
FROM nginx:1.27-alpine
COPY --from=web /out /usr/share/nginx/html
COPY --from=assets /scratch-assets /usr/share/nginx/html/static/scratch-assets
COPY scratch/nginx.conf /etc/nginx/conf.d/default.conf
COPY scratch/SOURCE.md /usr/share/nginx/html/SOURCE.md
COPY scratch/LICENSE-scratch-gui.txt /usr/share/nginx/html/LICENSE-scratch-gui.txt
EXPOSE 80
