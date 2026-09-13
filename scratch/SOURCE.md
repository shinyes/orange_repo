# Scratch 编辑器容器 · 源码与许可说明

本容器分发的是 **Scratch 编辑器**（`@scratch/scratch-gui`，由 Scratch Foundation 开发，
许可证 **AGPL-3.0-only**，全文见同目录 `LICENSE-scratch-gui.txt`）。

## 关于 AGPL 的来源提供义务

AGPL 第 13 条要求：如果你**修改**该程序并通过网络向用户提供服务，需要向这些用户提供
「你修改后的版本」的对应源码。本容器遵循以下做法：

1. **未修改上游 Scratch 源码。** 镜像内的 `scratch-gui-standalone.js` 等文件是
   `@scratch/scratch-gui`（版本见下）npm 包的**原样产物**，逐字节未改。
2. **本站新增的只是"宿主页"**（`index.html` / `host.js`）：它调用官方公开接口
   （`GUI.EditorState`、`GUI.createStandaloneRoot`、`onVmInit` 回调、`vm.saveProjectSb3()`、
   `vm.loadProject()`、`GUI.ScratchStorage.addWebStore`）把编辑器挂起来，并通过受控的
   跨窗口消息与主站通信。这些文件是我们自己的代码，不是 Scratch 的修改版。
3. 因此本容器**不构成 AGPL 意义上的"修改版"**；但为便于核对与再分发，我们公开：

   - 上游源码：https://github.com/scratchfoundation/scratch-editor （包路径 `packages/scratch-gui`）
   - 本容器的构建脚本与宿主页源码：本仓库 `scratch/Dockerfile`、`app/scratch/host/`
   - 所用版本：见 `app/scratch/package.json` 中的 `@scratch/scratch-gui`

## 素材库

镜像内的 `static/scratch-assets/` 是 Scratch 官方素材库（角色/造型/声音/背景）的本地镜像，
构建期由 `cmd/scratchassets` 从 `https://assets.scratch.mit.edu` 抓取，供离线使用。
素材版权归 Scratch Foundation 及其贡献者所有，随 AGPL 组件一同分发。

## 如需修改 Scratch 源码

如果将来确实要改上游代码（例如在编辑器内部加按钮），请把补丁放在本仓库并**在此页面补充
修改版源码的获取方式**，以满足 AGPL 第 13 条。
