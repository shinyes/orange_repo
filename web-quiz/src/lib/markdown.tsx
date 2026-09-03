import { useEffect, useMemo, useRef } from 'react'
import { marked } from 'marked'
import DOMPurify from 'dompurify'
// katex 0.16+ 的 auto-render 以 default 形式导出 renderMathInElement（类型见 src/types/katex-auto-render.d.ts）
import renderMathInElementDefault from 'katex/contrib/auto-render'

interface KatexDelimiter {
  left: string
  right: string
  display: boolean
}
interface RenderMathInElementOptions {
  delimiters?: KatexDelimiter[]
  throwOnError?: boolean
}
const renderMathInElement = renderMathInElementDefault as (
  element: HTMLElement,
  options?: RenderMathInElementOptions,
) => void

// 与上游 OrangeOJ 相同的渲染链路：marked → DOMPurify → KaTeX auto-render。
export function renderMarkdown(text: string): string {
  const raw = marked.parse(text ?? '', { async: false })
  return DOMPurify.sanitize(raw)
}

// preserveLineBreaks 把文本中的“孤立换行”转换为 Markdown 硬换行（行尾两空格 → <br>），
// 使多行选项/短文本按编写时的换行真实显示；段落分隔（空行）、已有硬换行语法、
// 代码围栏等场景不受影响。
export function preserveLineBreaks(text: string): string {
  const lines = (text ?? '').split('\n')
  for (let i = 0; i < lines.length - 1; i++) {
    const line = lines[i]
    const nextBlank = lines[i + 1].trim() === ''
    if (nextBlank) continue // 段分隔，交由 Markdown 处理
    if (/ {2,}$/.test(line)) continue // 已显式硬换行
    if (line.endsWith('\\')) continue // 反斜杠换行（部分方言硬换行）
    if (line.trim() === '') continue // 空行自身不动
    // 行首为代码围栏/列表等块级语法时，行尾补两空格可能改变其含义；
    // 选项文本多为纯文本/公式，此处仅跳过围栏行（``` 等）
    if (/^\s*(```|~~~|>\s|[-*+]\s|\d+\.\s)/.test(line)) continue
    lines[i] = line + '  '
  }
  return lines.join('\n')
}

const KATEX_OPTIONS: RenderMathInElementOptions = {
  delimiters: [
    { left: '$$', right: '$$', display: true },
    { left: '$', right: '$', display: false },
    { left: '\\[', right: '\\]', display: true },
    { left: '\\(', right: '\\)', display: false },
  ],
  throwOnError: false,
}

export function Markdown({ text, className }: { text: string; className?: string }) {
  const html = useMemo(() => renderMarkdown(text), [text])
  const ref = useRef<HTMLDivElement>(null)
  useEffect(() => {
    if (ref.current) renderMathInElement(ref.current, KATEX_OPTIONS)
  }, [html])
  return <div ref={ref} className={className} dangerouslySetInnerHTML={{ __html: html }} />
}
