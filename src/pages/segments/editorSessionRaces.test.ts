import { HttpResponse } from 'msw'
import { afterEach, expect, test } from 'vitest'
import { createActor, waitFor } from 'xstate'
import type { ActorRefFrom } from 'xstate'

import { defineEditorSession } from '../../editing/editorSession.ts'
import { handleSiteSegmentsGet, handleSiteSegmentsUpdate } from '../../generated/site/msw.gen.ts'
import { siteSegmentsGet, siteSegmentsUpdate } from '../../generated/site/sdk.gen.ts'
import type { SiteSegmentResource } from '../../generated/site/types.gen.ts'
import { zSiteUpdateSegmentInput } from '../../generated/site/zod.gen.ts'
import { resourceEditorAdapter, withRevision } from '../../resources/resourceEditor.ts'
import { editorSessionRaces } from '../../test/editorSessionRaces.ts'
import type { RaceRig } from '../../test/editorSessionRaces.ts'
import { TIMESTAMPS } from '../../test/payloads.ts'
import { problem } from '../../test/problem.ts'
import { requestGate } from '../../test/requestGate.ts'
import { worker } from '../../test/worker.ts'
import type { SegmentValues } from './SegmentForm.tsx'
import { segmentSchema } from './segmentSchema.ts'

interface OperationEvent {
  type: string
}

// The Segment's editor session, as its edit page assembles it (defineResource).
const adapter = resourceEditorAdapter({
  id: 'segmentId',
  schema: segmentSchema,
  update: withRevision(siteSegmentsUpdate),
  operations: {},
})
const session = defineEditorSession(adapter)
const PATH = { workspaceId: 'TestWs01', id: '7' }
const RESOURCE: SiteSegmentResource = {
  id: '7',
  name: 'Pros',
  condition: { kind: 'group', combinator: 'and', rules: [] },
  archivedAt: null,
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

// The server keeps what it is sent when the revision is the stored one and moves it by one; a
// stale revision is a 409 revision_changed and changes nothing (ADR 0119). `elsewhere` is another
// writer's save.
function serve({ refuse }: { refuse: boolean }) {
  const gate = requestGate()
  let server = RESOURCE
  const sent: number[] = []
  worker.use(
    handleSiteSegmentsGet(() => HttpResponse.json(server)),
    handleSiteSegmentsUpdate(async ({ request }) => {
      const { revision, name, condition } = zSiteUpdateSegmentInput.parse(await request.json())
      sent.push(revision)
      await gate.hold()
      if (refuse) {
        return problem(422, 'Refused segment')
      }
      if (revision !== server.revision) {
        return problem(409, { title: 'Changed', code: 'revision_changed' })
      }
      server = {
        ...server,
        name: (name ?? server.name).trim(),
        ...(condition && { condition }),
        revision: server.revision + 1,
      }
      return HttpResponse.json(server)
    }),
  )
  return {
    gate,
    sent,
    elsewhere: () => {
      server = { ...server, name: 'Saved elsewhere', revision: server.revision + 1 }
      return server
    },
  }
}

function startForm(resource: SiteSegmentResource) {
  const page: RaceRig<OperationEvent>['page'] = { cached: null, toasts: [], refreshedOver: null }
  let current: SegmentValues = adapter.open({ path: PATH, resource })
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
    edit: (next: (content: SegmentValues) => SegmentValues) => {
      current = next(current)
    },
  }
}

function capture(content: SegmentValues) {
  const submission = adapter.submit(content)
  if (!submission) {
    throw new Error('Fixture is not a valid Segment')
  }
  return submission
}

async function stored() {
  const { data } = await siteSegmentsGet({ path: PATH, throwOnError: true })
  return data
}

const idle = async (form: ReturnType<typeof startForm>) =>
  waitFor(form.actor, (state) => state.matches('idle'))

function startRaces({ refuse }: { refuse: boolean }): RaceRig<OperationEvent> {
  const { gate } = serve({ refuse })
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
    operations: [],
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

editorSessionRaces(startRaces, { operations: false })

test('a refetch that brings a newer revision says so, and the Save over it is refused', async () => {
  const server = serve({ refuse: false })
  server.gate.release()
  const form = startForm(RESOURCE)
  form.edit((content) => ({ ...content, name: 'Mine' }))
  form.actor.send({ type: 'REFRESH', resource: server.elsewhere() })
  const refreshed = form.actor.getSnapshot().context
  expect(refreshed.changed).toBe(true)
  expect(refreshed.base).toBe(1)

  form.actor.send({ type: 'SAVE', submission: capture(form.read()) })
  const { context } = await idle(form)

  expect(server.sent).toEqual([1])
  expect(context.result).toBe('error')
  expect(context.changed).toBe(true)
  expect(context.base).toBe(1)
  expect(form.read().name).toBe('Mine')
  expect(adapter.fingerprint(form.read())).not.toBe(context.saved)
  expect(form.page.toasts).toEqual([])
  const kept = await stored()
  expect(kept.name).toBe('Saved elsewhere')
})

test('saving mine over it sends the same content on the newer revision', async () => {
  const server = serve({ refuse: false })
  server.gate.release()
  const form = startForm(RESOURCE)
  form.edit((content) => ({ ...content, name: 'Mine' }))
  server.elsewhere()
  form.actor.send({ type: 'SAVE', submission: capture(form.read()) })
  await idle(form)

  form.actor.send({ type: 'OVERWRITE', submission: capture(form.read()), resource: await stored() })
  const { context } = await idle(form)

  expect(server.sent).toEqual([1, 2])
  expect(context.result).toBe('success')
  expect(context.changed).toBe(false)
  expect(context.base).toBe(3)
  const written = await stored()
  expect(written.name).toBe('Mine')
})

test('loading the saved version replaces the content, which becomes the acknowledged one', async () => {
  const server = serve({ refuse: false })
  server.gate.release()
  const form = startForm(RESOURCE)
  form.edit((content) => ({ ...content, name: 'Mine' }))
  form.actor.send({ type: 'REFRESH', resource: server.elsewhere() })

  form.actor.send({ type: 'RELOAD', resource: await stored() })
  const { context } = form.actor.getSnapshot()

  expect(form.read().name).toBe('Saved elsewhere')
  expect(adapter.fingerprint(form.read())).toBe(context.saved)
  expect(context.changed).toBe(false)
  expect(context.base).toBe(2)
  expect(context.content.revision).toBe(1)
})

test('two Saves in a row from one editor never conflict with each other', async () => {
  const server = serve({ refuse: false })
  server.gate.release()
  const form = startForm(RESOURCE)
  form.edit((content) => ({ ...content, name: 'First' }))
  form.actor.send({ type: 'SAVE', submission: capture(form.read()) })
  await idle(form)
  // The refetch the first Save starts brings the revision it made: not a change of another's.
  form.actor.send({ type: 'REFRESH', resource: await stored() })
  form.edit((content) => ({ ...content, name: 'Second' }))
  form.actor.send({ type: 'SAVE', submission: capture(form.read()) })
  const { context } = await idle(form)

  expect(server.sent).toEqual([1, 2])
  expect(context.result).toBe('success')
  expect(context.changed).toBe(false)
  const written = await stored()
  expect(written.name).toBe('Second')
})
