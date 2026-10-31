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
  events: { visitorId: string; action: string; properties: unknown; clientEventId?: string }[]
}

// Every request the tracker made, copied as fetch got it: the client hands fetch a Request, and
// ky reads its body, so the copy is what a test reads.
let sentRequests: Request[]

async function sentBodies(path: string): Promise<SentBody[]> {
  return Promise.all(
    sentRequests
      .filter((request) => request.url.endsWith(path))
      .map(async (request) => {
        const body: SentBody = JSON.parse((await request.clone().text()) || '{}')
        return body
      }),
  )
}

async function sentBody(path: string): Promise<SentBody> {
  const [body] = await sentBodies(path)
  if (!body) {
    throw new Error(`no request sent to ${path}`)
  }
  return body
}

// firstCall returns the URL and the collect key of the first request the tracker made.
function firstCall() {
  const [request] = sentRequests
  return { url: request?.url, collectKey: request?.headers.get('x-collect-key') }
}

let fetchStub: FetchStub

beforeEach(() => {
  vi.useFakeTimers()
  fetchStub = vi.fn<typeof fetch>(async () => Promise.resolve(new Response(null, { status: 204 })))
  sentRequests = []
  vi.stubGlobal('fetch', async (input: RequestInfo | URL, init?: RequestInit) => {
    sentRequests.push(new Request(input, init).clone())
    return fetchStub(input, init)
  })
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
  const { url, collectKey } = firstCall()
  expect(url).toBe('https://collect.example/collect/events')
  expect(collectKey).toBe('ck_test')
  const { events } = await sentBody('/collect/events')
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
  expect((await sentBody('/collect/events')).events[0]?.visitorId).toBe('existing-visitor')

  clearVisitorCookie()
  const second = await loadTracker()
  second.initTracking(CONFIG)
  expect(document.cookie).toMatch(/om_vid=[^;]+/)
})

test('identify posts the visitor id with nulls for missing fields', async () => {
  const { initTracking, identify } = await loadTracker()
  initTracking(CONFIG)

  await identify({ email: 'a@example.com' })

  const body = await sentBody('/collect/identify')
  expect(body).toMatchObject({
    email: 'a@example.com',
    phone: null,
    subjectId: null,
    traits: null,
  })
  expect(body.visitorId).toEqual(expect.any(String))
})

test('identify swallows network failures', async () => {
  fetchStub.mockRejectedValue(new TypeError('Failed to fetch'))
  const { initTracking, identify } = await loadTracker()
  initTracking(CONFIG)

  await expect(identify({ email: 'a@example.com' })).resolves.toBeUndefined()
})

const QUEUE_KEY = 'om_q'

interface Sent {
  clientEventId?: string
  action: string
}

async function sentEvents(): Promise<Sent[][]> {
  return (await sentBodies('/collect/events')).map((b) => b.events)
}

function storedQueue(): Sent[] {
  const raw: Sent[] = JSON.parse(localStorage.getItem(QUEUE_KEY) ?? '[]')
  return raw
}

beforeEach(() => {
  localStorage.clear()
})

test('every event carries a clientEventId, generated once and reused on a retry', async () => {
  fetchStub.mockRejectedValueOnce(new TypeError('Failed to fetch'))
  const { initTracking, track } = await loadTracker()
  initTracking(CONFIG)

  track('first')
  await vi.advanceTimersByTimeAsync(200)
  await vi.advanceTimersByTimeAsync(1000)

  const [first, retry] = await sentEvents()
  expect(first?.[0]?.clientEventId).toEqual(expect.any(String))
  expect(retry?.[0]?.clientEventId).toBe(first?.[0]?.clientEventId)
})

test('an event that failed to send stays in the browser queue until it is accepted', async () => {
  fetchStub.mockRejectedValueOnce(new TypeError('Failed to fetch'))
  const { initTracking, track } = await loadTracker()
  initTracking(CONFIG)

  track('first')
  await vi.advanceTimersByTimeAsync(200)
  expect(storedQueue().map((e) => e.action)).toEqual(['first'])

  await vi.advanceTimersByTimeAsync(1000)
  expect(storedQueue()).toEqual([])
})

test('429 and 5xx are retried, honouring Retry-After', async () => {
  fetchStub.mockResolvedValueOnce(
    new Response(null, { status: 503, headers: { 'retry-after': '5' } }),
  )
  const { initTracking, track } = await loadTracker()
  initTracking(CONFIG)

  track('first')
  await vi.advanceTimersByTimeAsync(200)
  await vi.advanceTimersByTimeAsync(4000)
  expect(await sentEvents()).toHaveLength(1)

  await vi.advanceTimersByTimeAsync(1000)
  expect(await sentEvents()).toHaveLength(2)
  expect(storedQueue()).toEqual([])
})

test('the backoff grows while the instance keeps failing', async () => {
  fetchStub.mockResolvedValue(new Response(null, { status: 500 }))
  const { initTracking, track } = await loadTracker()
  initTracking(CONFIG)

  track('first')
  await vi.advanceTimersByTimeAsync(200)
  await vi.advanceTimersByTimeAsync(1000)
  expect(await sentEvents()).toHaveLength(2)

  await vi.advanceTimersByTimeAsync(1999)
  expect(await sentEvents()).toHaveLength(2)
  await vi.advanceTimersByTimeAsync(1)
  expect(await sentEvents()).toHaveLength(3)
})

test('a 4xx other than 429 drops the batch instead of retrying it', async () => {
  fetchStub.mockResolvedValueOnce(new Response(null, { status: 400 }))
  const { initTracking, track } = await loadTracker()
  initTracking(CONFIG)

  track('bad')
  await vi.advanceTimersByTimeAsync(200)
  await vi.advanceTimersByTimeAsync(60_000)

  expect(await sentEvents()).toHaveLength(1)
  expect(storedQueue()).toEqual([])
})

test('events left in the browser queue are sent on the next page load', async () => {
  localStorage.setItem(
    QUEUE_KEY,
    JSON.stringify([
      {
        visitorId: 'v',
        action: 'left over',
        properties: null,
        occurredAt: '2026-01-01T00:00:00.000Z',
        clientEventId: 'id-1',
      },
    ]),
  )
  const { initTracking } = await loadTracker()
  initTracking(CONFIG)
  await vi.advanceTimersByTimeAsync(200)

  expect(await sentEvents()).toEqual([
    [expect.objectContaining({ action: 'left over', clientEventId: 'id-1' })],
  ])
  expect(storedQueue()).toEqual([])
})

test('the queue never grows past its bound and drops the oldest first', async () => {
  fetchStub.mockResolvedValue(new Response(null, { status: 503 }))
  const { initTracking, track } = await loadTracker()
  initTracking(CONFIG)

  for (let i = 0; i < 150; i += 1) {
    track(`e${i}`)
  }
  await vi.advanceTimersByTimeAsync(200)

  const actions = storedQueue().map((e) => e.action)
  expect(actions).toHaveLength(100)
  expect(actions[0]).toBe('e50')
  expect(actions.at(-1)).toBe('e149')
})

test('a corrupt stored queue is ignored', async () => {
  localStorage.setItem(QUEUE_KEY, '{not json')
  const { initTracking, track } = await loadTracker()
  initTracking(CONFIG)

  track('ok')
  await vi.advanceTimersByTimeAsync(200)

  expect(await sentEvents()).toEqual([[expect.objectContaining({ action: 'ok' })]])
})

test('calls made before init are queued on window._omq and replayed by init', async () => {
  const { initTracking, track, identify } = await loadTracker()

  track('early', { a: 1 })
  await identify({ subjectId: 's1' })
  expect(window._omq).toHaveLength(2)

  initTracking(CONFIG)
  await vi.advanceTimersByTimeAsync(200)

  expect((await sentBody('/collect/identify')).subjectId).toBe('s1')
  expect((await sentBody('/collect/events')).events[0]?.action).toBe('early')
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

  const { url, collectKey } = firstCall()
  expect(url).toBe('https://q.example/collect/events')
  expect(collectKey).toBe('ck_queue')
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

  expect((await sentBody('/collect/identify')).email).toBe('p@example.com')
  expect((await sentBody('/collect/events')).events[0]?.action).toBe('pushed')
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

  expect(firstCall().url).toBe('https://collect.example/collect/events')
})

test('pagehide flushes pending events without waiting for the timer', async () => {
  const { initTracking, track } = await loadTracker()
  initTracking(CONFIG)
  track('leaving')

  window.dispatchEvent(new Event('pagehide'))
  await vi.advanceTimersByTimeAsync(0)

  // Trackers loaded by earlier tests keep their pagehide listeners, so look for this event only.
  const sent = (await sentEvents()).flat().filter((e) => e.action === 'leaving')
  expect(sent).toHaveLength(1)
})

test('trackPageView sends a page.view event with the page details', async () => {
  const { initTracking, trackPageView } = await loadTracker()
  initTracking(CONFIG)
  const page = { path: '/p', url: 'https://x/p', title: 'P', referrer: '', locale: 'en' }

  trackPageView(page)
  await vi.advanceTimersByTimeAsync(200)

  const { events } = await sentBody('/collect/events')
  expect(events[0]).toMatchObject({ action: 'page.view', properties: page })
})

test('a new visitor id is a UUID kept in the visitor cookie', async () => {
  const { initTracking, track } = await loadTracker()
  initTracking(CONFIG)
  track('a')
  await vi.advanceTimersByTimeAsync(200)

  const visitorId = (await sentBody('/collect/events')).events[0]?.visitorId
  expect(visitorId).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/)
  expect(document.cookie).toContain(`om_vid=${visitorId}`)
})

test('an empty visitor cookie is replaced by a new id', async () => {
  document.cookie = 'om_vid=; Path=/'
  const { initTracking, track } = await loadTracker()
  initTracking(CONFIG)
  track('a')
  await vi.advanceTimersByTimeAsync(200)

  expect((await sentBody('/collect/events')).events[0]?.visitorId).toMatch(/.+/)
})

test('an untyped snippet queue may omit the config base url and the identify payload', async () => {
  // The inline snippet is plain JS, so its queue is not checked against the command types.
  window._omq = JSON.parse('[["init", {"collectKey": "ck_snippet"}], ["identify"]]')
  const { initTracking } = await loadTracker()

  initTracking()
  await vi.advanceTimersByTimeAsync(200)

  const { url } = firstCall()
  expect(url).toBe(`${location.origin}/collect/identify`)
  expect(await sentBody('/collect/identify')).toMatchObject({ email: null, traits: null })
})
