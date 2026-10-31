/// <reference types="vitest" />
import { defineConfig } from 'vitest/config'
import { readFileSync } from 'fs'
import { resolve } from 'path'
import { fileURLToPath } from 'url'
import react from '@vitejs/plugin-react'

function getVersion(): string {
  if (process.env.VITE_APP_VERSION) return process.env.VITE_APP_VERSION
  try {
    return readFileSync(resolve(fileURLToPath(new URL('.', import.meta.url)), '../VERSION'), 'utf-8').trim()
  } catch {
    return 'dev'
  }
}

export default defineConfig({
  plugins: [react()],
  base: './',
  define: {
    __APP_VERSION__: JSON.stringify(getVersion()),
  },
  server: {
    port: 3000,
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
      '/health': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
  build: {
    target: 'es2020',
    outDir: 'dist',
    sourcemap: false,
  },
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./src/test/setup.ts'],
    coverage: {
      provider: 'v8',
      reporter: ['text', 'text-summary'],
      include: ['src/**/*.{ts,tsx}'],
      exclude: ['src/main.tsx', 'src/vite-env.d.ts', 'src/test/**', 'src/**/*.test.{ts,tsx}'],
    },
  },
})
