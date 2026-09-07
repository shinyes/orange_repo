// 编程题代码编辑器：Monaco Editor（本地资源加载，见 monaco-setup.ts）。
// 支持字体缩放：Ctrl+鼠标滚轮（mouseWheelZoom 原生）、Alt+= / Alt+- / Alt+Z 放大/缩小/重置。
import Editor, { type OnMount } from '@monaco-editor/react'
import { useEffect, useRef } from 'react'
import type { CodeLang } from '@/api/types'
import { setupMonaco } from '@/lib/monaco-setup'

// Monaco 语言标识（cpp/python）。
const LANG_MAP: Record<CodeLang, string> = { cpp: 'cpp', python: 'python' }
const BASE_FONT = 14

export function CodeEditor({
  language,
  value,
  onChange,
  readOnly,
}: {
  language: CodeLang
  value: string
  onChange: (v: string) => void
  /** 只读回顾模式（不可编辑） */
  readOnly?: boolean
}) {
  const onChangeRef = useRef(onChange)
  onChangeRef.current = onChange
  const editorRef = useRef<{ editor: Parameters<OnMount>[0]; monaco: Parameters<OnMount>[1] } | null>(null)

  // 确保本地 monaco 配置就绪（幂等，早于 Editor 实例化）
  setupMonaco()

  // Alt+= / Alt+- / Alt+Z：触发 Monaco 内置 fontZoom 命令——与 Ctrl+滚轮(mouseWheelZoom)
  // 同走 EditorZoom 缩放系统，行为完全一致（每档字号 ×(1+zoom*0.1)，全局档位 -5..20）。
  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      const ctx = editorRef.current
      if (!ctx || !ctx.editor.hasTextFocus()) return
      if (!e.altKey || e.ctrlKey || e.metaKey) return
      const k = e.key
      const cmd =
        k === '=' || k === '+' ? 'editor.action.fontZoomIn'
          : k === '-' || k === '_' ? 'editor.action.fontZoomOut'
            : k === 'z' || k === 'Z' ? 'editor.action.fontZoomReset'
              : null
      if (cmd) {
        e.preventDefault()
        ctx.editor.trigger('keyboard', cmd, null)
      }
    }
    document.addEventListener('keydown', onKeyDown, true)
    return () => document.removeEventListener('keydown', onKeyDown, true)
  }, [])

  const handleMount: OnMount = (editor, monaco) => {
    editorRef.current = { editor, monaco }
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
        // 行号留白：左侧最小化（glyph 关 + 行号列宽收紧），右侧保留标准空白
        glyphMargin: false,
        folding: true,
        lineDecorationsWidth: 10,
        lineNumbersMinChars: 2,
        // Ctrl+鼠标滚轮 缩放字号（Monaco 原生）
        mouseWheelZoom: true,
        ...(readOnly ? { readOnly: true, domReadOnly: true } : {}),
      }}
      loading={<div className="p-4 text-xs text-muted-foreground">编辑器加载中…</div>}
    />
  )
}
