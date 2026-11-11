import { HttpResponse } from 'msw'
import { afterEach, expect, test } from 'vitest'
import { createActor, waitFor } from 'xstate'
import type { ActorRefFrom } from 'xstate'

import { defineEditorSession } from '../../editing/editorSession.ts'
import {
  handleSiteRulesGet,
  handleSiteRulesPublish,
  handleSiteRulesRevert,
  handleSiteRulesUpdate,
} from '../../generated/site/msw.gen.ts'
import { siteRulesGet, siteRulesUpdate } from '../../generated/site/sdk.gen.ts'
import type { SiteRuleResource } from '../../generated/site/types.gen.ts'
import { zSiteUpdateRuleInput } from '../../generated/site/zod.gen.ts'
import { resourceEditorAdapter } from '../../resources/resourceEditor.ts'
import { editorSessionRaces } from '../../test/editorSessionRaces.ts'
import type { RaceRig } from '../../test/editorSessionRaces.ts'
import { TIMESTAMPS } from '../../test/payloads.ts'
import { problem } from '../../test/problem.ts'
import { requestGate } from '../../test/requestGate.ts'
import { worker } from '../../test/worker.ts'
import { editedRule, ruleOperations, ruleSchema } from './ruleEditor.ts'
import type { RuleValues } from './RuleForm.tsx'

type OperationEvent = { type: 'PUBLISH'; current: string } | { type: 'REVERT' }

// The Rule's editor session, as its edit page assembles it (defineResource).
const adapter = resourceEditorAdapter({
  id: 'ruleId',
  schema: ruleSchema,
  editedValues: editedRule,
  update: siteRulesUpdate,
  operations: ruleOperations,
})
const session = defineEditorSession(adapter)
const PATH = { workspaceId: 'TestWs01', id: '7' }
// Published as version 1, with a draft above it.
const RESOURCE: SiteRuleResource = {
  id: '7',
  name: 'Tag signups',
  status: 'active',
  live: { number: 1, triggerEvents: ['signed_up'], actions: [{ type: 'apply_tag', tag: 'live' }] },
  draft: {
    number: 2,
    triggerEvents: ['signed_up'],
    actions: [{ type: 'apply_tag', tag: 'drafted' }],
  },
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
function startForm(resource: SiteRuleResource) {
  const page: RaceRig<OperationEvent>['page'] = { cached: null, toasts: [], refreshedOver: null }
  let current: RuleValues = adapter.open({ path: PATH, resource })
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
    edit: (next: (content: RuleValues) => RuleValues) => {
      current = next(current)
    },
  }
}

function capture(content: RuleValues) {
  const submission = adapter.submit(content)
  if (!submission) {
    throw new Error('Fixture is not a valid Rule')
  }
  return submission
}

async function stored() {
  const { data } = await siteRulesGet({ path: PATH, throwOnError: true })
  return data
}

// The server keeps the draft it is sent, and normalizes the name it is sent: it trims it.
function serve({ refuse }: { refuse: boolean }) {
  const gate = requestGate()
  let server = RESOURCE
  worker.use(
    handleSiteRulesGet(() => HttpResponse.json(server)),
    handleSiteRulesUpdate(async ({ request }) => {
      const { name, ...definition } = zSiteUpdateRuleInput.parse(await request.json())
      await gate.hold()
      if (refuse) {
        return problem(422, 'Refused definition')
      }
      server = {
        ...server,
        name: (name ?? server.name).trim(),
        draft: {
          number: server.draft?.number ?? 2,
          triggerEvents: definition.triggerEvents ?? [],
          actions: definition.actions ?? [],
        },
        updatedAt: '2026-01-02T00:00:00Z',
      }
      return HttpResponse.json(server)
    }),
    handleSiteRulesPublish(() => {
      server = { ...server, live: server.draft, draft: null }
      return HttpResponse.json(server)
    }),
    handleSiteRulesRevert(() => {
      server = { ...server, draft: null }
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
    page: form.page,
    stored,
    initial: RESOURCE,
    gate,
  }
}

editorSessionRaces(startRaces)

test('a Save the form still holds takes the normalized response and leaves the form clean', async () => {
  const gate = serve({ refuse: false })
  gate.release()
  const form = startForm(RESOURCE)
  form.edit((content) => ({ ...content, name: '  Tag new signups  ' }))
  form.actor.send({ type: 'SAVE', submission: capture(form.read()) })
  const { context } = await waitFor(form.actor, (state) => state.matches('idle'))
  expect(form.read().name).toBe('Tag new signups')
  expect(adapter.fingerprint(form.read())).toBe(context.saved)
  expect(context.content.revision).toBe(1)
})

test('Revert replaces the form with the live version, which becomes the acknowledged content', async () => {
  serve({ refuse: false })
  const form = startForm(RESOURCE)
  form.actor.send({ type: 'REVERT' })
  const { context } = await waitFor(form.actor, (state) => state.matches('idle'))
  expect(form.read().actions).toEqual([{ type: 'apply_tag', tag: 'live' }])
  expect(adapter.fingerprint(form.read())).toBe(context.saved)
  expect(form.page.toasts).toEqual(['accepted REVERT'])
})
