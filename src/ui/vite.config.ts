import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'
import fs from 'node:fs'
import path from "path"
import tailwindcss from "@tailwindcss/vite"

function readHomerAppVersion(): string {
  try {
    const versionGo = path.resolve(__dirname, '../version.go')
    const src = fs.readFileSync(versionGo, 'utf8')
    const m = src.match(/VERSION_APPLICATION\s*=\s*"([^"]+)"/)
    return m?.[1] ?? 'dev'
  } catch {
    return 'dev'
  }
}

// https://vite.dev/config/
export default defineConfig({
  define: {
    __HOMER_APP_VERSION__: JSON.stringify(readHomerAppVersion()),
  },
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "./src"),
    },
  },
  server: {
    port: 5173,
    proxy: {
      '/api': {
        target: 'http://de7.sipcapture.io:8081',
        changeOrigin: true,
      },
      // Doom widget IWAD — served by the coordinator from gamedata_dir,
      // never part of the UI bundle (see docs/VOIPGames.md).
      '/gamedata': {
        target: 'http://de7.sipcapture.io:8081',
        changeOrigin: true,
      },
    },
  },
  build: {
    outDir: '../dist',
    emptyOutDir: true,
  },
  test: {
    environment: 'jsdom',
    setupFiles: './src/test/setup.ts',
  },
})
