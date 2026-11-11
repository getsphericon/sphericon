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
import { zSiteUpdateAutomationInput } from '../../generated/site/zod.gen.ts'
import { editorSessionRaces } from '../../test/editorSessionRaces.ts'
import type { RaceRig } from '../../test/editorSessionRaces.ts'
import { problem } from '../../test/problem.ts'
import { requestGate } from '../../test/requestGate.ts'
import { worker } from '../../test/worker.ts'
import { stepsToGraph } from './definition.ts'
import type { AutomationGraph } from './definition.ts'
import { automationAdapter, editorSession } from './editorSession.ts'
import type { AutomationContent, AutomationOperation } from './editorSession.ts'

type OperationEvent = { type: 'PUBLISH'; current: string } | { type: 'REVERT' }

const PATH = { workspaceId: 'TestWs01', id: '5' }
const DRAFT = {
  number: 2,
  triggerEvent: 'email.clicked',
  exitEvents: [],
  goalEvent: null,
  steps: [],
}
const RESOURCE: SiteAutomationResource = {
  id: '5',
  name: 'Welcome flow',
  status: 'active',
  live: { ...DRAFT, number: 1 },
  draft: DRAFT,
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
}
const GRAPH = stepsToGraph(DRAFT, [], {})
const sessions = new Set<ActorRefFrom<typeof editorSession>>()

afterEach(() => {
  for (const actor of sessions) {
    actor.stop()
  }
  sessions.clear()
})

// A session over the canvas adapter, with the bar and canvas held in memory and a page whose cache,
// toasts and refreshes the effects write to.
function startCanvas(graph: AutomationGraph) {
  let current: AutomationContent = { name: RESOURCE.name, graph }
  const page: RaceRig<OperationEvent>['page'] = { cached: null, toasts: [], refreshedOver: null }
  const actor = createActor(editorSession, {
    input: {
      path: PATH,
      resource: RESOURCE,
      graph,
      labels: {},
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
      const body = zSiteUpdateAutomationInput.parse(await request.json())
      await gate.hold()
      if (refuse) {
        return problem(422, 'Refused definition')
      }
      server = { ...server, name: body.name ?? server.name, updatedAt: '2026-01-02T00:00:00Z' }
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
  const canvas = startCanvas(GRAPH)
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

// The content with new properties on one node, as the properties panel writes them.
function withProperties(
  content: AutomationContent,
  nodeId: string,
  properties: Record<string, unknown>,
): AutomationContent {
  const nodes = content.graph.nodes.map((node) =>
    node.id === nodeId
      ? { ...node, data: { ...node.data, properties: { ...node.data.properties, ...properties } } }
      : node,
  )
  return { ...content, graph: { ...content.graph, nodes } }
}

test('Save folds the stored HTTP identity into a canvas edited meanwhile, without reloading it', async () => {
  const gate = requestGate()
  worker.use(
    handleSiteAutomationsUpdate(async () => {
      await gate.hold()
      return HttpResponse.json({
        ...RESOURCE,
        draft: {
          ...DRAFT,
          steps: [{ type: 'http', id: '01ASSIGNED', body: '{}', hasSecret: false }],
        },
      })
    }),
  )
  const graph = stepsToGraph(DRAFT, [{ type: 'http', body: '{}' }], {})
  const httpId = graph.nodes[1]?.id ?? ''
  const canvas = startCanvas(graph)
  canvas.edit((content) => withProperties(content, httpId, { url: 'https://example.com/hook' }))
  canvas.actor.send({ type: 'SAVE', submission: capture(canvas.read()) })
  await gate.received
  canvas.edit((content) => withProperties(content, httpId, { body: '{"newer":true}' }))
  gate.release()
  const settled = await waitFor(canvas.actor, (state) => state.matches('idle'))
  const http = canvas.read().graph.nodes[1]
  const current = automationAdapter.fingerprint(canvas.read())
  expect(http?.data.properties.stepId).toBe('01ASSIGNED')
  expect(http?.data.properties.body).toBe('{"newer":true}')
  expect(current).not.toBe(settled.context.saved)
  expect(settled.context.content.revision).toBe(0)
})
