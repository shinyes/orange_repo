import path from 'node:path'
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

// 单前端应用（门户 + 未来 /admin 管理区并入）。
// 与后端单进程 internal/app 对接：/api 代理到合服端口 :8080（单 cookie orange_session）。
// https://vite.dev/config/
export default defineConfig({
  plugins: [react(), tailwindcss()],
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
