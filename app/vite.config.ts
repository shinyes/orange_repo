import path from 'node:path'
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// 单前端应用（门户 + 未来 /admin 管理区并入）。
// 与后端单进程 internal/app 对接：/api 代理到合服端口 :8080（单 cookie orange_session）。
// https://vite.dev/config/
export default defineConfig({
  // Tailwind 由 PostCSS 承担（见 postcss.config.js + tailwind.config.js）。
  // 已移除 @tailwindcss/vite：Tailwind v4 会输出 oklch()/color-mix()，
  // 而目标环境 Chrome 109（Windows 7 上最后可用的 Chrome）无法解析。
  plugins: [react()],
  build: {
    // 兼容 Chrome 109：esbuild/lightningcss 不再向下降级到更新的语法。
    target: 'chrome109',
    cssTarget: 'chrome109',
  },
  resolve: {
    alias: {
      '@': path.resolve(import.meta.dirname, './src'),
    },
  },
  server: {
    host: true, // 局域网设备可访问（默认仅 localhost）
    port: 5175,
    proxy: {
      '/api': 'http://127.0.0.1:8080',
    },
  },
})
