import { HttpResponse } from 'msw'
import { afterEach, expect, test } from 'vitest'
import { createActor, waitFor } from 'xstate'
import type { ActorRefFrom } from 'xstate'

import {
  handleSiteAutomationsGet,
  handleSiteAutomationsPublish,
  handleSiteAutomationsRevert,
  handleSiteAutomationsUpdate,
} from '../../generated/site/msw.gen.ts'
import { siteAutomationsGet } from '../../generated/site/sdk.gen.ts'
import type { SiteAutomationResource } from '../../generated/site/types.gen.ts'
import { zUpdateAutomationInput } from '../../generated/site/zod.gen.ts'
import { editorSessionRaces } from '../../test/editorSessionRaces.ts'
import type { RaceRig } from '../../test/editorSessionRaces.ts'
import { stepId, testId } from '../../test/payloads.ts'
import { problem } from '../../test/problem.ts'
import { requestGate } from '../../test/requestGate.ts'
import { worker } from '../../test/worker.ts'
import { flowOf } from './definition.ts'
import type { Flow } from './definition.ts'
import { automationAdapter, editorSession } from './editorSession.ts'
import type { AutomationContent, AutomationOperation } from './editorSession.ts'

type OperationEvent = { type: 'PUBLISH'; current: string } | { type: 'REVERT' }

const PATH = { workspaceId: 'TestWs01', id: testId(5) }
const DRAFT = {
  number: 2,
  trigger: {
    kind: 'event' as const,
    action: 'email.clicked',
    entryPolicy: 'once' as const,
    cooldown: 3600,
  },
  exitEvents: [],
  goalEvent: null,
  steps: { id: stepId(1), steps: [] },
}
const RESOURCE: SiteAutomationResource = {
  revision: 1,
  id: testId(5),
  name: 'Welcome flow',
  status: 'active',
  live: { ...DRAFT, number: 1 },
  draft: DRAFT,
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
}
const FLOW = flowOf(DRAFT)
const sessions = new Set<ActorRefFrom<typeof editorSession>>()

afterEach(() => {
  for (const actor of sessions) {
    actor.stop()
  }
  sessions.clear()
})

// A session over the canvas adapter, with the bar and canvas held in memory and a page whose cache,
// toasts and refreshes the effects write to.
function startCanvas(flow: Flow) {
  let current: AutomationContent = { name: RESOURCE.name, flow }
  const page: RaceRig<OperationEvent>['page'] = { cached: null, toasts: [], refreshedOver: null }
  const actor = createActor(editorSession, {
    input: {
      path: PATH,
      resource: RESOURCE,
      flow,
      editor: {
        read: () => current,
        write: (content) => {
          current = content
        },
      },
      effects: {
        store: (resource) => {
          page.cached = resource
        },
        accepted: (operation: AutomationOperation | 'SAVE') => {
          page.toasts.push(`accepted ${operation}`)
        },
        failed: (operation: AutomationOperation | 'SAVE') => {
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
    edit: (next: (content: AutomationContent) => AutomationContent) => {
      current = next(current)
    },
  }
}

function capture(content: AutomationContent) {
  const submission = automationAdapter.submit(content)
  if (!submission) {
    throw new Error('Fixture has no Trigger')
  }
  return submission
}

async function stored() {
  const { data } = await siteAutomationsGet({ path: PATH, throwOnError: true })
  return data
}

function startRaces({ refuse }: { refuse: boolean }): RaceRig<OperationEvent> {
  const gate = requestGate()
  let server = RESOURCE
  worker.use(
    handleSiteAutomationsGet(() => HttpResponse.json(server)),
    handleSiteAutomationsUpdate(async ({ request }) => {
      const body = zUpdateAutomationInput.parse(await request.json())
      await gate.hold()
      if (refuse) {
        return problem(422, 'Refused definition')
      }
      server = {
        ...server,
        name: body.name ?? server.name,
        revision: server.revision + 1,
        updatedAt: '2026-01-02T00:00:00Z',
      }
      return HttpResponse.json(server)
    }),
    handleSiteAutomationsPublish(() => {
      server = { ...server, live: server.draft, draft: null }
      return HttpResponse.json(server)
    }),
    handleSiteAutomationsRevert(() => {
      server = { ...server, draft: null }
      return HttpResponse.json(server)
    }),
  )
  const canvas = startCanvas(FLOW)
  const { actor } = canvas
  const fingerprint = () => automationAdapter.fingerprint(canvas.read())
  let submitted = ''
  return {
    save: () => {
      const submission = capture(canvas.read())
      submitted = submission.fingerprint
      actor.send({ type: 'SAVE', submission })
    },
    canSave: () => actor.getSnapshot().can({ type: 'SAVE', submission: capture(canvas.read()) }),
    edit: () => {
      canvas.edit((content) => ({ ...content, name: `${content.name} (edited)` }))
    },
    operations: [{ type: 'PUBLISH', current: fingerprint() }, { type: 'REVERT' }],
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
    page: canvas.page,
    stored,
    initial: RESOURCE,
    gate,
  }
}

editorSessionRaces(startRaces)

// The content with the HTTP step's draft changed, as its form writes it.
function withHttp(
  content: AutomationContent,
  change: Partial<{ url: string; body: string }>,
): AutomationContent {
  const steps = content.flow.path.steps.map((step) =>
    step.kind === 'http' ? { ...step, draft: { ...step.draft, ...change } } : step,
  )
  return { ...content, flow: { ...content.flow, path: { ...content.flow.path, steps } } }
}

test('Save leaves a canvas edited meanwhile as edited, each node keeping its Step id, without reloading it', async () => {
  const gate = requestGate()
  worker.use(
    handleSiteAutomationsUpdate(async () => {
      await gate.hold()
      return HttpResponse.json({
        ...RESOURCE,
        draft: {
          ...DRAFT,
          steps: {
            id: stepId(1),
            steps: [{ type: 'http', id: stepId(7), body: '{}', headers: [], hasSecret: false }],
          },
        },
      })
    }),
  )
  const flow = flowOf({
    ...DRAFT,
    steps: {
      id: stepId(1),
      steps: [{ id: stepId(7), type: 'http', body: '{}', headers: [], hasSecret: false }],
    },
  })
  const canvas = startCanvas(flow)
  canvas.edit((content) => withHttp(content, { url: 'https://example.com/hook' }))
  canvas.actor.send({ type: 'SAVE', submission: capture(canvas.read()) })
  await gate.received
  canvas.edit((content) => withHttp(content, { body: '{"newer":true}' }))
  gate.release()
  const settled = await waitFor(canvas.actor, (state) => state.matches('idle'))
  const [http] = canvas.read().flow.path.steps
  const current = automationAdapter.fingerprint(canvas.read())
  expect(http).toMatchObject({ id: stepId(7), draft: { body: '{"newer":true}' } })
  expect(current).not.toBe(settled.context.saved)
  expect(settled.context.content.revision).toBe(0)
})

// The canvas tells a change made elsewhere from its own (ADR 0119): load, another writer saves, the
// query refetches, Save is refused with revision_changed, and the canvas is kept as edited.
test('a Save over a change made elsewhere is refused and the canvas is kept', async () => {
  let server = RESOURCE
  const sent: number[] = []
  worker.use(
    handleSiteAutomationsGet(() => HttpResponse.json(server)),
    handleSiteAutomationsUpdate(async ({ request }) => {
      const body = zUpdateAutomationInput.parse(await request.json())
      sent.push(body.revision)
      if (body.revision !== server.revision) {
        return problem(409, { title: 'Changed', code: 'revision_changed' })
      }
      server = { ...server, name: body.name ?? server.name, revision: server.revision + 1 }
      return HttpResponse.json(server)
    }),
  )
  const canvas = startCanvas(FLOW)
  canvas.edit((content) => ({ ...content, name: 'Mine' }))
  server = { ...server, name: 'Saved elsewhere', revision: server.revision + 1 }
  canvas.actor.send({ type: 'REFRESH', resource: await stored() })

  canvas.actor.send({ type: 'SAVE', submission: capture(canvas.read()) })
  const { context } = await waitFor(canvas.actor, (state) => state.matches('idle'))

  expect(sent).toEqual([1])
  expect(context.result).toBe('error')
  expect(context.changed).toBe(true)
  expect(context.base).toBe(1)
  expect(canvas.read().name).toBe('Mine')
  expect(canvas.page.toasts).toEqual([])
  const kept = await stored()
  expect(kept.name).toBe('Saved elsewhere')
})
