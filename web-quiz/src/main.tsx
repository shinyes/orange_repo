import { Component, StrictMode, type ReactNode } from 'react'
import { createRoot } from 'react-dom/client'
import 'katex/dist/katex.min.css'
import './index.css'
import App from './App.tsx'

// 应用级错误边界：渲染期异常不再白屏，展示可恢复的错误页。
class AppErrorBoundary extends Component<{ children: ReactNode }, { error: Error | null }> {
  state: { error: Error | null } = { error: null }

  static getDerivedStateFromError(error: Error) {
    return { error }
  }

  render() {
    if (this.state.error) {
      return (
        <div className="flex h-dvh flex-col items-center justify-center gap-3 bg-background text-center">
          <img src="/favicon.png" alt="" className="size-14 rounded-2xl" />
          <div className="text-base font-medium">页面出了点问题</div>
          <div className="max-w-md px-6 text-xs text-muted-foreground">{String(this.state.error.message ?? this.state.error)}</div>
          <button
            type="button"
            className="mt-1 rounded-lg border border-input bg-background px-3 py-1.5 text-sm transition-colors hover:bg-muted"
            onClick={() => {
              this.setState({ error: null })
              window.location.reload()
            }}
          >
            重新加载
          </button>
        </div>
      )
    }
    return this.props.children
  }
}

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <AppErrorBoundary>
      <App />
    </AppErrorBoundary>
  </StrictMode>,
)