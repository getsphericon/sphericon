import { expect, test } from 'vitest'

import type { AutomationStep, AutomationVersion } from '../../generated/site/types.gen.ts'
import { stepId } from '../../test/payloads.ts'
import { flowOf, pathInputOf } from './definition.ts'
import { copyStep, insertStep, removeStep } from './flowEdits.ts'

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
    steps: { id: stepId(1), steps },
  }
}

test('a Step inserted on an edge goes there, with a new id of its own', () => {
  const flow = flowOf(version(STEPS))

  const between = insertStep(flow, 1, 'remove_tag')
  const first = insertStep(flow, 0, 'wait')
  const last = insertStep(flow, 2, 'apply_tag')

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

  const email = insertStep(flow, 0, 'email').flow.path.steps[0]
  const telegram = insertStep(flow, 0, 'telegram').flow.path.steps[0]
  const sms = insertStep(flow, 0, 'sms').flow.path.steps[0]
  const http = insertStep(flow, 0, 'http').flow.path.steps[0]
  const wait = insertStep(flow, 0, 'wait').flow.path.steps[0]

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
    { stored: false, url: '', headers: [], body: '{}', secret: '' },
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
// copy does not have: the copy asks for them as a new step does.
test('a copied HTTP step is not stored: it asks for its URL, secret and header values', () => {
  const flow = flowOf(
    version([
      { id: stepId(11), type: 'http', body: '{}', headers: [{ name: 'X-Key' }], hasSecret: true },
    ]),
  )

  const copied = copyStep(flow, stepId(11)).flow.path.steps[1]

  expect(copied).toMatchObject({
    kind: 'http',
    draft: { stored: false, url: '', headers: [{ name: 'X-Key' }], secret: '' },
  })
})
