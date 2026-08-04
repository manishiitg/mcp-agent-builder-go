import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import { fileURLToPath } from 'node:url'

const frontendRoot = fileURLToPath(new URL('..', import.meta.url))

export default defineConfig({
  plugins: [react()],
  server: {
    host: '127.0.0.1',
    port: 3200,
    strictPort: true,
    fs: { allow: [fileURLToPath(new URL('.', import.meta.url)), frontendRoot] },
  },
  preview: {
    host: '127.0.0.1',
    port: 3200,
    strictPort: true,
  },
  resolve: { dedupe: ['react', 'react-dom'] },
})
