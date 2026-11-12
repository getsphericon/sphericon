import { HttpResponse } from 'msw'
import { afterEach } from 'vitest'
import { createActor, waitFor } from 'xstate'
import type { ActorRefFrom } from 'xstate'

import { defineEditorSession } from '../../editing/editorSession.ts'
import {
  handleSiteBroadcastsGet,
  handleSiteBroadcastsSchedule,
  handleSiteBroadcastsSend,
  handleSiteBroadcastsUpdate,
} from '../../generated/site/msw.gen.ts'
import { siteBroadcastsGet, siteBroadcastsUpdate } from '../../generated/site/sdk.gen.ts'
import type { SiteBroadcastResource } from '../../generated/site/types.gen.ts'
import { zUpdateBroadcastInput } from '../../generated/site/zod.gen.ts'
import { resourceEditorAdapter, withRevision } from '../../resources/resourceEditor.ts'
import { editorSessionRaces } from '../../test/editorSessionRaces.ts'
import type { RaceRig } from '../../test/editorSessionRaces.ts'
import { TIMESTAMPS, testId } from '../../test/payloads.ts'
import { problem } from '../../test/problem.ts'
import { requestGate } from '../../test/requestGate.ts'
import { worker } from '../../test/worker.ts'
import { broadcastOperations, broadcastSchema } from './broadcastEditor.ts'
import type { BroadcastValues } from './BroadcastForm.tsx'

type OperationEvent =
  | { type: 'SEND'; current: string }
  | { type: 'SCHEDULE'; current: string; input: { scheduledAt: string } }

// The Broadcast's editor session, as its edit page assembles it (defineResource).
const adapter = resourceEditorAdapter({
  id: 'broadcastId',
  schema: broadcastSchema,
  update: withRevision(siteBroadcastsUpdate),
  operations: broadcastOperations,
})
const session = defineEditorSession(adapter)
const PATH = { workspaceId: 'TestWs01', id: testId(7) }
// A draft email Broadcast, its delivery not started.
const RESOURCE: SiteBroadcastResource = {
  id: testId(7),
  name: 'Launch',
  channel: 'email',
  content: {
    channel: 'email',
    subject: 'Hello',
    fromName: 'Ada',
    fromEmail: 'ada@example.com',
    body: '<p>Hi</p>',
  },
  status: 'draft',
  stats: {
    recipientsTotal: 0,
    sentCount: 0,
    skippedCount: 0,
    failedCount: 0,
    pendingCount: 0,
    openedCount: 0,
    clickedCount: 0,
    unsubscribedCount: 0,
    deliveredCount: 0,
    undeliveredCount: null,
    bouncedCount: 0,
    complainedCount: 0,
    capturedCount: null,
    deliveryRate: 0,
    openRate: 0,
    clickRate: 0,
    clickToOpenRate: 0,
    unsubscribeRate: 0,
    failureRate: 0,
    revenue: { reportingCurrency: null, orders: 0, amount: 0 },
  },
  revision: 1,
  ...TIMESTAMPS,
}
const sessions = new Set<ActorRefFrom<typeof session>>()

afterEach(() => {
  for (const actor of sessions) {
    actor.stop()
  }
  sessions.clear()
})

// A session over the schema-form adapter, with the form's values held in memory and a page whose
// cache, toasts and refreshes the effects write to.
function startForm(resource: SiteBroadcastResource) {
  const page: RaceRig<OperationEvent>['page'] = { cached: null, toasts: [], refreshedOver: null }
  let current: BroadcastValues = adapter.open({ path: PATH, resource })
  const actor = createActor(session, {
    input: {
      path: PATH,
      resource,
      editor: {
        read: () => current,
        write: (content) => {
          current = content
        },
      },
      effects: {
        store: (written) => {
          page.cached = written
        },
        accepted: (operation) => {
          page.toasts.push(`accepted ${operation}`)
        },
        failed: (operation) => {
          page.toasts.push(`failed ${operation}`)
        },
        invalidate: () => {
          page.refreshedOver = page.cached
        },
      },
    },
  }).start()
  sessions.add(actor)
  return {
    actor,
    page,
    read: () => current,
    edit: (next: (content: BroadcastValues) => BroadcastValues) => {
      current = next(current)
    },
  }
}

function capture(content: BroadcastValues) {
  const submission = adapter.submit(content)
  if (!submission) {
    throw new Error('Fixture is not a valid Broadcast')
  }
  return submission
}

async function stored() {
  const { data } = await siteBroadcastsGet({ path: PATH, throwOnError: true })
  return data
}

// The server keeps the draft it is sent, trims its name and moves its revision; Send now and
// Schedule start its delivery.
function serve({ refuse }: { refuse: boolean }) {
  const gate = requestGate()
  let server = RESOURCE
  worker.use(
    handleSiteBroadcastsGet(() => HttpResponse.json(server)),
    handleSiteBroadcastsUpdate(async ({ request }) => {
      const { name } = zUpdateBroadcastInput.parse(await request.json())
      await gate.hold()
      if (refuse) {
        return problem(422, 'Refused broadcast')
      }
      server = { ...server, name: (name ?? server.name).trim(), revision: server.revision + 1 }
      return HttpResponse.json(server)
    }),
    handleSiteBroadcastsSend(() => {
      server = { ...server, status: 'sending', revision: server.revision + 1 }
      return HttpResponse.json(server)
    }),
    handleSiteBroadcastsSchedule(() => {
      server = { ...server, status: 'scheduled', revision: server.revision + 1 }
      return HttpResponse.json(server)
    }),
  )
  return gate
}

function startRaces({ refuse }: { refuse: boolean }): RaceRig<OperationEvent> {
  const gate = serve({ refuse })
  const form = startForm(RESOURCE)
  const { actor } = form
  const fingerprint = () => adapter.fingerprint(form.read())
  let submitted = ''
  return {
    save: () => {
      const submission = capture(form.read())
      submitted = submission.fingerprint
      actor.send({ type: 'SAVE', submission })
    },
    canSave: () => actor.getSnapshot().can({ type: 'SAVE', submission: capture(form.read()) }),
    edit: () => {
      form.edit((content) => ({ ...content, name: `${content.name} (edited)` }))
    },
    operations: [
      { type: 'SEND', current: fingerprint() },
      { type: 'SCHEDULE', current: fingerprint(), input: { scheduledAt: '2030-05-06T07:08:00Z' } },
    ],
    send: (event) => {
      actor.send(event)
    },
    can: (event) => actor.getSnapshot().can(event),
    stop: () => {
      actor.stop()
    },
    settled: async () => {
      const { context } = await waitFor(actor, (state) => state.matches('idle'))
      return { saved: context.saved, result: context.result, revision: context.content.revision }
    },
    current: fingerprint,
    submitted: () => submitted,
    page: form.page,
    stored,
    initial: RESOURCE,
    gate,
  }
}

editorSessionRaces(startRaces)
