# OrangeOJ Scratch 编辑器容器
#
# 为什么独立成容器：
#   1) 许可证边界清晰——scratch-gui 是 AGPL-3.0，独立镜像/独立服务让源码提供义务只落在这个组件上
#   2) 发布解耦——Scratch 子构建很重（700+ npm 包），主镜像保持精简
#   3) 可选部署——不需要 Scratch 的部署不启动本容器即可
#
# 镜像内容：
#   · scratch-gui 官方 standalone 产物（未修改上游代码）
#   · 本站宿主页 host.js / index.html（把编辑器挂起来并暴露受控消息通道）
#   · 积木素材（scratch-blocks/media）
#   · 离线素材库（角色/造型/声音/背景）：直接取仓库 scratch-assets/（1347 个文件，约 54MB）
#
# 构建（在仓库根目录执行）：构建**不需要外网**，素材随仓库提供
#   docker build -f scratch/Dockerfile -t orangeoj-scratch .
# 镜像内自查素材数量：
#   docker run --rm --entrypoint sh orangeoj-scratch -c \
#     'cat /usr/share/nginx/html/static/scratch-assets/asset-count.txt'   # 期望 1347

# ---------- 1) 取 scratch-gui 产物 + 宿主页 ----------
FROM node:24.15-bookworm-slim AS web
WORKDIR /build
# --global=false 显式加上：某些宿主 npm 配置默认 global=true，会把包装到全局而不是本地
COPY app/scratch/package.json app/scratch/package-lock.json ./
RUN npm install --global=false --no-audit --no-fund --legacy-peer-deps
COPY app/scratch/host ./host
RUN set -eux; \
    mkdir -p /out/static/blocks-media/default /out/static/blocks-media/high-contrast /out/chunks; \
    cp node_modules/@scratch/scratch-gui/dist/scratch-gui-standalone.js /out/; \
    cp node_modules/@scratch/scratch-gui/dist/extension-worker.js /out/; \
    cp -r node_modules/@scratch/scratch-gui/dist/chunks/. /out/chunks/; \
    cp host/index.html host/host.js /out/; \
    cp -r node_modules/scratch-blocks/media/. /out/static/blocks-media/; \
    cp -r node_modules/scratch-blocks/media/. /out/static/blocks-media/default/; \
    cp -r node_modules/scratch-blocks/media/. /out/static/blocks-media/high-contrast/; \
    test -f /out/static/blocks-media/default/repeat.svg; \
    test -f /out/static/blocks-media/default/zoom-in.svg; \
    test -f /out/static/blocks-media/high-contrast/repeat.svg

# ---------- 2) 运行时：静态服务 + 素材库校验 ----------
# 素材完整性按仓库清单校验：数量少于 manifest.tsv 的行数即构建失败——
# 避免再出现"构建成功但素材缺失"的小镜像（历史上出过一次：素材没进去，包只有 71MB）。
# 素材更新流程见 scratch-assets/README.md。
FROM nginx:1.27-alpine
COPY --from=web /out /usr/share/nginx/html
COPY scratch-assets /tmp/scratch-assets
RUN set -eux; \
    assets=/usr/share/nginx/html/static/scratch-assets; \
    mkdir -p "$assets"; \
    find /tmp/scratch-assets -maxdepth 1 -type f \
      \( -name '*.svg' -o -name '*.png' -o -name '*.wav' -o -name '*.mp3' \
         -o -name '*.jpg' -o -name '*.jpeg' -o -name '*.gif' -o -name '*.webp' \) \
      -exec cp -n {} "$assets/" \; ; \
    count=$(find "$assets" -type f | wc -l); \
    expected=$(grep -c . /tmp/scratch-assets/manifest.tsv 2>/dev/null || echo 1000); \
    echo "素材库文件数: $count / 清单期望: $expected"; \
    echo "$count" > "$assets/asset-count.txt"; \
    if [ "$count" -lt "$expected" ]; then \
      echo "错误：素材库不完整（$count < $expected）——请确认仓库 scratch-assets/ 已完整提交；" >&2; \
      echo "     更新方式见 scratch-assets/README.md" >&2; \
      exit 1; \
    fi; \
    rm -rf /tmp/scratch-assets
COPY scratch/nginx.conf /etc/nginx/conf.d/default.conf
COPY scratch/SOURCE.md /usr/share/nginx/html/SOURCE.md
COPY scratch/LICENSE-scratch-gui.txt /usr/share/nginx/html/LICENSE-scratch-gui.txt
EXPOSE 80
