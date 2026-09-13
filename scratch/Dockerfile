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
#   · 离线素材库（角色/造型/声音/背景）：来自仓库 scratch-assets/，构建期拷入镜像
#
# 构建（在仓库根目录执行）：
#   docker build --build-arg ASSET_MIRROR_REQUIRED=1 -f scratch/Dockerfile -t orangeoj-scratch .
#   （ASSET_MIRROR_REQUIRED=1 会让"素材不完整"直接构建失败；CI 已默认开启）

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

# ---------- 2) 组装离线素材库（角色/造型/声音/背景）----------
# 素材随仓库提供（scratch-assets/，1347 个文件，约 54MB）→ 构建期不依赖外网。
# 只有"仓库里缺失的素材"才会联网补齐；若构建机没有外网，用
#   powershell -File scratch-assets/fetch-assets.ps1 -Out scratch-assets -Retry
# 在能联网的机器上补齐并提交。ASSET_MIRROR_REQUIRED=1 时素材不完整会直接构建失败。
# 版本必须与 go.mod 的 go 指令一致（当前 go 1.25.5）——用 1.24 会报
# "go.mod requires go >= 1.25.5 (running go 1.24.x; GOTOOLCHAIN=local)" 直接构建失败。
# 说明：该阶段只编译 cmd/scratchassets（纯标准库，无第三方依赖）。
FROM golang:1.25-bookworm AS assets
WORKDIR /src
ARG ASSET_MIRROR_REQUIRED=0
COPY go.mod go.sum ./
COPY cmd ./cmd
RUN go build -o /usr/local/bin/scratchassets ./cmd/scratchassets
COPY --from=web /build/node_modules/@scratch/scratch-gui/src/lib/libraries /lib
# 素材来自仓库 scratch-assets/（1347 个文件，随仓库分发）→ 构建期无需外网。
# 若某个素材缺失（例如上游素材库更新后仓库还没补），这里的工具会联网补齐；
# 构建机没有外网时，用 scratch-assets/fetch-assets.ps1 在能联网的机器上补齐后提交即可。
COPY scratch-assets /local-assets
RUN set -eux; \
    mkdir -p /scratch-assets; \
    if [ -d /local-assets ]; then \
      find /local-assets -maxdepth 1 -type f \
        \( -name '*.svg' -o -name '*.png' -o -name '*.wav' -o -name '*.mp3' \
           -o -name '*.jpg' -o -name '*.jpeg' -o -name '*.gif' -o -name '*.webp' \) \
        -exec cp -n {} /scratch-assets/ \; ; \
    fi; \
    preset=$(find /scratch-assets -type f \( -name '*.svg' -o -name '*.png' -o -name '*.wav' -o -name '*.mp3' -o -name '*.jpg' -o -name '*.jpeg' -o -name '*.gif' -o -name '*.webp' \) | wc -l); \
    echo "仓库预置素材文件数: $preset"; \
    /usr/local/bin/scratchassets -lib /lib -out /scratch-assets -workers 12 \
      || /usr/local/bin/scratchassets -lib /lib -out /scratch-assets -workers 4 \
      || true; \
    count=$(find /scratch-assets -type f \( -name '*.svg' -o -name '*.png' -o -name '*.wav' -o -name '*.mp3' -o -name '*.jpg' -o -name '*.jpeg' -o -name '*.gif' -o -name '*.webp' \) | wc -l); \
    echo "素材库镜像文件数: $count（仓库应提供 1347；少了说明 scratch-assets/ 不完整）"; \
    echo "$count" > /scratch-assets/asset-count.txt; \
    if [ "$ASSET_MIRROR_REQUIRED" = "1" ] && [ "$count" -lt 1000 ]; then \
      echo "错误：素材库不完整（$count 个文件，期望 1347）——请确认 scratch-assets/ 已完整提交，" >&2; \
      echo "     或用 scratch-assets/fetch-assets.ps1 -Retry 补齐后重新构建" >&2; \
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
