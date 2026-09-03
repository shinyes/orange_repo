// 代码高亮（Shiki）：测评记录等场景的提交代码渲染。
// 动态 import('shiki') 使高亮引擎按需加载（oniguruma wasm 由 shiki 内联，语言自动分包），不拖累主包。
// 首次加载/引擎初始化可能较慢：加载期间显示纯文本 + “高亮加载中”标记，避免被误认为未高亮。
import { useEffect, useState } from 'react'
import { cn } from '@/lib/utils'

// 语言 → Shiki 语言映射。
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

// CodeBlock 用 Shiki 高亮代码（异步加载引擎，加载期间显示纯文本兜底并标注状态）。
export function CodeBlock({ code, language, className }: { code: string; language?: string; className?: string }) {
  const lang = resolveHighlightLang(language)
  const [html, setHtml] = useState<string | null>(null)
  const [failed, setFailed] = useState(false)

  useEffect(() => {
    let alive = true
    setHtml(null)
    setFailed(false)
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
        if (alive) setFailed(true) // 引擎加载失败：保持纯文本
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
    <div className={cn('relative', className)}>
      <pre className="overflow-x-auto rounded-lg bg-muted p-3 text-xs leading-relaxed">
        <code>{code}</code>
      </pre>
      {!failed && (
        <span className="pointer-events-none absolute right-2 top-2 rounded bg-background/80 px-1.5 py-0.5 text-[10px] text-muted-foreground">
          高亮加载中…
        </span>
      )}
    </div>
  )
}
