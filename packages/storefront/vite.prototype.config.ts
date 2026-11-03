// PROTOTYPE (throwaway, ticket #958): the on-site form builder and the shop, standalone on
// http://localhost:5199 with no dev stack, no tracker and no storefront server.
import { fileURLToPath } from 'node:url'

import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

export default defineConfig({
  root: fileURLToPath(new URL('./prototype-onsite', import.meta.url)),
  plugins: [react()],
  server: {
    port: 5199,
    strictPort: true,
    fs: { allow: [fileURLToPath(new URL('..', import.meta.url)), fileURLToPath(new URL('../..', import.meta.url))] },
    watch: { ignored: ['**/.claude/worktrees/**/node_modules/**'] },
  },
})
