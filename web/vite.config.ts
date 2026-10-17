/// <reference types="vitest/config" />
import { writeFileSync } from 'node:fs'
import { resolve } from 'node:path'
import react from '@vitejs/plugin-react'
import { defineConfig, type Plugin } from 'vite'

// web/embed.go embeds web/dist, and go:embed fails on a missing or empty
// directory. The placeholder is tracked in git; Vite wipes dist on every
// build, so it is written back after the bundle is closed.
function keepDistPlaceholder(): Plugin {
  return {
    name: 'dndvtt-keep-dist-placeholder',
    apply: 'build',
    closeBundle() {
      writeFileSync(resolve(import.meta.dirname, 'dist/.gitkeep'), '')
    },
  }
}

const goServer = 'localhost:7777'

export default defineConfig({
  plugins: [react(), keepDistPlaceholder()],
  build: {
    // "/assets/" is reserved for campaign files served by the Go server.
    assetsDir: 'static',
  },
  server: {
    port: 5173,
    strictPort: true,
    proxy: {
      '/api': `http://${goServer}`,
      '/assets': `http://${goServer}`,
      '/ws': { target: `ws://${goServer}`, ws: true },
    },
  },
  test: {
    environment: 'node',
    include: ['src/**/*.test.ts'],
  },
})
