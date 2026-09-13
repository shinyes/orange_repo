# 小恐龙（内联第三方代码）

本项目「休息时间」的小恐龙游戏**不是自研实现**，而是内联的开源实现：

| 项 | 值 |
|---|---|
| 上游 | [wayou/t-rex-runner](https://github.com/wayou/t-rex-runner)（分支 `gh-pages`） |
| 来源 | 从 Chromium 的离线小恐龙（T-Rex Runner）提取 |
| 许可 | BSD 3-Clause（Chromium 部分为 BSD-style），见 `LICENSE-dino.txt` |
| 文件 | `dino-runner.js`（逐字节拷贝 + 末尾适配段）、`sprites-1x.png` / `sprites-2x.png` |

## 我们对上游做的全部改动

1. IIFE 边界两行：`(function () {` → `const dinoInternals = (function () {`，并在 IIFE 收尾处 `return { Runner: Runner }`，
   使文件末尾的适配层能拿到构造器（上游此处直接结束，仅能在错误页里用全局 `new Runner(...)` 启动）。
2. 文件末尾新增**适配段**（`createDinoRunner` / `dinoScore` / `dinoCrashed` / `disposeDinoRunner`）：
   - 不再用 `DOMContentLoaded` 自动启动，改由 React 组件在精灵图加载完成后显式创建；
   - `loadSounds` 置空：本项目不内置上游音频素材（其 `index.html` 的 `<template id="audio-resources">` 我们不带）；
   - `setArcadeMode` / `setArcadeModeContainerScale` 置空：上游按整页游戏处理，会给 `body` 加 `arcade-mode`
     并按窗口尺寸对 canvas 做 `transform: scale()`，会破坏「游戏 + 榜单」并排布局；
   - `disposeDinoRunner` 复位 `Runner.instance_` 单例，并让旧实例的键盘/触摸处理失效
     （上游把监听 bind 到 document 且无法解绑；不失效的话按空格会让已丢弃的实例对着已移除的
     canvas 空转）；
   - 宿主节点在每次创建前清空（`replaceChildren`）：上游是把新建的 `.runner-container` **追加**进去的，
     不清空会在重开时叠加出第二个游戏窗口。

除以上内容，`dino-runner.js` 与上游一致，便于日后同步升级。

## 升级方式

1. 重新下载上游 `index.js` 与精灵图；
2. 仅重做上面第 1 步的两行改动；
3. 把 `%TEMP%` 之外的新文件覆盖 `dino-runner.js`，保留文件末尾适配段（可从 git 历史取回）。

## 依赖的上游 DOM 约定（React 组件已提供，见 `DinoGame.tsx`）

- 构造参数是**容器选择器**，canvas 会创建在该容器内；
- 页面需存在 `.icon-offline` 元素（上游 `init()` 会把它隐藏）；
- 需存在 `<img id="offline-resources-1x">` 与 `…-2x`（精灵图，构造时读取尺寸算坐标）；
- 容器需要 `padding-left` 可解析（我们用 `px-0`）、宽度决定 canvas 宽度（上限 600）。

## 新增别的游戏怎么做

1. 在 `app/src/pages/break/games/<game-id>/` 放实现，导出一个 React 组件（props：`onGameOver(score)`、`gameKey`）；
2. 在 `app/src/pages/break/games/registry.ts` 的 `GAMES` 追加一条（`id` 即榜单维度，后端无需改动）；
3. 榜单、成绩提交、本域/全域切换、我的排名都由 `BreakPage` 与后端 `/api/portal/game/:game/*` 通用处理。
