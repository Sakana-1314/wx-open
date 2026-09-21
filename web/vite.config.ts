import vue from '@vitejs/plugin-vue'
import { fileURLToPath, URL } from 'node:url'
import { defineConfig, loadEnv } from 'vite'

// 构建/开发时把后端 HOST 注入为 __API_HOST__，供 src/api/config.ts 解析 API base。
// 取值优先级：进程环境变量 HOST > .env(.production) 中的 HOST；未配置则前端走相对 /api/v1。
export default defineConfig(({ mode }) => {
  const envDir = fileURLToPath(new URL('.', import.meta.url))
  const env = loadEnv(mode, envDir, '')
  const host = process.env.HOST?.trim() || env.HOST?.trim() || ''

  return {
    plugins: [vue()],
    resolve: {
      alias: {
        '@': fileURLToPath(new URL('./src', import.meta.url)),
      },
    },
    define: {
      __API_HOST__: JSON.stringify(host),
    },
    server: {
      host: true,
      port: 5174,
      proxy: {
        // 开发环境将 /api 代理到后端，避免跨域，图片等资源同源加载。
        '/api': {
          target: 'http://127.0.0.1:8091',
          changeOrigin: true,
        },
      },
    },
    preview: {
      host: true,
      port: 4174,
      proxy: {
        // 生产预览同样代理 /api（供本地/CI 端到端验证）。
        '/api': {
          target: 'http://127.0.0.1:8091',
          changeOrigin: true,
        },
      },
    },
  }
})