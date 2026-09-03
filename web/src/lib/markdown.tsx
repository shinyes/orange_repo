import { useEffect, useMemo, useRef } from 'react'
import { marked } from 'marked'
import DOMPurify from 'dompurify'

interface KatexDelimiter {
  left: string
  right: string
  display: boolean
}
interface RenderMathInElementOptions {
  delimiters?: KatexDelimiter[]
  throwOnError?: boolean
}

// 与上游 OrangeOJ 相同的渲染链路：marked → DOMPurify → KaTeX auto-render。
// KaTeX（体积大）按需动态加载：仅在真正出现 Markdown 内容时拉取，不进首屏主包。
export function renderMarkdown(text: string): string {
  const raw = marked.parse(text ?? '', { async: false })
  return DOMPurify.sanitize(raw)
}

// preserveLineBreaks 把文本中的“孤立换行”转换为 Markdown 硬换行（行尾两空格 → <br>），
// 使多行选项/短文本按编写时的换行真实显示；段落分隔（空行）、已有硬换行语法、
// 代码围栏等场景不受影响。
export function preserveLineBreaks(text: string): string {
  const lines = (text ?? '').split('\n')
  let inFence = false
  let prevWasList = false // 上一行是否为列表项（后续缩进续行需保持列表结构）
  for (let i = 0; i < lines.length - 1; i++) {
    const line = lines[i]
    const nextBlank = lines[i + 1].trim() === ''
    if (nextBlank) continue // 段分隔，交由 Markdown 处理
    if (/ {2,}$/.test(line)) continue // 已显式硬换行
    if (line.endsWith('\\')) continue // 反斜杠换行（部分方言硬换行）
    if (line.trim() === '') continue // 空行自身不动
    const trimmed = line.trim()
    // 代码围栏开始/结束：围栏内一律不动
    if (/^```|^~~~/.test(trimmed)) {
      inFence = !inFence
      continue
    }
    if (inFence) continue
    const isListItem = /^\s*[-+*]\s/.test(line) || /^\s*\d+[.)]\s/.test(line)
    // 块级语法行跳过（行尾补两空格会改变语义）：
    //   表格（含分隔行）、引用、列表项及其缩进续行、setext 标题（===/--- 下划线）、
    //   缩进代码块、HTML 块、主题分隔线
    if (
      /^\s*\|/.test(line) || // 表格行（含对齐分隔行）
      /^\s*>\s?/.test(line) || // 引用
      isListItem || // 列表项
      prevWasList || // 列表项缩进续行
      /^\s*([-=])\1{2,}\s*$/.test(line) || // setext 标题下划线
      /^\s{4,}\S/.test(line) || // 缩进代码块
      /^\s*<\/?[a-zA-Z][^>]*>$/.test(line) || // HTML 块
      /^\s*([-*_])\s*\1\s*\1/.test(line) // 分隔线
    ) {
      prevWasList = isListItem
      continue
    }
    prevWasList = false
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
    if (!ref.current) return
    // 仅当渲染结果里疑似含公式定界符时才动态加载 KaTeX（约 250kB，避免进首屏）
    const looksMath = /\$|\\\(|\\\[/.test(html)
    if (!looksMath) return
    let alive = true
    void import('katex/contrib/auto-render').then((mod) => {
      if (!alive || !ref.current) return
      const renderMathInElement = (mod as { default?: unknown }).default ?? mod
      ;(renderMathInElement as (el: HTMLElement, opts?: RenderMathInElementOptions) => void)(ref.current, KATEX_OPTIONS)
    }).catch(() => { /* 公式渲染失败不影响正文 */ })
    return () => {
      alive = false
    }
  }, [html])
  return <div ref={ref} className={className} dangerouslySetInnerHTML={{ __html: html }} />
}
