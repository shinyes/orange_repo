# 本地素材库目录（可选）

Scratch 镜像构建时，素材库（角色/造型/声音/背景）默认在**构建机联网抓取**
（`cmd/scratchassets` → `https://assets.scratch.mit.edu`，共约 1347 个资源）。

如果构建机访问不到该域名（内网 / 被墙），可以在一台**能联网的机器**上先抓下来，
把结果放进本目录（保持目录结构：`<md5>.<ext>` 平铺即可），再构建镜像——
构建脚本会**先使用本目录里的文件、再联网补齐缺失的部分**：

```bash
# 在能联网的机器上（仓库根目录）
go run ./cmd/scratchassets -out ./scratch-assets -workers 12
# 完成后把整个 scratch-assets/ 目录拷到构建机（或直接提交/挂载）

# 构建机（离线也能产出完整镜像）
docker build --build-arg ASSET_MIRROR_REQUIRED=1 -f scratch/Dockerfile -t orangeoj-scratch .
```

自检（镜像起来后）：

```bash
docker run --rm --entrypoint sh orangeoj-scratch -c \
  'wc -l < /usr/share/nginx/html/static/scratch-assets/asset-count.txt'
# 期望 1000+
```

本目录除本说明外为空时，表示"完全依赖构建期联网抓取"。
