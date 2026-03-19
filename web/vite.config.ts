import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import packageJSON from './package.json'

const resolvedVersion = process.env.APP_VERSION?.trim() || `v${packageJSON.version}`

export default defineConfig({
  plugins: [vue()],
  define: {
    __APP_VERSION__: JSON.stringify(resolvedVersion),
  },
  server: {
    host: '127.0.0.1',
    port: 5173,
    strictPort: true,
    proxy: {
      '^/api(/|$)': {
        target: 'http://localhost:8081',
        changeOrigin: true,
      },
      '^/s(/|$)': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
})
