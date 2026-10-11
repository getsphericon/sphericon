// Global setup for Vitest Browser Mode tests. Loaded once per test file via
// `test.setupFiles` in vite.config.ts.
//
// - Mantine styles so rendered components look/behave like the real app.
// - i18n init so `t(($) => $.x)` translation keys resolve to English strings.
import '@mantine/core/styles.css'
import '@mantine/notifications/styles.css'
import '../i18n.ts'
import { afterEach, beforeAll } from 'vitest'

import { client as operatorClient } from '../generated/operator/client.gen.ts'
import { client } from '../generated/site/client.gen.ts'
import { worker } from './worker.ts'

// The app pins the generated client's base URL in src/main.tsx; mirror it here.
client.setConfig({ baseUrl: '/site' })
operatorClient.setConfig({ baseUrl: '/operator' })

// MSW answers a request without a handler with a 500 and only logs it, which the app may
// swallow. Record each one and fail the test that made it; handlers are dropped after every test.
const unhandled: string[] = []

function isApiFrame({ data }: { data: unknown }) {
  if (typeof data !== 'object' || data === null || !('request' in data)) return false
  const { request } = data
  if (!(request instanceof Request)) return false
  const { pathname } = new URL(request.url)
  return pathname.startsWith('/site/') || pathname.startsWith('/operator/')
}

beforeAll(() =>
  worker.start({
    quiet: true,
    onUnhandledFrame: async ({ frame, defaults }) => {
      // Page assets and the analytics package's own traffic are not the app's API.
      if (!isApiFrame(frame)) {
        frame.passthrough()
        return
      }
      unhandled.push(await frame.getUnhandledMessage())
      defaults.error()
    },
  }),
)
// A test may end before its own requests have reached the worker (the round-trip takes a beat).
// Give them that beat while the test's handlers are still installed, or they would be answered
// by the next test's handlers and reported as unmocked there.
afterEach(async () => {
  await new Promise((resolve) => setTimeout(resolve, 20))
  worker.resetHandlers()
  const messages = unhandled.splice(0)
  if (messages.length > 0) {
    throw new Error(`unmocked request:\n${messages.join('\n')}`)
  }
})

// The worker is never stopped: `worker.stop()` hangs while a never-resolving request (the
// loading-state tests) is pending, and the page teardown drops the worker anyway.
