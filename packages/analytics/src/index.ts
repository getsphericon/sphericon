import { range, throttle } from 'es-toolkit'
import Cookies from 'js-cookie'
import { v4 as uuid } from 'uuid'

import { createClient } from './generated/collect/client/index.ts'
import type { Client } from './generated/collect/client/index.ts'
import { collectEventsCreate, collectIdentifyCreate } from './generated/collect/sdk.gen.ts'
import type { CollectEventInput, CollectIdentifyInput } from './generated/collect/types.gen.ts'

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
// visitorId and the browser's context are supplied internally, so consumers never pass them.
type IdentifyInput = Omit<CollectIdentifyInput, 'visitorId' | 'context'>
type EventProperties = CollectEventInput['properties']

type QueueCommand =
  | ['init', Partial<TrackerConfig>]
  | ['identify', IdentifyInput?]
  | ['track', string, EventProperties | undefined]

type QueueApi = QueueCommand[] & {
  push: (...items: QueueCommand[]) => number
}

interface Runtime {
  client: Client
  visitorId: string
  pendingEvents: CollectEventInput[]
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

// The collect client of one tracker. A batch the instance cannot take now (a network failure, a
// timeout, a 429 or a 5xx) is resent by ky until it is taken: after the wait a Retry-After names,
// or a doubling backoff. Any other refusal is final.
function collectClient(config: TrackerConfig): Client {
  return createClient({
    baseUrl: `${config.baseUrl}/collect`,
    auth: config.collectKey,
    keepalive: true,
    retry: {
      limit: Number.MAX_SAFE_INTEGER,
      methods: ['post'],
      statusCodes: [429, ...range(500, 600)],
      retryOnTimeout: true,
      delay: (attempt) => RETRY_BASE_MS * 2 ** (attempt - 1),
      backoffLimit: RETRY_MAX_MS,
    },
    kyOptions: { throwHttpErrors: true },
  })
}

function getOrCreateVisitorId(): string {
  const existing = Cookies.get(VISITOR_COOKIE_NAME)

  if (existing) {
    return existing
  }

  const created = uuid()
  Cookies.set(VISITOR_COOKIE_NAME, created, { path: '/', expires: 365, sameSite: 'Lax' })

  return created
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

// Events tracked close together go out as one batch, FLUSH_DELAY_MS after the first of them.
const flushSoon = throttle(() => void flushEvents(), FLUSH_DELAY_MS, { edges: ['trailing'] })

async function flushEvents(): Promise<void> {
  if (!runtime || runtime.sending || runtime.pendingEvents.length === 0) {
    return
  }

  const state = runtime
  const events = state.pendingEvents.slice()

  state.sending = true

  // Settles once the instance took the batch or refused it for good; the retries happen inside.
  const { response } = await collectEventsCreate({ client: state.client, body: { events } })

  state.sending = false

  // No answer at all (the request could not be made): the batch stays queued for the next flush.
  if (!response) {
    return
  }

  state.pendingEvents = state.pendingEvents.filter((event) => !events.includes(event))
  saveQueue(state.pendingEvents)
  flushSoon()
}

function flushEventsOnPageHide(): void {
  flushSoon.cancel()
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
    client: collectClient(resolved),
    visitorId: getOrCreateVisitorId(),
    pendingEvents: readQueue(),
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

  // An identify is sent once: a failure is dropped, not retried.
  await collectIdentifyCreate({
    client: runtime.client,
    retry: 0,
    body: {
      visitorId: runtime.visitorId,
      email: payload.email ?? null,
      phone: payload.phone ?? null,
      subjectId: payload.subjectId ?? null,
      traits: payload.traits ?? null,
      context: browserContext(),
    },
  }).catch(() => undefined)
}

// The browser's time zone and language: guesses the server uses only for a Contact that has none.
function browserContext(): NonNullable<CollectIdentifyInput['context']> {
  return {
    timeZone: Intl.DateTimeFormat().resolvedOptions().timeZone || null,
    locale: navigator.language || null,
  }
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
    clientEventId: uuid(),
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
