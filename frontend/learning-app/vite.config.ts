import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import { fileURLToPath } from 'node:url'

// The learning app is its own Vite root, but in platform mode it renders the
// same chat transcript AgentWorks and Video Studio do (../src/components), so
// the AgentWorks source tree must be reachable: the fs allowlist, the "@"
// alias its components use, and one copy of every store (zustand singletons
// must not be duplicated between node_modules trees).
const agentworksFrontend = fileURLToPath(new URL('..', import.meta.url))
const agentworksSrc = fileURLToPath(new URL('../src', import.meta.url))

export default defineConfig({
  plugins: [react()],
  server: {
    host: '127.0.0.1',
    port: 5174,
    strictPort: true,
    fs: { allow: [fileURLToPath(new URL('.', import.meta.url)), agentworksFrontend] },
  },
  resolve: {
    // The AgentWorks components must see the AgentWorks copies of the
    // packages they were written against (icons that a newer lucide has,
    // one zustand for the singleton stores), not the older copies in
    // learning-app/node_modules.
    alias: {
      '@': agentworksSrc,
      // A file, not a directory: a directory alias bypasses the package's
      // "exports" map, and subpaths like zustand/react/shallow then resolve
      // to nothing in the production build.
      'lucide-react': fileURLToPath(new URL('../node_modules/lucide-react/dist/esm/lucide-react.js', import.meta.url)),
    },
    // One copy of every store library across both node_modules trees.
    dedupe: ['react', 'react-dom', 'zustand', '@tanstack/react-query'],
  },
})
