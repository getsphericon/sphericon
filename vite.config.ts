import { readFileSync } from 'node:fs'
import { createRequire } from 'node:module'

import { tanstackRouter } from '@tanstack/router-plugin/vite'
import react from '@vitejs/plugin-react'
import { playwright } from '@vitest/browser-playwright'
import type { Plugin } from 'vite'
import { configDefaults, defineConfig } from 'vitest/config'

const require = createRequire(import.meta.url)

const SUPPORTED_LOCALES = new Set(['en'])

// Dev-only: substitute the {{APP_LOCALE}} sentinels in index.html the same way
// the Go backend does at serve time in prod (internal/server/spa/spa.go). Applied
// only in `serve` so the `build` output keeps the raw sentinels for Go to
// replace at runtime. Drives off SPHERICON_APP_LOCALE — the same env var the backend uses.
function devLocalePlugin(): Plugin {
  return {
    name: 'sphericon:dev-locale',
    apply: 'serve',
    transformIndexHtml(html) {
      const env = process.env.SPHERICON_APP_LOCALE ?? 'en'
      const locale = SUPPORTED_LOCALES.has(env) ? env : 'en'
      // The gate is on in the saas Mode only, as in the backend's config (ADR 0060).
      const verification = process.env.SPHERICON_MODE?.toLowerCase() === 'saas' ? 'true' : 'false'
      return html
        .replaceAll('{{APP_LOCALE}}', locale)
        .replaceAll('{{REQUIRE_EMAIL_VERIFICATION}}', verification)
        .replaceAll('{{SENTRY_DSN}}', '')
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
  // vitest-browser-react imports react-dom/client from inside node_modules, where Vite's first scan
  // does not see it: found late, it re-bundles the dependencies and reloads a test mid-run, and
  // that test's page never comes back. Pre-bundle it up front.
  // Every dependency a test can reach is optimized before the run: one Vite discovers mid-run
  // (it found @mantine/notifications through a logic test's imports) reloads the browser and
  // fails whatever test file was collecting.
  optimizeDeps: {
    include: [
      'react-dom/client',
      '@mantine/charts',
      '@mantine/core',
      '@mantine/form',
      '@mantine/hooks',
      '@mantine/modals',
      '@mantine/notifications',
      '@sphericon/analytics > es-toolkit',
      '@sphericon/analytics > js-cookie',
      '@sphericon/analytics > ky',
      '@sphericon/analytics > uuid',
      '@tiptap/react',
      'ahooks',
      'dayjs',
      'dayjs/plugin/localizedFormat.js',
      'dayjs/plugin/relativeTime.js',
      'react-error-boundary',
    ],
  },
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
        // Agent worktrees are full repo copies and `test:cover` rewrites coverage/ (HTML files
        // Vite treats as pages): watching either triggers spurious full reloads.
        watch: { ignored: ['**/.claude/worktrees/**', '**/coverage/**'] },
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
    // A UI test waits on what the page shows or did (expect.element, expect.poll), never on a
    // delay. A real Chromium on a busy CI runner can take more than a second to render, debounce
    // and answer, so a wait gives up after 5s (within the test's 10s), not the default 1s.
    expect: { poll: { timeout: 5_000 } },
    // .cache holds the Go module cache, whose dependencies ship their own *.test.* files.
    // typespec/lib's tests compile TypeSpec, which needs node: `mise run test:typespec` runs them.
    exclude: [...configDefaults.exclude, '.cache/**', '.claude/**', 'typespec/**'],
    setupFiles: ['./src/test/setup.tsx'],
    coverage: {
      provider: 'v8',
      include: ['src/**/*.{ts,tsx}', 'packages/analytics/src/**/*.ts'],
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
      // The floor is 90% on every metric, so CI catches regressions without flaking on
      // small changes.
      thresholds: { statements: 90, branches: 90, functions: 90, lines: 90 },
    },
    browser: {
      enabled: true,
      provider: playwright(),
      instances: [{ browser: 'chromium' }],
      headless: true, // the dev/CI container has no display
    },
  },
})
