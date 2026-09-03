// 代码高亮（Shiki）：题解等场景的代码块渲染。
// 动态 import('shiki') 使高亮引擎按需加载（语言由 shiki 内部自动分包），不拖累主包。
import { useEffect, useState } from 'react'
import { cn } from '@/lib/utils'

// 题解语言 → Shiki 语言映射（turtle 按 python 高亮）。
const LANG_ALIAS: Record<string, string> = {
  cpp: 'cpp',
  'c++': 'cpp',
  c: 'c',
  python: 'python',
  python3: 'python',
  py: 'python',
  go: 'go',
  golang: 'go',
  turtle: 'python',
}

export function resolveHighlightLang(language?: string): string {
  if (!language) return 'text'
  return LANG_ALIAS[language.trim().toLowerCase()] ?? 'text'
}

// CodeBlock 用 Shiki 高亮代码（异步加载引擎，加载期间先显示纯文本兜底）。
export function CodeBlock({ code, language, className }: { code: string; language?: string; className?: string }) {
  const lang = resolveHighlightLang(language)
  const [html, setHtml] = useState<string | null>(null)

  useEffect(() => {
    let alive = true
    setHtml(null)
    void import('shiki')
      .then(({ codeToHtml }) =>
        codeToHtml(code ?? '', {
          lang,
          theme: 'github-light',
        }),
      )
      .then((h) => {
        if (alive) setHtml(h)
      })
      .catch(() => {
        if (alive) setHtml(null) // 语言加载失败等：退回纯文本
      })
    return () => {
      alive = false
    }
  }, [code, lang])

  if (html) {
    return (
      <div
        className={cn('overflow-x-auto rounded-lg text-xs leading-relaxed [&_pre]:m-0 [&_pre]:bg-transparent [&_pre]:p-3', className)}
        dangerouslySetInnerHTML={{ __html: html }}
      />
    )
  }
  return (
    <pre className={cn('overflow-x-auto rounded-lg bg-muted p-3 text-xs leading-relaxed', className)}>
      <code>{code}</code>
    </pre>
  )
}
