import fs from 'node:fs'
import path from 'node:path'
import { defineConfig, loadEnv } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '')
  const target = env.VITE_API_PROXY_TARGET || 'http://127.0.0.1:8900'
  return {
    plugins: [
      react(),
      {
        name: 'exclude-retired-browser-runtimes',
        closeBundle() {
          // Browser-player runtimes are historical assets without artifact-level
          // license/integrity records. Keep them out of the production WebUI;
          // MGA-99 removes the retired source surface completely.
          fs.rmSync(path.resolve(__dirname, 'dist/runtimes'), { recursive: true, force: true })
        },
      },
    ],
    resolve: {
      alias: {
        '@': path.resolve(__dirname, 'src'),
      },
    },
    server: {
      port: 5173,
      strictPort: true,
      proxy: {
        '/api': { target, changeOrigin: true },
        '/health': { target, changeOrigin: true },
      },
    },
    build: {
      outDir: 'dist',
      emptyOutDir: true,
    },
  }
})
