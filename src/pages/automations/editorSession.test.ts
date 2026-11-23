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
import { stepId, testId } from '../../test/payloads.ts'
import { problem } from '../../test/problem.ts'
import { worker } from '../../test/worker.ts'
import { stepsToGraph } from './definition.ts'
import type { AutomationGraph } from './definition.ts'
import { automationSubmission, editorSession } from './editorSession.ts'
import { automationSnapshot } from './leaveSnapshot.ts'

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
const GRAPH = stepsToGraph(DRAFT, DRAFT.steps, {})
const sessions = new Set<ActorRefFrom<typeof editorSession>>()

function startSession() {
  const actor = createActor(editorSession, {
    input: { path: PATH, resource: RESOURCE, graph: GRAPH, labels: {} },
  }).start()
  sessions.add(actor)
  return actor
}

function stopSessions() {
  for (const actor of sessions) {
    actor.stop()
  }
  sessions.clear()
}
afterEach(stopSessions)

function capture(name: string, graph: AutomationGraph = GRAPH) {
  const submission = automationSubmission(name, graph)
  if (!submission) {
    throw new Error('Fixture has no Trigger')
  }
  return submission
}

function httpNode(graph: AutomationGraph) {
  const node = graph.nodes[1]
  if (!node) {
    throw new Error('Fixture has no HTTP node')
  }
  return node
}

// The API's write-only HTTP fields can be omitted only when the Step's id names a stored step, whose
// id the server keeps.
function httpServer() {
  const received = Promise.withResolvers<undefined>()
  const release = Promise.withResolvers<undefined>()
  let stored = RESOURCE
  worker.use(
    handleSiteAutomationsGet(() => HttpResponse.json(stored)),
    handleSiteAutomationsUpdate(async ({ request }) => {
      const body = zUpdateAutomationInput.parse(await request.json())
      const step = body.steps?.steps[0]
      if (step?.type !== 'http' || step.id === undefined) {
        return problem(422, 'Expected an HTTP step with its id')
      }
      const previous = stored.draft?.steps.steps[0]
      const known = previous?.type === 'http' && step.id === previous.id
      if (!known && !step.url) {
        return problem(422, 'A new HTTP step needs a URL')
      }
      received.resolve(undefined)
      await release.promise
      stored = {
        ...RESOURCE,
        name: body.name ?? RESOURCE.name,
        draft: {
          ...DRAFT,
          steps: {
            id: stepId(1),
            steps: [
              {
                type: 'http',
                id: step.id,
                body: step.body,
                headers: [],
                hasSecret: Boolean(step.secret) || (known && previous.hasSecret),
              },
            ],
          },
        },
      }
      return HttpResponse.json(stored)
    }),
  )
  return { received, release }
}

test('one in-flight Save excludes other writes and acknowledges only its captured content', async () => {
  const received = Promise.withResolvers<undefined>()
  const release = Promise.withResolvers<undefined>()
  let stored = RESOURCE
  worker.use(
    handleSiteAutomationsGet(() => HttpResponse.json(stored)),
    handleSiteAutomationsUpdate(async ({ request }) => {
      const body = zUpdateAutomationInput.parse(await request.json())
      received.resolve(undefined)
      await release.promise
      stored = { ...RESOURCE, name: body.name ?? RESOURCE.name }
      return HttpResponse.json(stored)
    }),
  )
  const actor = startSession()
  const submitted = capture('Submitted name')
  actor.send({ type: 'SAVE', submission: submitted })
  await received.promise
  actor.send({ type: 'SAVE', submission: capture('Later name') })
  actor.send({ type: 'PUBLISH', current: actor.getSnapshot().context.saved })
  actor.send({ type: 'REVERT' })
  release.resolve(undefined)
  const settled = await waitFor(actor, (state) => state.matches('idle'))
  const { data: persisted } = await siteAutomationsGet({ path: PATH, throwOnError: true })
  expect(persisted.name).toBe('Submitted name')
  expect(settled.context.saved).toBe(submitted.fingerprint)
  expect(settled.context.result).toBe('success')
})

test('a refused Save retains its previous receipt and allows another attempt', async () => {
  worker.use(handleSiteAutomationsUpdate(() => problem(422, 'Refused definition')))
  const actor = startSession()
  const before = actor.getSnapshot().context.saved
  actor.send({ type: 'SAVE', submission: capture('Refused name') })
  const settled = await waitFor(actor, (state) => state.matches('idle'))
  expect(settled.context.saved).toBe(before)
  expect(settled.context.result).toBe('error')
  expect(settled.can({ type: 'SAVE', submission: capture('Another attempt') })).toBe(true)
  expect(settled.can({ type: 'PUBLISH', current: 'unsaved content' })).toBe(false)
})

test.each([
  { type: 'PUBLISH' as const, current: automationSnapshot(RESOURCE.name, GRAPH) },
  { type: 'REVERT' as const },
])('a refused $type preserves the editor content and receipt', async (event) => {
  worker.use(
    handleSiteAutomationsPublish(() => problem(409, 'Cannot publish')),
    handleSiteAutomationsRevert(() => problem(409, 'Cannot revert')),
  )
  const actor = startSession()
  const before = actor.getSnapshot().context
  actor.send(event)
  const settled = await waitFor(actor, (state) => state.matches('idle'))
  expect(settled.context.content).toBe(before.content)
  expect(settled.context.saved).toBe(before.saved)
  expect(settled.context.resource).toBe(before.resource)
  expect(settled.can({ type: 'SAVE', submission: capture('Retry') })).toBe(true)
})

test('a refresh updates lifecycle metadata without replacing content or its receipt', () => {
  const actor = startSession()
  const before = actor.getSnapshot().context
  actor.send({ type: 'REFRESH', resource: { ...RESOURCE, draft: null } })
  const after = actor.getSnapshot().context
  expect(after.resource.draft).toBeNull()
  expect(after.content).toBe(before.content)
  expect(after.saved).toBe(before.saved)
  expect(actor.getSnapshot().can({ type: 'REVERT' })).toBe(false)
})

test('two Saves keep an HTTP step by its id and its stored secret while preserving newer content', async () => {
  // A node dropped on the canvas: its id, which the SDK minted, is the Step's from the first Save.
  const graph = stepsToGraph(
    DRAFT,
    {
      id: stepId(1),
      steps: [{ id: stepId(7), type: 'http', body: '{}', headers: [], hasSecret: false }],
    },
    {},
  )
  const node = httpNode(graph)
  node.data.properties.url = 'https://example.com/hook'
  node.data.properties.secret = `whsec_${btoa('submitted')}`
  const server = httpServer()
  const actor = startSession()
  actor.send({ type: 'SAVE', submission: capture('HTTP flow', graph) })
  await server.received.promise
  node.data.properties.body = '{"newer":true}'
  server.release.resolve(undefined)
  const first = await waitFor(actor, (state) => state.matches('idle'))
  const current = graph
  const currentHttp = httpNode(current)
  expect(currentHttp.data.properties.body).toBe('{"newer":true}')
  expect(currentHttp.id).toBe(stepId(7))
  expect(automationSnapshot('HTTP flow', current)).not.toBe(first.context.saved)
  currentHttp.data.properties.url = ''
  currentHttp.data.properties.secret = ''
  actor.send({ type: 'SAVE', submission: capture('HTTP flow', current) })
  const second = await waitFor(actor, (state) => state.matches('idle'))
  const { data: persisted } = await siteAutomationsGet({ path: PATH, throwOnError: true })
  expect(second.context.result).toBe('success')
  expect(second.context.saved).toBe(automationSnapshot('HTTP flow', current))
  expect(persisted.draft?.steps.steps).toEqual([
    { type: 'http', id: stepId(7), body: '{"newer":true}', headers: [], hasSecret: true },
  ])
})
