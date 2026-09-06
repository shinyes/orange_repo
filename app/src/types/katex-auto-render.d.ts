// katex 贡献包 auto-render 无官方类型声明，此处补充。
// 注意：0.16+ 运行时以 default 导出 renderMathInElement（见 markdown.tsx）。
declare module 'katex/contrib/auto-render' {
  export interface KatexDelimiter {
    left: string
    right: string
    display: boolean
  }
  export interface RenderMathInElementOptions {
    delimiters?: KatexDelimiter[]
    throwOnError?: boolean
    ignoredTags?: string[]
    ignoredClasses?: string[]
    errorCallback?: (msg: string, err: Error) => void
  }
  export function renderMathInElement(element: HTMLElement, options?: RenderMathInElementOptions): void
  export default renderMathInElement
}
