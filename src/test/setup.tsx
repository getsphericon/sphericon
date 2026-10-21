// Global setup for Vitest Browser Mode tests. Loaded once per test file via
// `test.setupFiles` in vite.config.ts.
//
// - Mantine styles so rendered components look/behave like the real app.
// - i18n init so `t(($) => $.x)` translation keys resolve to English strings.
import '@mantine/core/styles.css'
import '@mantine/notifications/styles.css'
import '../i18n.ts'
import { QueryClient } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { afterEach, beforeAll, beforeEach } from 'vitest'

import { client } from '../generated/site/client.gen.ts'
import {
  handleSiteAuthSigninProviders,
  handleSiteBrandKitGet,
  handleSiteIdentitiesList,
  handleSitePasskeyLoginBegin,
  handleSitePasskeysList,
} from '../generated/site/msw.gen.ts'
import { client as staffClient } from '../generated/staff/client.gen.ts'
import { createAppRouter } from '../router.ts'
import { UNSAVED_BRAND_KIT } from './payloads.ts'
import { problem } from './problem.ts'
import { settled } from './queryClients.ts'
import { worker } from './worker.ts'

// The app pins the generated client's base URL in src/main.tsx; mirror it here.
client.setConfig({ baseUrl: '/site' })
staffClient.setConfig({ baseUrl: '/staff' })

// A route object gets its `to`, `fullPath` and parent when a router is built over the generated
// tree (the app builds one in src/main.tsx); tests read them off the routes they import.
createAppRouter(new QueryClient())

// MSW answers a request without a handler with a 500 and only logs it, which the app may
// swallow. Record each one and fail the test that made it; handlers are dropped after every test.
const unhandled: string[] = []
const recording: Promise<void>[] = []

async function recordUnhandled(frame: { getUnhandledMessage: () => Promise<string> }) {
  unhandled.push(await frame.getUnhandledMessage())
}

function isApiFrame({ data }: { data: unknown }) {
  if (typeof data !== 'object' || data === null || !('request' in data)) {
    return false
  }
  const { request } = data
  if (!(request instanceof Request)) {
    return false
  }
  const { pathname } = new URL(request.url)
  return pathname.startsWith('/site/') || pathname.startsWith('/staff/')
}

beforeAll(() =>
  worker.start({
    quiet: true,
    onUnhandledFrame: ({ frame, defaults }) => {
      // Page assets and the analytics package's own traffic are not the app's API.
      if (!isApiFrame(frame)) {
        frame.passthrough()
        return
      }
      recording.push(recordUnhandled(frame))
      defaults.error()
    },
  }),
)
// Every page with a message editor reads the Workspace's Brand kit; a test that is about the kit
// installs its own handler over this one.
beforeEach(() => {
  worker.use(handleSiteBrandKitGet({ body: UNSAVED_BRAND_KIT }))
  // The login and register pages ask which Sign-in providers are enabled; by default none are.
  worker.use(handleSiteAuthSigninProviders({ body: { items: [] } }))
  // The account security page lists the User's Linked identities; by default there are none.
  worker.use(handleSiteIdentitiesList({ body: { items: [] } }))
  // ...and their Passkeys; by default none, and a password to prove with.
  worker.use(
    handleSitePasskeysList({
      body: { items: [], available: true, passwordSet: true, identityProofFresh: false },
    }),
  )
  // The login page starts a Passkey ceremony in the background when the browser can autofill
  // one; by default the Instance offers none, so that begin is refused and the page stays quiet.
  worker.use(handleSitePasskeyLoginBegin(() => problem(404)))
})
// A test may end before its own requests have reached the worker. The worker takes requests in
// the order the page made them, so a request answered by a handler of its own has passed every
// earlier one: wait for that, then for the unmocked ones to be recorded, while the test's handlers
// are still installed (or they would be answered by the next test's handlers and reported there).
const FLUSH_PATH = '/__flush'
afterEach(async () => {
  await settled()
  worker.use(http.get(FLUSH_PATH, () => new HttpResponse(null)))
  await fetch(FLUSH_PATH)
  await Promise.all(recording.splice(0))
  worker.resetHandlers()
  const messages = unhandled.splice(0)
  if (messages.length > 0) {
    throw new Error(`unmocked request:\n${messages.join('\n')}`)
  }
})

// The worker is never stopped: `worker.stop()` hangs while a never-resolving request (the
// loading-state tests) is pending, and the page teardown drops the worker anyway.
