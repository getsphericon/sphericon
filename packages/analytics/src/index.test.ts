import { afterEach, beforeEach, expect, test, vi } from 'vitest'

type Tracker = typeof import('./index.ts')

const CONFIG = { collectKey: 'ck_test', baseUrl: 'https://collect.example/' }

// The tracker keeps its runtime in module state, so every test loads a fresh copy.
// Browser mode has no working vi.resetModules, so a unique query string forces a re-evaluation.
let loads = 0
async function loadTracker(): Promise<Tracker> {
  loads += 1
  const tracker: Tracker = await import(/* @vite-ignore */ `./index.ts?load=${loads}`)
  return tracker
}

function clearVisitorCookie() {
  document.cookie = 'om_vid=; Path=/; Max-Age=0'
}

type FetchStub = ReturnType<typeof vi.fn<typeof fetch>>

interface SentBody {
  visitorId: string
  email?: string | null
  phone?: string | null
  subjectId?: string | null
  traits?: unknown
  events: { visitorId: string; action: string; properties: unknown }[]
}

function sentBodies(stub: FetchStub, path: string): SentBody[] {
  return stub.mock.calls
    .filter(([url]) => typeof url === 'string' && url.endsWith(path))
    .map(([, init]) => {
      const body: SentBody = JSON.parse(typeof init?.body === 'string' ? init.body : '{}')
      return body
    })
}

function sentBody(stub: FetchStub, path: string): SentBody {
  const [body] = sentBodies(stub, path)
  if (!body) {
    throw new Error(`no request sent to ${path}`)
  }
  return body
}

// firstCall returns the URL and headers of the first fetch the tracker made.
function firstCall(stub: FetchStub) {
  const [url, init] = stub.mock.calls[0] ?? []
  return { url, headers: init?.headers }
}

let fetchStub: FetchStub

beforeEach(() => {
  vi.useFakeTimers()
  fetchStub = vi.fn<typeof fetch>(() => Promise.resolve(new Response(null, { status: 202 })))
  vi.stubGlobal('fetch', fetchStub)
  clearVisitorCookie()
  delete window._omq
})

afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

test('batches tracked events into one authenticated POST after the flush delay', async () => {
  const { initTracking, track } = await loadTracker()
  initTracking(CONFIG)

  track('signup', { plan: 'pro' })
  track('click')
  expect(fetchStub).not.toHaveBeenCalled()

  await vi.advanceTimersByTimeAsync(200)

  expect(fetchStub).toHaveBeenCalledTimes(1)
  const { url, headers } = firstCall(fetchStub)
  expect(url).toBe('https://collect.example/collect/events')
  expect(headers).toMatchObject({ 'x-collect-key': 'ck_test' })
  const { events } = sentBody(fetchStub, '/collect/events')
  expect(events.map((e) => e.action)).toEqual(['signup', 'click'])
  expect(events[0]?.properties).toEqual({ plan: 'pro' })
  expect(events[1]?.properties).toBeNull()
})

test('reuses the visitor id from the cookie and persists a new one', async () => {
  document.cookie = 'om_vid=existing-visitor; Path=/'
  const first = await loadTracker()
  first.initTracking(CONFIG)
  first.track('a')
  await vi.advanceTimersByTimeAsync(200)
  expect(sentBody(fetchStub, '/collect/events').events[0]?.visitorId).toBe('existing-visitor')

  clearVisitorCookie()
  const second = await loadTracker()
  second.initTracking(CONFIG)
  expect(document.cookie).toMatch(/om_vid=[^;]+/)
})

test('identify posts the visitor id with nulls for missing fields', async () => {
  const { initTracking, identify } = await loadTracker()
  initTracking(CONFIG)

  await identify({ email: 'a@example.com' })

  const body = sentBody(fetchStub, '/collect/identify')
  expect(body).toMatchObject({
    email: 'a@example.com',
    phone: null,
    subjectId: null,
    traits: null,
  })
  expect(body.visitorId).toEqual(expect.any(String))
})

test('identify swallows network failures', async () => {
  fetchStub.mockRejectedValue(new Error('offline'))
  const { initTracking, identify } = await loadTracker()
  initTracking(CONFIG)

  await expect(identify({ email: 'a@example.com' })).resolves.toBeUndefined()
})

test('events that failed to send are re-queued and retried on the next flush', async () => {
  fetchStub.mockRejectedValueOnce(new Error('offline'))
  const { initTracking, track } = await loadTracker()
  initTracking(CONFIG)

  track('first')
  await vi.advanceTimersByTimeAsync(200)
  track('second')
  await vi.advanceTimersByTimeAsync(200)

  const bodies = sentBodies(fetchStub, '/collect/events')
  expect(bodies).toHaveLength(2)
  expect(bodies[1]?.events.map((e) => e.action)).toEqual(['first', 'second'])
})

test('calls made before init are queued on window._omq and replayed by init', async () => {
  const { initTracking, track, identify } = await loadTracker()

  track('early', { a: 1 })
  await identify({ subjectId: 's1' })
  expect(window._omq).toHaveLength(2)

  initTracking(CONFIG)
  await vi.advanceTimersByTimeAsync(200)

  expect(sentBody(fetchStub, '/collect/identify').subjectId).toBe('s1')
  expect(sentBody(fetchStub, '/collect/events').events[0]?.action).toBe('early')
})

test('resolves the config from an init command in the async stub queue', async () => {
  window._omq = [
    ['init', { collectKey: '   ', baseUrl: '' }],
    ['init', { collectKey: ' ck_queue ', baseUrl: 'https://q.example' }],
    ['track', 'from-stub', undefined],
  ]
  const { initTracking } = await loadTracker()

  initTracking()
  await vi.advanceTimersByTimeAsync(200)

  const { url, headers } = firstCall(fetchStub)
  expect(url).toBe('https://q.example/collect/events')
  expect(headers).toMatchObject({ 'x-collect-key': 'ck_queue' })
})

test('commands pushed to window._omq after init are processed immediately', async () => {
  const { initTracking } = await loadTracker()
  initTracking(CONFIG)

  window._omq?.push(
    ['init', CONFIG],
    ['identify', { email: 'p@example.com' }],
    ['track', 'pushed', undefined],
  )
  await vi.advanceTimersByTimeAsync(200)

  expect(sentBody(fetchStub, '/collect/identify').email).toBe('p@example.com')
  expect(sentBody(fetchStub, '/collect/events').events[0]?.action).toBe('pushed')
})

test('init without a usable collect key does not start tracking', async () => {
  const { initTracking, track } = await loadTracker()

  initTracking({ collectKey: ' ', baseUrl: '' })
  track('ignored')
  await vi.advanceTimersByTimeAsync(200)

  expect(fetchStub).not.toHaveBeenCalled()
})

test('a second init is ignored', async () => {
  const { initTracking, track } = await loadTracker()
  initTracking(CONFIG)
  initTracking({ collectKey: 'other', baseUrl: 'https://other.example' })

  track('x')
  await vi.advanceTimersByTimeAsync(200)

  expect(firstCall(fetchStub).url).toBe('https://collect.example/collect/events')
})

test('pagehide flushes pending events without waiting for the timer', async () => {
  const { initTracking, track } = await loadTracker()
  initTracking(CONFIG)
  track('leaving')

  window.dispatchEvent(new Event('pagehide'))

  expect(fetchStub).toHaveBeenCalledTimes(1)
})

test('trackPageView sends a page.view event with the page details', async () => {
  const { initTracking, trackPageView } = await loadTracker()
  initTracking(CONFIG)
  const page = { path: '/p', url: 'https://x/p', title: 'P', referrer: '', locale: 'en' }

  trackPageView(page)
  await vi.advanceTimersByTimeAsync(200)

  const { events } = sentBody(fetchStub, '/collect/events')
  expect(events[0]).toMatchObject({ action: 'page.view', properties: page })
})

test('falls back to a generated visitor id when crypto.randomUUID is unavailable', async () => {
  vi.stubGlobal('crypto', {})
  const { initTracking, track } = await loadTracker()
  initTracking(CONFIG)
  track('a')
  await vi.advanceTimersByTimeAsync(200)

  const visitorId = sentBody(fetchStub, '/collect/events').events[0]?.visitorId
  expect(visitorId).toMatch(/^vid_[a-z0-9]+$/)
  expect(document.cookie).toContain(`om_vid=${visitorId}`)
})

test('an empty visitor cookie is replaced by a new id', async () => {
  document.cookie = 'om_vid=; Path=/'
  const { initTracking, track } = await loadTracker()
  initTracking(CONFIG)
  track('a')
  await vi.advanceTimersByTimeAsync(200)

  expect(sentBody(fetchStub, '/collect/events').events[0]?.visitorId).toMatch(/.+/)
})

test('an untyped snippet queue may omit the config base url and the identify payload', async () => {
  // The inline snippet is plain JS, so its queue is not checked against the command types.
  window._omq = JSON.parse('[["init", {"collectKey": "ck_snippet"}], ["identify"]]')
  const { initTracking } = await loadTracker()

  initTracking()
  await vi.advanceTimersByTimeAsync(200)

  const { url } = firstCall(fetchStub)
  expect(url).toBe('/collect/identify')
  expect(sentBody(fetchStub, '/collect/identify')).toMatchObject({ email: null, traits: null })
})
