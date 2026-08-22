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
