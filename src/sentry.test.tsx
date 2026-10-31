import { CatchBoundary } from '@tanstack/react-router'
import { createRoot } from 'react-dom/client'
import { afterEach, expect, test, vi } from 'vitest'

import { sentryDsn } from './instance.ts'
import { startReporting } from './sentry.ts'

const sentry = vi.hoisted(() => ({
  init: vi.fn<(options: unknown) => void>(),
  captured: [] as unknown[],
}))

vi.mock('@sentry/react', () => ({
  init: sentry.init,
  reactErrorHandler: () => (error: unknown) => {
    sentry.captured.push(error)
  },
}))

afterEach(() => {
  sentry.init.mockClear()
  sentry.captured.length = 0
  Reflect.deleteProperty(window, '__SENTRY_DSN__')
})

test('reporting is off unless the page carries a DSN', () => {
  expect(sentryDsn()).toBe('')

  Reflect.set(window, '__SENTRY_DSN__', '  ')
  expect(sentryDsn()).toBe('')

  Reflect.set(window, '__SENTRY_DSN__', 42)
  expect(sentryDsn()).toBe('')

  Reflect.set(window, '__SENTRY_DSN__', ' https://key@o1.ingest.sentry.io/2 ')
  expect(sentryDsn()).toBe('https://key@o1.ingest.sentry.io/2')
})

test('startReporting initialises the SDK with the DSN', () => {
  startReporting('https://key@o1.ingest.sentry.io/2')

  expect(sentry.init).toHaveBeenCalledExactlyOnceWith({ dsn: 'https://key@o1.ingest.sentry.io/2' })
})

function Broken(): never {
  throw new Error('render failed')
}

// A render error a boundary catches never reaches window.onerror: only the root's
// onCaughtError sees it, and TanStack Router wraps every route in such a boundary.
test('a render error caught by a route boundary is reported through the root options', async () => {
  const container = document.createElement('div')
  document.body.append(container)
  const root = createRoot(container, startReporting('https://key@o1.ingest.sentry.io/2'))

  root.render(
    <CatchBoundary getResetKey={() => 'key'} errorComponent={() => 'fallback'}>
      <Broken />
    </CatchBoundary>,
  )

  await expect.poll(() => container.textContent).toBe('fallback')
  expect(sentry.captured).toEqual([new Error('render failed')])

  root.unmount()
  container.remove()
})
