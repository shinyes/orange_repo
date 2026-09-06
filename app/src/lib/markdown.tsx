import { useEffect, useMemo, useState } from 'react'
import { marked } from 'marked'
import DOMPurify from 'dompurify'

// 与上游 OrangeOJ 相同的渲染链路：marked → DOMPurify → KaTeX。
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

// renderMathHTML 把 sanitize 后的 HTML 中的公式定界符（$$…$$ / $…$ / \[…\] / \(…\)）
// 渲染为 KaTeX HTML（renderToString）。**文本级替换，无 DOM 侵入、可重复执行**——
// 避免了 auto-render 的动态 DOM 修改在组件重渲染/重挂时丢失或破坏公式的问题。
function renderMathHTML(html: string, katex: { renderToString: (tex: string, opts: Record<string, unknown>) => string }): string {
  const esc = (s: string) => s
    .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;').replace(/'/g, '&#39;')
  const render = (tex: string, display: boolean): string => {
    try {
      return katex.renderToString(tex, {
        displayMode: display,
        throwOnError: false,
        output: 'html',
      })
    } catch {
      // 渲染失败时保留原样（转义后显示源码），不影响正文
      return esc(tex)
    }
  }
  // 逐段处理，避免 $$ 与 $ 相互干扰：先 display（$$…$$ 与 \[…\]），再 inline（$…$ 与 \(…\)）
  // 公式内不允许出现未转义 HTML（sanitize 后文本安全）。
  let out = html
  // $$ ... $$（跨行）
  out = out.replace(/\$\$([\s\S]+?)\$\$/g, (_m, tex: string) => render(tex, true))
  // \[ ... \]（display；文本中可能被 marked 转义为 \[，还原处理）
  out = out.replace(/\\\[([\s\S]+?)\\\]/g, (_m, tex: string) => render(tex, true))
  // $ ... $（inline，不跨行，避免匹配货币等单 $）
  out = out.replace(/(^|[^\\$])\$([^$\n]+?)\$(?!\d)/g, (_m, pre: string, tex: string) => pre + render(tex, false))
  // \( ... \)（inline）
  out = out.replace(/\\\(([^\\\n]+?)\\\)/g, (_m, tex: string) => render(tex, false))
  return out
}

// 是否疑似含公式（懒加载 KaTeX 的判定）。
function looksMath(text: string): boolean {
  return /\$|\\\(|\\\[/.test(text)
}

export function Markdown({ text, className }: { text: string; className?: string }) {
  const html = useMemo(() => renderMarkdown(text), [text])
  const needsMath = looksMath(html)
  const [mathHtml, setMathHtml] = useState<string | null>(null)

  // 懒加载 KaTeX 并做文本级公式替换（结果缓存在 state；组件重渲染/重挂只重放同一结果，
  // 无 DOM 侵入、幂等安全——修「切换语言后公式消失」：旧 auto-render 在重渲染竞态下
  // 可能对已渲染 DOM 二次处理或回调丢失）。
  useEffect(() => {
    if (!needsMath) {
      setMathHtml(null)
      return
    }
    let alive = true
    void import('katex').then((mod) => {
      if (!alive) return
      const m = mod as unknown as { default?: unknown; renderToString?: (tex: string, opts: Record<string, unknown>) => string }
      const renderToString = typeof m.renderToString === 'function' ? m.renderToString : (m.default as { renderToString: (tex: string, opts: Record<string, unknown>) => string }).renderToString
      setMathHtml(renderMathHTML(html, { renderToString }))
    }).catch(() => { /* 公式不可用则保留源码 */ })
    return () => {
      alive = false
    }
  }, [html, needsMath])

  const finalHtml = needsMath ? (mathHtml ?? html) : html
  return <div className={className} dangerouslySetInnerHTML={{ __html: finalHtml }} />
}
