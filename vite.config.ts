import { readFileSync } from 'node:fs'
import { createRequire } from 'node:module'

import { tanstackRouter } from '@tanstack/router-plugin/vite'
import react from '@vitejs/plugin-react'
import { playwright } from '@vitest/browser-playwright'
import type { Plugin } from 'vite'
import { configDefaults, defineConfig } from 'vitest/config'

const require = createRequire(import.meta.url)

const SUPPORTED_LOCALES = new Set(['en', 'ru', 'es'])

// Dev-only: substitute the {{APP_LOCALE}} sentinels in index.html the same way
// the Go backend does at serve time in prod (internal/server/spa.go). Applied
// only in `serve` so the `build` output keeps the raw sentinels for Go to
// replace at runtime. Drives off APP_LOCALE — the same env var the backend uses.
function devLocalePlugin(): Plugin {
  return {
    name: 'sphericon:dev-locale',
    apply: 'serve',
    transformIndexHtml(html) {
      const env = process.env.APP_LOCALE ?? 'en'
      const locale = SUPPORTED_LOCALES.has(env) ? env : 'en'
      // Only an explicit `false` turns the gate off, as in the backend's config.
      const verification = process.env.REQUIRE_EMAIL_VERIFICATION === 'false' ? 'false' : 'true'
      return html
        .replaceAll('{{APP_LOCALE}}', locale)
        .replaceAll('{{REQUIRE_EMAIL_VERIFICATION}}', verification)
    },
  }
}

// Test-only: serve MSW's service worker script at the origin root so the worker used by
// src/test/setup.tsx gets scope `/` (it must not be a committed or public/ file: it would
// ship in the production bundle). Applied only under Vitest.
function mswWorkerPlugin(): Plugin {
  return {
    name: 'sphericon:msw-worker',
    apply: () => Boolean(process.env.VITEST),
    configureServer(server) {
      server.middlewares.use('/mockServiceWorker.js', (_req, res) => {
        res.setHeader('content-type', 'text/javascript')
        res.end(readFileSync(require.resolve('msw/mockServiceWorker.js')))
      })
    },
  }
}

// The app is reached via Caddy at https://sphericon.localhost (a linked worktree has its own
// origin, see .mise.toml), which terminates TLS
// and is the single place that routes API paths (/site, /collect, /auth, /mcp, /oauth, /.well-known,
// /avatar, /api) to the Go backend. Vite serves only the SPA + HMR here.
export default defineConfig({
  // The router plugin generates src/routeTree.gen.ts from src/routes and splits each route's
  // component into its own chunk; it must run before the React plugin.
  plugins: [
    tanstackRouter({ target: 'react', autoCodeSplitting: true }),
    react(),
    devLocalePlugin(),
    mswWorkerPlugin(),
  ],
  // Under Vitest browser mode the page loads @vite/client, which would try to open
  // the dev HMR websocket below (wss://sphericon.localhost:443 — unreachable in CI) and
  // race the teardown with an unhandled "WebSocket closed without opened" rejection.
  // Disable HMR entirely during tests; keep the Caddy dev config for `make dev`.
  server: process.env.VITEST
    ? { hmr: false }
    : {
        host: true,
        // The mise `frontend` daemon exports FRONTEND_PORT: 5173 in the primary checkout, the
        // worktree's own slot in a linked one.
        port: Number(process.env.FRONTEND_PORT ?? 5173),
        strictPort: true,
        // Agent worktrees are full repo copies; watching them triggers spurious reloads.
        watch: { ignored: ['**/.claude/worktrees/**'] },
        // The primary checkout is sphericon.localhost, a linked worktree <dir>.sphericon.localhost.
        allowedHosts: ['.sphericon.localhost'],
        // HMR runs through Caddy's HTTPS origin, so the client connects over wss to the
        // stack's host and Caddy port (APP_HOST / CADDY_PORT, set in .mise.toml).
        hmr: {
          protocol: 'wss',
          host: process.env.APP_HOST ?? 'sphericon.localhost',
          clientPort: Number(process.env.CADDY_PORT ?? 443),
        },
      },
  test: {
    testTimeout: 10_000,
    // .cache holds the Go module cache, whose dependencies ship their own *.test.* files.
    exclude: [...configDefaults.exclude, '.cache/**', '.claude/**'],
    setupFiles: ['./src/test/setup.tsx'],
    coverage: {
      provider: 'v8',
      include: ['src/**', 'packages/analytics/src/**'],
      // The route files are declarations: the router plugin compiles each into lazy-loading
      // glue whose branches no test calls, and the guards in them delegate to src/guards.ts.
      exclude: [
        '**/generated/**',
        '**/*.test.*',
        'src/test/**',
        'src/main.tsx',
        'src/routes/**',
        'src/routeTree.gen.ts',
      ],
      reporter: ['text-summary', 'html'],
      // Current coverage is ~99% lines / ~93% branches; the floor sits a little below so
      // CI catches regressions without flaking on small changes.
      thresholds: { statements: 96, branches: 90, functions: 96, lines: 96 },
    },
    browser: {
      enabled: true,
      provider: playwright(),
      instances: [{ browser: 'chromium' }],
      headless: true, // the dev/CI container has no display
    },
  },
})
