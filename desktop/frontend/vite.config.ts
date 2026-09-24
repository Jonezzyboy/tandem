import { fileURLToPath } from 'node:url'
import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'

// Keep in sync with compilerOptions.paths in tsconfig.json.
export default defineConfig({
  plugins: [svelte()],
  resolve: {
    alias: {
      '@lib': fileURLToPath(new URL('./src/lib', import.meta.url)),
      '@wailsjs': fileURLToPath(new URL('./wailsjs', import.meta.url)),
    },
  },
  build: { target: 'safari16', outDir: 'dist', emptyOutDir: true },
})
