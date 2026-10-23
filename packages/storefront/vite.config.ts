import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

// The storefront is a customer site: it runs on its own origin (brightpath.localhost, or
// brightpath.<dir>.sphericon.localhost in a linked worktree), behind Caddy, which terminates TLS.
// The mise `demo` daemon exports DEMO_PORT, DEMO_HOST and CADDY_PORT (.mise.toml); the tracker's
// collect key and base URL arrive as DEMO_COLLECT_KEY and DEMO_COLLECT_URL.
export default defineConfig({
  plugins: [react()],
  envPrefix: 'DEMO_COLLECT_',
  server: {
    host: true,
    port: Number(process.env.DEMO_PORT ?? 5373),
    strictPort: true,
    // Agent worktrees are full repo copies; watching them triggers spurious reloads.
    watch: { ignored: ['**/.claude/worktrees/**'] },
    allowedHosts: ['.localhost'],
    hmr: {
      protocol: 'wss',
      host: process.env.DEMO_HOST ?? 'brightpath.localhost',
      clientPort: Number(process.env.CADDY_PORT ?? 443),
    },
  },
})
