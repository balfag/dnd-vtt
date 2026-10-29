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
    // popout.html is the blank page of a detached panel window (dockview fills it from the main window).
    rollupOptions: {
      input: {
        main: resolve(import.meta.dirname, 'index.html'),
        popout: resolve(import.meta.dirname, 'popout.html'),
      },
    },
  },
  server: {
    port: 5173,
    strictPort: true,
    // changeOrigin stays off: the server's CSRF check compares Origin with
    // Host, and rewriting Host to the Go port would make every POST fail.
    proxy: {
      '/api': { target: `http://${goServer}`, changeOrigin: false },
      '/assets': { target: `http://${goServer}`, changeOrigin: false },
      '/ws': { target: `ws://${goServer}`, ws: true, changeOrigin: false },
    },
  },
  test: {
    environment: 'node',
    include: ['src/**/*.test.ts'],
  },
})
