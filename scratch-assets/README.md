# Scratch 素材库（角色 / 造型 / 声音 / 背景）

本目录是 Scratch 编辑器**离线素材库**，随仓库一起分发，供 `orangeoj-scratch` 镜像构建时打进镜像。
成员在 Scratch 创作页打开「角色 / 造型 / 声音」库时，素材由本站提供，**不访问外网**。

## 现状

| 项 | 值 |
|---|---|
| 素材文件数 | **1347**（svg 804 / wav 350 / png 193） |
| 合计体积 | 约 54 MB |
| 来源 | `https://assets.scratch.mit.edu`（Scratch 官方素材库） |
| 清单 | `manifest.tsv`（每行 `URL<TAB>文件名`，1347 行） |

构建时 Dockerfile 会把这些文件拷进镜像的 `/static/scratch-assets/`，并写入
`asset-count.txt` 便于自查；若某个文件缺失，会再尝试联网补齐（构建机无外网也能用）。

## 更新素材（当上游素材库新增内容时）

```powershell
# 1) 重新生成清单（仓库根目录）
go run ./cmd/scratchassets -list scratch-assets/manifest.tsv

# 2) 下载缺失项（Windows；-Jobs 8 并发，可按需调整）
powershell -ExecutionPolicy Bypass -File scratch-assets\fetch-assets.ps1 -Out scratch-assets -Retry

# 3) 提交更新的素材
git add scratch-assets && git commit -m "chore(scratch): 更新离线素材库"
```

> 注意：`fetch-assets.ps1` 必须保存为 **UTF-8 带 BOM**，否则 Windows PowerShell 5.1 会把中文
> 按 ANSI 解析而报语法错误。`fetch-assets.sh` 供 Linux/macOS 使用。

## 备选：不把素材放进仓库时

可用素材包方式（适合想让仓库保持精简的部署）：

```bash
tar -czf scratch-assets.tar.gz -C scratch-assets .
# 把该包挂到 GitHub Release，然后构建时指定：
docker build --build-arg ASSET_BUNDLE_URL=<素材包URL> \
  --build-arg ASSET_MIRROR_REQUIRED=1 -f scratch/Dockerfile -t orangeoj-scratch .
```

## 自查（镜像起来后）

```bash
docker run --rm --entrypoint sh orangeoj-scratch -c \
  'cat /usr/share/nginx/html/static/scratch-assets/asset-count.txt'
# 期望 1347（至少 1000+）
```

## 版权

素材版权归 Scratch Foundation 及其贡献者所有，随 AGPL-3.0 的 Scratch 编辑器组件一同分发
（见 `scratch/SOURCE.md`、`scratch/LICENSE-scratch-gui.txt`）。
