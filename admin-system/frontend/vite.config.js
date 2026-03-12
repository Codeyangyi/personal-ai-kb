import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  server: {
    port: 8009,
    proxy: {
      '/api': {
        target: 'http://localhost:8007',
        changeOrigin: true
      },
      // 代理 personal-ai-kb 后端的上传文件（反馈图片）
      // personal-ai-kb 后端运行在 8005 端口
      '/uploads': {
        target: 'http://localhost:8005',
        changeOrigin: true
      }
    }
  }
})
