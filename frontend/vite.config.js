import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// In dev, the app runs on :5173 and proxies API calls to the Go server on :8080,
// so the browser sees a single origin and there's no CORS. In prod the Go server
// serves the built dist/ directly.
export default defineConfig({
  plugins: [vue()],
  server: {
    proxy: {
      '/api': 'http://localhost:8080',
    },
  },
})
