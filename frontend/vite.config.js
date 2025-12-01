import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  server: {
    port: 8006,
    proxy: {
      '/api': {
        target: 'http://192.168.100.18:8005',
        //  target: 'http://127.0.0.1:8005',
        changeOrigin: true
      }
    }
  },
  build: {
    outDir: 'dist',
    assetsDir: 'assets'
  }
})

