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
COPY go.mod go.sum ./
COPY cmd ./cmd
RUN go build -o /usr/local/bin/scratchassets ./cmd/scratchassets
COPY --from=web /build/node_modules/@scratch/scratch-gui/src/lib/libraries /lib
RUN set -eux; \
    mkdir -p /scratch-assets; \
    /usr/local/bin/scratchassets -lib /lib -out /scratch-assets -workers 12 \
      || /usr/local/bin/scratchassets -lib /lib -out /scratch-assets -workers 4 \
      || true; \
    count=$(find /scratch-assets -type f | wc -l); \
    echo "素材库镜像文件数: $count"; \
    echo "$count" > /scratch-assets/.count; \
    if [ "$ASSET_MIRROR_REQUIRED" = "1" ] && [ "$count" -lt 100 ]; then \
      echo "错误：素材库镜像为空（$count 个文件）——构建机无法访问 assets.scratch.mit.edu，或需先手动准备素材目录" >&2; \
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
