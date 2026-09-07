// 编程题代码编辑器：Monaco Editor（本地资源加载，见 monaco-setup.ts）。
// 支持字体缩放：Ctrl+鼠标滚轮（mouseWheelZoom 原生）、Alt+= / Alt+- / Alt+Z 放大/缩小/重置。
import Editor, { type OnMount } from '@monaco-editor/react'
import { useRef } from 'react'
import type { CodeLang } from '@/api/types'
import { setupMonaco } from '@/lib/monaco-setup'

// Monaco 语言标识（cpp/python）。
const LANG_MAP: Record<CodeLang, string> = { cpp: 'cpp', python: 'python' }
const BASE_FONT = 14

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

  const handleMount: OnMount = (editor, monaco) => {
    const zoom = (delta: number) => {
      const cur = editor.getOption(monaco.editor.EditorOption.fontSize)
      const next = Math.min(32, Math.max(9, cur + delta))
      editor.updateOptions({ fontSize: next })
    }
    // Alt + = / - 缩放；Alt + Z 重置
    editor.addCommand(monaco.KeyMod.Alt | monaco.KeyCode.Equal, () => zoom(1))
    editor.addCommand(monaco.KeyMod.Alt | monaco.KeyCode.Minus, () => zoom(-1))
    editor.addCommand(monaco.KeyMod.Alt | monaco.KeyCode.KeyZ, () => editor.updateOptions({ fontSize: BASE_FONT }))
  }

  return (
    <Editor
      height="100%"
      language={LANG_MAP[language] ?? 'plaintext'}
      value={value}
      theme="vs"
      onMount={handleMount}
      onChange={(v) => onChangeRef.current(v ?? '')}
      options={{
        minimap: { enabled: false },
        fontSize: BASE_FONT,
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
        // 行号留白：左侧不留（无折叠箭头/装饰）；右侧保留 glyph 断点区
        glyphMargin: true,
        folding: false,
        lineDecorationsWidth: 0,
        // Ctrl+鼠标滚轮 缩放字号（Monaco 原生）
        mouseWheelZoom: true,
      }}
      loading={<div className="p-4 text-xs text-muted-foreground">编辑器加载中…</div>}
    />
  )
}
