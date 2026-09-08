// monaco-editor 0.56 的 editor.api.d.ts 缺 breadcrumbs 选项声明（运行时支持），
// 此处补丁合并类型，供关闭顶部面包屑（当前所在函数/类路径）使用。
declare module 'monaco-editor' {
  namespace editor {
    interface IEditorOptions {
      /** 顶部面包屑（当前所在符号路径） */
      breadcrumbs?: {
        enabled?: boolean
      }
    }
  }
}
