import type {
  CollectEventInput,
  CollectEventsInput,
  CollectIdentifyInput,
} from './generated/collect/types.gen.ts'

const VISITOR_COOKIE_NAME = 'om_vid'
const FLUSH_DELAY_MS = 200
const QUEUE_KEY = 'om_q'
// contract-copy: the tracker's offline queue size, client behaviour no API declares
const MAX_QUEUE = 100
const RETRY_BASE_MS = 1000
const RETRY_MAX_MS = 60_000

// SSR guard: the package is imported on the server (Next/Remix/TanStack Start).
// `typeof window` never throws when undefined, so this is safe at module load,
// and every exported entry point below short-circuits to a no-op on the server.
const isBrowser = typeof window !== 'undefined'

export interface TrackerConfig {
  collectKey: string
  baseUrl: string
}

// Public payload shapes are derived from the generated collect contract;
// visitorId is supplied internally, so consumers never pass it.
type IdentifyInput = Omit<CollectIdentifyInput, 'visitorId'>
type EventProperties = CollectEventInput['properties']

type QueueCommand =
  | ['init', Partial<TrackerConfig>]
  | ['identify', IdentifyInput?]
  | ['track', string, EventProperties | undefined]

type QueueApi = QueueCommand[] & {
  push: (...items: QueueCommand[]) => number
}

interface Runtime {
  config: TrackerConfig
  visitorId: string
  pendingEvents: CollectEventInput[]
  flushTimer: number | null
  retryTimer: number | null
  attempt: number
  sending: boolean
}

declare global {
  interface Window {
    _omq?: QueueCommand[]
  }
}

let runtime: Runtime | null = null

function normalizeConfig(config: Partial<TrackerConfig> | null | undefined): TrackerConfig | null {
  const collectKey = config?.collectKey?.trim()

  if (!collectKey) {
    return null
  }

  const baseUrl = (config?.baseUrl?.trim() || '').replace(/\/$/, '')

  return { collectKey, baseUrl }
}

function findConfigInQueue(commands: QueueCommand[]): TrackerConfig | null {
  for (const command of commands) {
    if (command[0] === 'init') {
      const normalized = normalizeConfig(command[1])

      if (normalized) {
        return normalized
      }
    }
  }

  return null
}

function randomVisitorId(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID()
  }

  return `vid_${Math.random().toString(36).slice(2)}${Date.now().toString(36)}`
}

function readCookie(name: string): string | null {
  const cookie = document.cookie.split('; ').find((part) => part.startsWith(`${name}=`))

  if (!cookie) {
    return null
  }

  const value = cookie.slice(name.length + 1)

  return value ? decodeURIComponent(value) : null
}

function writeCookie(name: string, value: string): void {
  // document.cookie on purpose: the Cookie Store API is async and not suitable here.
  document.cookie = `${name}=${encodeURIComponent(value)}; Path=/; Max-Age=${60 * 60 * 24 * 365}; SameSite=Lax`
}

function getOrCreateVisitorId(): string {
  const existing = readCookie(VISITOR_COOKIE_NAME)

  if (existing) {
    return existing
  }

  const created = randomVisitorId()
  writeCookie(VISITOR_COOKIE_NAME, created)

  return created
}

function buildHeaders(config: TrackerConfig): HeadersInit {
  return {
    'content-type': 'application/json',
    'x-collect-key': config.collectKey,
  }
}

function collectUrl(config: TrackerConfig, path: string): string {
  return `${config.baseUrl}${path}`
}

// Unsent events live in localStorage so a restart of the instance, or a page load, never loses them.
function readQueue(): CollectEventInput[] {
  try {
    const stored: CollectEventInput[] | null = JSON.parse(localStorage.getItem(QUEUE_KEY) ?? '[]')

    return Array.isArray(stored) ? stored.slice(-MAX_QUEUE) : []
  } catch {
    return []
  }
}

function saveQueue(events: CollectEventInput[]): void {
  try {
    localStorage.setItem(QUEUE_KEY, JSON.stringify(events))
  } catch {
    // storage is full or blocked: the in-memory queue still retries
  }
}

function flushSoon(): void {
  if (runtime?.flushTimer !== null || runtime.retryTimer !== null) {
    return
  }

  runtime.flushTimer = window.setTimeout(() => {
    void flushEvents()
  }, FLUSH_DELAY_MS)
}

function retryDelay(response: Response | null, attempt: number): number {
  const seconds = Number(response?.headers.get('retry-after'))

  if (seconds > 0) {
    return Math.min(seconds * 1000, RETRY_MAX_MS)
  }

  return Math.min(RETRY_BASE_MS * 2 ** attempt, RETRY_MAX_MS)
}

async function flushEvents(): Promise<void> {
  if (!runtime || runtime.sending) {
    return
  }

  window.clearTimeout(runtime.flushTimer ?? undefined)
  window.clearTimeout(runtime.retryTimer ?? undefined)
  runtime.flushTimer = null
  runtime.retryTimer = null

  if (runtime.pendingEvents.length === 0) {
    return
  }

  const state = runtime
  const events = state.pendingEvents.slice()
  const body: CollectEventsInput = { events }
  let retry = false
  let response: Response | null = null

  state.sending = true

  try {
    response = await fetch(collectUrl(state.config, '/collect/events'), {
      method: 'POST',
      headers: buildHeaders(state.config),
      body: JSON.stringify(body),
      keepalive: true,
    })
    retry = response.status === 429 || response.status >= 500
  } catch {
    retry = true
  }

  state.sending = false

  if (retry) {
    state.retryTimer = window.setTimeout(
      () => void flushEvents(),
      retryDelay(response, state.attempt),
    )
    state.attempt += 1
    return
  }

  state.attempt = 0
  state.pendingEvents = state.pendingEvents.filter((event) => !events.includes(event))
  saveQueue(state.pendingEvents)
  flushSoon()
}

function flushEventsOnPageHide(): void {
  if (!runtime || runtime.pendingEvents.length === 0) {
    return
  }

  void flushEvents()
}

function processQueueCommand(command: QueueCommand): void {
  if (command[0] === 'init') {
    // Config is resolved during initTracking; ignore late init commands.
    return
  }

  if (command[0] === 'identify') {
    void identify(command[1] ?? {})
    return
  }

  track(command[1], command[2] ?? null)
}

function createQueueApi(initial: QueueCommand[]): QueueApi {
  const push = (...items: QueueCommand[]) => {
    for (const item of items) {
      processQueueCommand(item)
    }

    return initial.length
  }

  return Object.assign(initial, { push })
}

export function initTracking(config?: TrackerConfig): void {
  if (!isBrowser || runtime) {
    return
  }

  const initialQueue = Array.isArray(window._omq) ? [...window._omq] : []

  window._omq = createQueueApi([])

  const resolved = normalizeConfig(config) ?? findConfigInQueue(initialQueue)

  if (!resolved) {
    return
  }

  runtime = {
    config: resolved,
    visitorId: getOrCreateVisitorId(),
    pendingEvents: readQueue(),
    flushTimer: null,
    retryTimer: null,
    attempt: 0,
    sending: false,
  }

  for (const command of initialQueue) {
    processQueueCommand(command)
  }

  window.addEventListener('pagehide', flushEventsOnPageHide)
  flushSoon()
}

export async function identify(payload: IdentifyInput): Promise<void> {
  if (!runtime) {
    if (isBrowser) {
      const queue = (window._omq ??= [])
      Array.prototype.push.call(queue, ['identify', payload])
    }

    return
  }

  const body: CollectIdentifyInput = {
    visitorId: runtime.visitorId,
    email: payload.email ?? null,
    phone: payload.phone ?? null,
    subjectId: payload.subjectId ?? null,
    traits: payload.traits ?? null,
  }

  await fetch(collectUrl(runtime.config, '/collect/identify'), {
    method: 'POST',
    headers: buildHeaders(runtime.config),
    body: JSON.stringify(body),
    keepalive: true,
  }).catch(() => undefined)
}

export function track(action: string, properties?: EventProperties): void {
  if (!runtime) {
    if (isBrowser) {
      const queue = (window._omq ??= [])
      Array.prototype.push.call(queue, ['track', action, properties])
    }

    return
  }

  runtime.pendingEvents.push({
    visitorId: runtime.visitorId,
    action,
    properties: properties ?? null,
    occurredAt: new Date().toISOString(),
    clientEventId: randomVisitorId(),
  })
  runtime.pendingEvents = runtime.pendingEvents.slice(-MAX_QUEUE)
  saveQueue(runtime.pendingEvents)

  flushSoon()
}

export function trackPageView(input: {
  path: string
  url: string
  title: string
  referrer: string
  locale: string
}): void {
  track('page.view', input)
}
