import { expect, test } from 'vitest'

import type {
  AutomationStep,
  AutomationVersion,
  BranchCondition,
} from '../../generated/site/types.gen.ts'
import { stepId } from '../../test/payloads.ts'
import { flowOf, forkPaths, pathInputOf } from './definition.ts'
import type { BranchStep, Flow } from './definition.ts'
import {
  addPath,
  copyStep,
  insertStep,
  movePath,
  nestedSteps,
  removePath,
  removeStep,
  setAwaitedForm,
  setUnknownPath,
  updateFork,
  updateWaitForEvent,
} from './flowEdits.ts'

const ROOT = stepId(1)
const UUID = /^[\da-f]{8}-[\da-f]{4}-4[\da-f]{3}-[89ab][\da-f]{3}-[\da-f]{12}$/

const STEPS: AutomationStep[] = [
  { id: stepId(11), type: 'wait', seconds: 60 },
  { id: stepId(12), type: 'apply_tag', tag: 'vip' },
]

function version(steps: AutomationStep[]): AutomationVersion {
  return {
    number: 1,
    trigger: { kind: 'event', action: 'paid', entryPolicy: 'once', cooldown: 3600 },
    exitEvents: [],
    goalEvent: null,
    steps: { id: ROOT, steps },
  }
}

test('a Step inserted on an edge goes there, with a new id of its own', () => {
  const flow = flowOf(version(STEPS))

  const between = insertStep(flow, { path: ROOT, at: 1 }, 'remove_tag')
  const first = insertStep(flow, { path: ROOT, at: 0 }, 'wait')
  const last = insertStep(flow, { path: ROOT, at: 2 }, 'apply_tag')

  expect(between.id).toMatch(UUID)
  expect(pathInputOf(between.flow)?.steps).toEqual([
    STEPS[0],
    { id: between.id, type: 'remove_tag', tag: '' },
    STEPS[1],
  ])
  expect(pathInputOf(first.flow)?.steps.map((step) => step.id)).toEqual([
    first.id,
    stepId(11),
    stepId(12),
  ])
  expect(pathInputOf(last.flow)?.steps.map((step) => step.id)).toEqual([
    stepId(11),
    stepId(12),
    last.id,
  ])
  expect(new Set([between.id, first.id, last.id]).size).toBe(3)
})

test('a new Step opens with what its kind starts with', () => {
  const flow = flowOf(version([]))

  const email = insertStep(flow, { path: ROOT, at: 0 }, 'email').flow.path.steps[0]
  const telegram = insertStep(flow, { path: ROOT, at: 0 }, 'telegram').flow.path.steps[0]
  const sms = insertStep(flow, { path: ROOT, at: 0 }, 'sms').flow.path.steps[0]
  const http = insertStep(flow, { path: ROOT, at: 0 }, 'http').flow.path.steps[0]
  const wait = insertStep(flow, { path: ROOT, at: 0 }, 'wait').flow.path.steps[0]

  expect(
    [email, telegram, sms, http, wait].map((step) => step && 'draft' in step && step.draft),
  ).toEqual([
    { integrationId: '', subject: '', body: '', doc: null },
    {
      integrationId: '',
      text: '',
      unsubscribeButton: '',
      imageUrl: '',
      buttons: [],
      linkPreview: true,
    },
    { integrationId: '', text: '', linkTracking: true },
    { stored: false, name: '', url: '', headers: [], body: '{}', secret: '' },
    { seconds: 3600 },
  ])
})

test('a removed Step leaves the path, the others keeping their order', () => {
  const flow = flowOf(version(STEPS))

  expect(pathInputOf(removeStep(flow, stepId(11)))?.steps).toEqual([STEPS[1]])
})

test('a copied Step follows its original with a new id', () => {
  const flow = flowOf(version(STEPS))

  const copied = copyStep(flow, stepId(12))

  expect(copied.id).toMatch(UUID)
  expect(pathInputOf(copied.flow)?.steps).toEqual([
    STEPS[0],
    STEPS[1],
    { id: copied.id, type: 'apply_tag', tag: 'vip' },
  ])
})

// A stored HTTP step's URL, secret and header values are kept by the server under its id, which a
// copy does not have, and its name is its own (ADR 0161): the copy asks for them as a new step does.
test('a copied HTTP step is not stored: it asks for its name, URL, secret and header values', () => {
  const flow = flowOf(
    version([
      {
        id: stepId(11),
        type: 'http',
        name: 'notify',
        body: '{}',
        headers: [{ name: 'X-Key' }],
        hasSecret: true,
      },
    ]),
  )

  const copied = copyStep(flow, stepId(11)).flow.path.steps[1]

  expect(copied).toMatchObject({
    kind: 'http',
    draft: { stored: false, name: '', url: '', headers: [{ name: 'X-Key' }], secret: '' },
  })
})

const VIP: BranchCondition = {
  kind: 'group',
  combinator: 'and',
  entries: [{ kind: 'tag', name: 'vip', operator: 'has' }],
}
const BUYER: BranchCondition = {
  kind: 'group',
  combinator: 'and',
  entries: [{ kind: 'tag', name: 'buyer', operator: 'has' }],
}

// A Branch on VIP holding a wait, its `otherwise` empty, then a tag step after it.
const BRANCHING: AutomationStep[] = [
  {
    id: stepId(20),
    type: 'branch',
    paths: [
      { id: stepId(21), condition: VIP, steps: [{ id: stepId(22), type: 'wait', seconds: 60 }] },
    ],
    otherwise: { id: stepId(23), steps: [] },
  },
  { id: stepId(24), type: 'apply_tag', tag: 'done' },
]

function branchOf(flow: Flow): BranchStep {
  const [branch] = flow.path.steps
  if (branch?.kind !== 'branch') {
    throw new Error('no Branch first')
  }
  return branch
}

test('a new Branch opens with one path to give a Condition and an empty otherwise', () => {
  const inserted = insertStep(flowOf(version([])), { path: ROOT, at: 0 }, 'branch')

  expect(inserted.flow.path.steps).toEqual([
    {
      id: inserted.id,
      kind: 'branch',
      paths: [{ id: expect.stringMatching(UUID), conditionText: '', steps: [] }],
      otherwise: { id: expect.stringMatching(UUID), steps: [] },
      unknown: null,
    },
  ])
})

test('a new Split opens with two paths of 50 each', () => {
  const inserted = insertStep(flowOf(version([])), { path: ROOT, at: 0 }, 'split')

  expect(pathInputOf(inserted.flow)?.steps).toEqual([
    {
      id: inserted.id,
      type: 'split',
      paths: [
        { id: expect.stringMatching(UUID), weight: 50, steps: [] },
        { id: expect.stringMatching(UUID), weight: 50, steps: [] },
      ],
    },
  ])
})

test('a Step inserted into an empty path of a fork goes there', () => {
  const flow = flowOf(version(BRANCHING))

  const inserted = insertStep(flow, { path: stepId(23), at: 0 }, 'end')

  expect(pathInputOf(inserted.flow)?.steps[0]).toMatchObject({
    otherwise: { id: stepId(23), steps: [{ id: inserted.id, type: 'end' }] },
  })
})

test("a Branch's path is added before otherwise, moved and removed", () => {
  const flow = flowOf(version(BRANCHING))

  const added = addPath(flow, stepId(20))
  const [, second] = branchOf(added.flow).paths
  const moved = movePath(added.flow, stepId(20), { pathId: added.id, by: -1 })
  const removed = removePath(moved, stepId(20), stepId(21))

  expect(second).toEqual({ id: added.id, conditionText: '', steps: [] })
  expect(branchOf(moved).paths.map((path) => path.id)).toEqual([added.id, stepId(21)])
  expect(branchOf(removed).paths.map((path) => path.id)).toEqual([added.id])
  expect(branchOf(removed).otherwise.id).toBe(stepId(23))
})

test("a fork's Conditions and weights are set by path id, its paths' Steps left as they are", () => {
  const flow = insertStep(flowOf(version(BRANCHING)), { path: stepId(21), at: 1 }, 'end').flow

  const edited = updateFork(flow, stepId(20), [
    { id: stepId(21), conditionText: JSON.stringify(BUYER) },
  ])

  expect(pathInputOf(edited)?.steps[0]).toMatchObject({
    paths: [{ id: stepId(21), condition: BUYER, steps: [{ type: 'wait' }, { type: 'end' }] }],
  })
})

test('a Branch gains an empty unknown path, and loses it', () => {
  const flow = flowOf(version(BRANCHING))

  const gained = setUnknownPath(flow, stepId(20), true)
  const lost = setUnknownPath(gained, stepId(20), false)

  expect(branchOf(gained).unknown).toEqual({ id: expect.stringMatching(UUID), steps: [] })
  expect(branchOf(lost).unknown).toBeNull()
})

test('removing a fork removes its paths; the Steps they hold are counted first', () => {
  const flow = flowOf(version(BRANCHING))

  expect(nestedSteps(branchOf(flow))).toBe(1)
  expect(pathInputOf(removeStep(flow, stepId(20)))?.steps).toEqual([BRANCHING[1]])
})

test('a Step inside a path is removed from it', () => {
  const flow = flowOf(version(BRANCHING))

  expect(nestedSteps(branchOf(removeStep(flow, stepId(22))))).toBe(0)
})

test('an End is not copied: it is the last Step of its path', () => {
  const flow = flowOf(version([...STEPS, { id: stepId(13), type: 'end' }]))

  expect(copyStep(flow, stepId(13))).toEqual({ flow, id: stepId(13) })
})

test('a copied fork follows its original, it and every path and Step in it with new ids', () => {
  const flow = flowOf(version(BRANCHING))

  const copied = copyStep(flow, stepId(20))
  const [original, copy] = pathInputOf(copied.flow)?.steps ?? []

  expect(copy).toEqual({
    ...original,
    id: copied.id,
    paths: [
      {
        id: expect.stringMatching(UUID),
        condition: VIP,
        steps: [{ id: expect.stringMatching(UUID), type: 'wait', seconds: 60 }],
      },
    ],
    otherwise: { id: expect.stringMatching(UUID), steps: [] },
  })
  const ids = JSON.stringify(copy).match(/[\da-f]{8}-[\da-f-]{27}/g) ?? []
  expect(ids.some((id) => JSON.stringify(original).includes(id))).toBe(false)
})

const PURCHASE: AutomationStep = {
  id: stepId(40),
  type: 'wait_for_event',
  name: 'purchase',
  timeoutSeconds: 3600,
  awaits: { kind: 'event', matchers: [{ action: 'order.created' }] },
  arrived: { id: stepId(41), steps: [{ id: stepId(43), type: 'apply_tag', tag: 'buyer' }] },
  timedOut: { id: stepId(42), steps: [] },
}

// A name belongs to one Step id (ADR 0136): a copy, which has an id of its own, asks for its own.
test('a copied Wait for event asks for a name, its paths and Steps with new ids', () => {
  const copied = copyStep(flowOf(version([PURCHASE])), stepId(40))
  const copy = copied.flow.path.steps[1]

  expect(copy).toEqual({
    id: copied.id,
    kind: 'wait_for_event',
    draft: {
      name: '',
      timeoutSeconds: 3600,
      form: 'event',
      matchers: [{ action: 'order.created', filterText: '' }],
      conditionText: '',
      messageStep: '',
      engagement: 'opened',
    },
    arrived: {
      id: expect.stringMatching(UUID),
      steps: [{ id: expect.stringMatching(UUID), kind: 'apply_tag', draft: { tag: 'buyer' } }],
    },
    timedOut: { id: expect.stringMatching(UUID), steps: [] },
    unknown: null,
  })
})

test("a Wait for event's panel sets its draft, its paths' Steps left as they are", () => {
  const flow = flowOf(version([PURCHASE]))
  const draft = {
    name: 'purchase',
    timeoutSeconds: 7200,
    form: 'event' as const,
    matchers: [{ action: 'call.booked', filterText: '' }],
    conditionText: '',
    messageStep: '',
    engagement: 'opened' as const,
  }

  const edited = updateWaitForEvent(flow, stepId(40), draft)

  expect(pathInputOf(edited)?.steps).toEqual([
    {
      ...PURCHASE,
      timeoutSeconds: 7200,
      awaits: { kind: 'event', matchers: [{ action: 'call.booked' }] },
    },
  ])
})

const WELCOME: AutomationStep = {
  id: stepId(30),
  type: 'email',
  subject: 'Welcome',
  body: '<p></p>',
}

// A Wait for event on an open or click (ADR 0124) has an unknown path; one on an Event never has.
// Switching keeps what the other form holds.
test('switching a Wait for event to an open or click adds an empty unknown path, and back removes it', () => {
  const flow = flowOf(version([WELCOME, PURCHASE]))

  const onClick = updateWaitForEvent(setAwaitedForm(flow, stepId(40), 'message'), stepId(40), {
    name: 'purchase',
    timeoutSeconds: 3600,
    form: 'message',
    matchers: [{ action: 'order.created', filterText: '' }],
    conditionText: '',
    messageStep: stepId(30),
    engagement: 'clicked',
  })
  // The Wait for event's paths, the only fork's: `arrived`, `timedOut` and the unknown path.
  const [, , unknown] = onClick.path.steps.flatMap((step) => forkPaths(step))
  const filled = insertStep(onClick, { path: unknown?.id ?? '', at: 0 }, 'apply_tag').flow
  const back = setAwaitedForm(filled, stepId(40), 'event')

  expect(pathInputOf(filled)?.steps[1]).toEqual({
    ...PURCHASE,
    awaits: { kind: 'message', step: stepId(30), engagement: 'clicked' },
    unknown: {
      id: expect.stringMatching(UUID),
      steps: [{ id: expect.stringMatching(UUID), type: 'apply_tag', tag: '' }],
    },
  })
  expect(pathInputOf(back)?.steps[1]).toEqual(PURCHASE)
})

test('a copied Wait for event on an open copies its unknown path with new ids', () => {
  const onOpen: AutomationStep = {
    ...PURCHASE,
    awaits: { kind: 'message', step: stepId(30), engagement: 'opened' },
    unknown: { id: stepId(44), steps: [{ id: stepId(45), type: 'wait', seconds: 60 }] },
  }
  const flow = flowOf(version([WELCOME, onOpen]))

  const copied = copyStep(flow, stepId(40))
  const copy = pathInputOf(copied.flow)?.steps[2]

  expect(copy).toMatchObject({
    awaits: { kind: 'message', step: stepId(30), engagement: 'opened' },
    unknown: {
      id: expect.stringMatching(UUID),
      steps: [{ id: expect.stringMatching(UUID), type: 'wait', seconds: 60 }],
    },
  })
  expect(JSON.stringify(copy)).not.toContain(stepId(44))
  expect(JSON.stringify(copy)).not.toContain(stepId(45))
})
