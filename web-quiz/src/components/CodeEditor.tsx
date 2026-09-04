// 编程题代码编辑器：Monaco Editor（本地资源加载，见 monaco-setup.ts）。
import Editor from '@monaco-editor/react'
import { useRef } from 'react'
import type { CodeLang } from '@/lib/types'
import { setupMonaco } from '@/lib/monaco-setup'

// Monaco 语言标识（cpp/python）。
const LANG_MAP: Record<CodeLang, string> = { cpp: 'cpp', python: 'python' }

export function CodeEditor({
  language,
  value,
  onChange,
}: {
  language: CodeLang
  value: string
  onChange: (v: string) => void
}) {
  const onChangeRef = useRef(onChange)
  onChangeRef.current = onChange

  // 确保本地 monaco 配置就绪（幂等，早于 Editor 实例化）
  setupMonaco()

  return (
    <Editor
      height="100%"
      language={LANG_MAP[language] ?? 'plaintext'}
      value={value}
      theme="vs"
      onChange={(v) => onChangeRef.current(v ?? '')}
      options={{
        minimap: { enabled: false },
        fontSize: 14,
        lineHeight: 22,
        tabSize: 4,
        insertSpaces: true,
        automaticLayout: true, // 容器尺寸变化自动重排
        scrollBeyondLastLine: false,
        wordWrap: 'off',
        renderLineHighlight: 'line',
        padding: { top: 8 },
        scrollbar: { verticalScrollbarSize: 10, horizontalScrollbarSize: 10 },
        contextmenu: true,
      }}
      loading={<div className="p-4 text-xs text-muted-foreground">编辑器加载中…</div>}
    />
  )
}
