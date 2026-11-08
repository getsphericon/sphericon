import { range, throttle } from 'es-toolkit'
import Cookies from 'js-cookie'
import { v4 as uuid } from 'uuid'
import { assign, assertEvent, createActor, fromPromise, setup } from 'xstate'

import { createClient } from './generated/collect/client/index.ts'
import type { Client } from './generated/collect/client/index.ts'
import {
  collectEventsCreate,
  collectIdentifyCreate,
  collectTelegramStartLinksCreate,
} from './generated/collect/sdk.gen.ts'
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
}

declare global {
  interface Window {
    _omq?: QueueCommand[]
  }
}

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

// The existing throttle schedules flush requests independently of an active submission.
const flushSoon = throttle(requestFlush, FLUSH_DELAY_MS, {
  edges: ['trailing'],
})

// A fulfilled client call can mean acceptance, a final refusal, or no HTTP response. Only
// the first two settle the batch; an unconfirmed result leaves the recoverable queue intact.
type SubmissionOutcome = 'accepted' | 'refused' | 'unconfirmed'

interface DeliveryContext {
  runtime: Runtime | null
  pendingEvents: CollectEventInput[]
  batch: CollectEventInput[]
}

type DeliveryEvent =
  | { type: 'initialize'; runtime: Runtime; events: CollectEventInput[] }
  | { type: 'track'; event: CollectEventInput }
  | { type: 'flush' }

interface Submission {
  client: Client
  events: CollectEventInput[]
}

const deliveryTypes: { context?: DeliveryContext; events?: DeliveryEvent } = {}

const deliveryMachine = setup({
  types: deliveryTypes,
  actors: {
    submitBatch: fromPromise<SubmissionOutcome, Submission>(async ({ input }) => {
      // Ky owns all retry attempts for this one captured submission.
      const { response } = await collectEventsCreate({
        client: input.client,
        body: { events: input.events },
      })
      if (!response) {
        return 'unconfirmed'
      }
      return response.ok ? 'accepted' : 'refused'
    }),
  },
  guards: {
    hasPendingEvents: ({ context }) => context.pendingEvents.length > 0,
  },
  actions: {
    initialize: assign(({ event }) => {
      assertEvent(event, 'initialize')
      return { runtime: event.runtime, pendingEvents: event.events }
    }),
    appendEvent: assign(({ context, event }) => {
      assertEvent(event, 'track')
      return { pendingEvents: [...context.pendingEvents, event.event].slice(-MAX_QUEUE) }
    }),
    captureBatch: assign(({ context }) => ({ batch: [...context.pendingEvents] })),
    settleBatch: assign(({ context }) => ({
      pendingEvents: context.pendingEvents.filter((event) => !context.batch.includes(event)),
      batch: [],
    })),
    releaseBatch: assign({ batch: () => [] }),
    persistQueue: ({ context }) => saveQueue(context.pendingEvents),
    scheduleFlush: () => flushSoon(),
  },
}).createMachine({
  id: 'analyticsDelivery',
  initial: 'uninitialized',
  context: { runtime: null, pendingEvents: [], batch: [] },
  states: {
    uninitialized: {
      on: { initialize: { target: 'running', actions: 'initialize' } },
    },
    running: {
      initial: 'idle',
      on: {
        // Targetless: appending never exits sending or restarts its invoked submission.
        track: { actions: ['appendEvent', 'persistQueue', 'scheduleFlush'] },
      },
      states: {
        idle: {
          on: { flush: { guard: 'hasPendingEvents', target: 'sending', actions: 'captureBatch' } },
        },
        sending: {
          on: { flush: {} },
          invoke: {
            src: 'submitBatch',
            input: ({ context }) => {
              if (!context.runtime) {
                throw new Error('analytics delivery has no initialized runtime')
              }
              return { client: context.runtime.client, events: context.batch }
            },
            onDone: [
              {
                // Final refusals intentionally discard this batch, unlike an unconfirmed result.
                guard: ({ event }) => event.output === 'accepted' || event.output === 'refused',
                target: 'idle',
                actions: ['settleBatch', 'persistQueue', 'scheduleFlush'],
              },
              { target: 'idle', actions: 'releaseBatch' },
            ],
            onError: { target: 'idle', actions: 'releaseBatch' },
          },
        },
      },
    },
  },
})

const delivery = createActor(deliveryMachine).start()

function requestFlush(): void {
  delivery.send({ type: 'flush' })
}

function flushEventsOnPageHide(): void {
  flushSoon.cancel()
  requestFlush()
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
  const current = delivery.getSnapshot().context.runtime
  if (!isBrowser || current) {
    return
  }

  const initialQueue = Array.isArray(window._omq) ? [...window._omq] : []

  window._omq = createQueueApi([])

  const resolved = normalizeConfig(config) ?? findConfigInQueue(initialQueue)

  if (!resolved) {
    return
  }

  const client = collectClient(resolved)
  const visitorId = getOrCreateVisitorId()
  const events = readQueue()
  delivery.send({ type: 'initialize', runtime: { client, visitorId }, events })

  for (const command of initialQueue) {
    processQueueCommand(command)
  }

  window.addEventListener('pagehide', flushEventsOnPageHide)
  flushSoon()
}

export async function identify(payload: IdentifyInput): Promise<void> {
  const runtime = delivery.getSnapshot().context.runtime
  if (!runtime) {
    if (isBrowser) {
      window._omq ??= []
      Array.prototype.push.call(window._omq, ['identify', payload])
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

// A Telegram start link for the current visitor, for a "Get updates in Telegram" button the site
// draws: whoever presses Start in the bot with it binds their chat to this browser, and it reaches
// the contact once identify resolves the visitor. null when there is nothing to link to: the
// tracker is not started, the workspace has no enabled default Telegram bot, or the call failed.
export async function telegramStartLink(): Promise<string | null> {
  const runtime = delivery.getSnapshot().context.runtime
  if (!runtime) {
    return null
  }

  try {
    const { data } = await collectTelegramStartLinksCreate({
      client: runtime.client,
      retry: 0,
      body: { visitorId: runtime.visitorId },
    })

    return data?.url ?? null
  } catch {
    return null
  }
}

// The browser's time zone and language: guesses the server uses only for a Contact that has none.
function browserContext(): NonNullable<CollectIdentifyInput['context']> {
  return {
    timeZone: Intl.DateTimeFormat().resolvedOptions().timeZone || null,
    locale: navigator.language || null,
  }
}

export function track(action: string, properties?: EventProperties): void {
  const runtime = delivery.getSnapshot().context.runtime
  if (!runtime) {
    if (isBrowser) {
      window._omq ??= []
      Array.prototype.push.call(window._omq, ['track', action, properties])
    }

    return
  }

  delivery.send({
    type: 'track',
    event: {
      visitorId: runtime.visitorId,
      action,
      properties: properties ?? null,
      occurredAt: new Date().toISOString(),
      clientEventId: uuid(),
    },
  })
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
