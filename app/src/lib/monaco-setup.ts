// Monaco Editor 本地化配置：
//  - 通过 @monaco-editor/react 的 loader 指向本地 monaco-editor 包（不从 CDN 加载）；
//  - worker 用 Vite `?worker` 打包为本地 chunk（editor/json/css/html/ts），无外部请求。
//
// 用法：在应用入口或使用前调用一次 setupMonaco()。
import { loader } from '@monaco-editor/react'
import * as monaco from 'monaco-editor'
// monaco-editor 的 package exports 未暴露 esm 子路径（./esm/* 映射会双前缀），
// 用相对路径直入包内文件，交由 Vite ?worker 本地打包。
import EditorWorker from '../../node_modules/monaco-editor/esm/vs/editor/editor.worker.js?worker'
import JsonWorker from '../../node_modules/monaco-editor/esm/vs/language/json/json.worker.js?worker'
import CssWorker from '../../node_modules/monaco-editor/esm/vs/language/css/css.worker.js?worker'
import HtmlWorker from '../../node_modules/monaco-editor/esm/vs/language/html/html.worker.js?worker'
import TsWorker from '../../node_modules/monaco-editor/esm/vs/language/typescript/ts.worker.js?worker'

let configured = false

export function setupMonaco() {
  if (configured) return
  configured = true

  // worker 分发：语言 worker 用不到时不会创建（按需加载本地 chunk）
  self.MonacoEnvironment = {
    getWorker(_, label: string) {
      switch (label) {
        case 'json':
          return new JsonWorker()
        case 'css':
        case 'scss':
        case 'less':
          return new CssWorker()
        case 'html':
        case 'handlebars':
        case 'razor':
          return new HtmlWorker()
        case 'typescript':
        case 'javascript':
          return new TsWorker()
        default:
          return new EditorWorker()
      }
    },
  }

  // 指向本地 monaco-editor（关键：@monaco-editor/react 默认走 CDN jsdelivr）
  loader.config({ monaco })
}

export { monaco }
