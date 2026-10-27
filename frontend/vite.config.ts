import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

const backend = process.env.MEDIA_HUB_DEV_API_URL || 'http://127.0.0.1:8080'
export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    port: 5173,
    strictPort: true,
    proxy: Object.fromEntries(['/api', '/health', '/p'].map((prefix) => [
      prefix, { target: backend, changeOrigin: true },
    ])),
  },
  test: { environment: 'node', include: ['src/**/*.test.{ts,tsx}'] },
})
