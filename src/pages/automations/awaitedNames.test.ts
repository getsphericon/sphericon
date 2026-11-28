import { expect, test } from 'vitest'

import type {
  AutomationStep,
  AutomationVersion,
  BranchCondition,
} from '../../generated/site/types.gen.ts'
import { stepId } from '../../test/payloads.ts'
import { awaitedBefore, httpBefore, keptWarnings } from './awaitedNames.ts'
import { flowOf } from './definition.ts'

// The Steps a version holds, as the editor opens them.
function rootOf(steps: AutomationStep[]) {
  const version: AutomationVersion = {
    number: 1,
    trigger: { kind: 'event', action: 'signed_up', entryPolicy: 'once', cooldown: 3600 },
    exitEvents: [],
    goalEvent: null,
    steps: { id: stepId(1), steps },
  }
  return flowOf(version).path
}

// A Wait for event on order.created named `name`, its paths holding the Steps given.
function waitFor(
  id: number,
  name: string,
  { arrived = [], timedOut = [] }: { arrived?: AutomationStep[]; timedOut?: AutomationStep[] } = {},
): AutomationStep {
  return {
    id: stepId(id),
    type: 'wait_for_event',
    name,
    timeoutSeconds: 86_400,
    awaits: { kind: 'event', matchers: [{ action: 'order.created' }] },
    arrived: { id: stepId(id + 1), steps: arrived },
    timedOut: { id: stepId(id + 2), steps: timedOut },
  }
}

function email(id: number, body: string): AutomationStep {
  return { id: stepId(id), type: 'email', subject: 'Thanks', body }
}

const TAG: AutomationStep = { id: stepId(90), type: 'apply_tag', tag: 'vip' }

const VIP: BranchCondition = {
  kind: 'group',
  combinator: 'and',
  entries: [{ kind: 'tag', name: 'vip', operator: 'has' }],
}

// A Branch on its Condition (the Contact's Tag): its first path holds `listed`, `otherwise` holds
// `otherwise`.
function branch(
  id: number,
  listed: AutomationStep[],
  {
    otherwise = [],
    condition = VIP,
  }: { otherwise?: AutomationStep[]; condition?: BranchCondition } = {},
): AutomationStep {
  return {
    id: stepId(id),
    type: 'branch',
    paths: [{ id: stepId(id + 1), condition, steps: listed }],
    otherwise: { id: stepId(id + 2), steps: otherwise },
  }
}

// A Branch path's test of the Event the Wait for event `name` arrived on.
function awaitedTest(name: string): BranchCondition {
  return {
    kind: 'group',
    combinator: 'and',
    entries: [
      {
        kind: 'property',
        of: { kind: 'awaited', name },
        key: 'total',
        test: { kind: 'number', operator: 'gt', value: 100 },
      },
    ],
  }
}

test('a Step is offered the Wait for events that can come before it, and none after it', () => {
  const root = rootOf([
    email(10, 'Hi'),
    waitFor(20, 'purchase', { arrived: [email(30, 'Thanks')], timedOut: [email(31, 'Reminder')] }),
    branch(40, [waitFor(50, 'booking')]),
    email(60, 'Bye'),
  ])

  expect(awaitedBefore(root, stepId(10))).toEqual([])
  expect(awaitedBefore(root, stepId(30))).toEqual(['purchase'])
  expect(awaitedBefore(root, stepId(31))).toEqual(['purchase'])
  expect(awaitedBefore(root, stepId(60))).toEqual(['purchase', 'booking'])
})

test('a Wait for event whose name is blank or of the wrong form is offered to none', () => {
  const root = rootOf([waitFor(20, ''), waitFor(30, 'Bad name'), email(60, 'Bye')])

  expect(awaitedBefore(root, stepId(60))).toEqual([])
})

test('a Step using a name a path before it skips is warned of, one past it on every path is not', () => {
  const root = rootOf([
    branch(10, [waitFor(20, 'purchase')]),
    email(30, '<p>{{ awaited.purchase.properties.product }}</p>'),
    waitFor(40, 'booking', {
      arrived: [email(50, '{{awaited.booking.action}}')],
      timedOut: [email(51, '{{ awaited.booking.action }}')],
    }),
    email(60, '{% if awaited.booking.properties.paid %}Paid{% endif %}'),
  ])

  expect(keptWarnings(root)).toEqual(new Map([[stepId(30), ['awaited.purchase']]]))
})

test('a name used before its Wait for event, or by none, is warned of; plain text naming it is not', () => {
  const root = rootOf([
    email(10, '{{ awaited.purchase.action }} {{ awaited.nobody.action }}'),
    email(11, 'Your awaited.purchase arrives soon'),
    waitFor(20, 'purchase'),
  ])

  expect(keptWarnings(root)).toEqual(
    new Map([[stepId(10), ['awaited.purchase', 'awaited.nobody']]]),
  )
})

test('a path that ends before the rest does not count among the paths before a Step', () => {
  const root = rootOf([
    branch(10, [waitFor(20, 'purchase')], { otherwise: [{ id: stepId(25), type: 'end' }] }),
    email(30, '{{ awaited.purchase.properties.product }}'),
  ])

  expect(keptWarnings(root)).toEqual(new Map())
})

test("a Branch's test of an awaited Event a path before it skips is warned of", () => {
  const root = rootOf([
    branch(10, [waitFor(20, 'purchase')]),
    branch(30, [TAG], { condition: awaitedTest('purchase') }),
    waitFor(40, 'booking'),
    branch(50, [], { condition: awaitedTest('booking') }),
  ])

  expect(keptWarnings(root)).toEqual(new Map([[stepId(30), ['awaited.purchase']]]))
})

test('an HTTP step, an SMS and a Telegram message using a name are read too', () => {
  const root = rootOf([
    {
      id: stepId(10),
      type: 'http',
      name: 'notify',
      body: '{"total": {{ awaited.purchase.properties.total }}}',
      headers: [],
      hasSecret: false,
    },
    { id: stepId(11), type: 'sms', text: '{{ awaited.purchase.action }}', linkTracking: true },
    {
      id: stepId(12),
      type: 'telegram',
      text: '{{ awaited.purchase.action }}',
      buttons: [],
      linkPreview: true,
    },
    waitFor(20, 'purchase'),
  ])

  expect(keptWarnings(root)).toEqual(
    new Map([
      [stepId(10), ['awaited.purchase']],
      [stepId(11), ['awaited.purchase']],
      [stepId(12), ['awaited.purchase']],
    ]),
  )
})

test('a Wait for event on an open or click keeps no Event: none offers its name, and it passes none', () => {
  const opened: AutomationStep = {
    id: stepId(20),
    type: 'wait_for_event',
    name: 'opened',
    timeoutSeconds: 86_400,
    awaits: { kind: 'message', step: stepId(10), engagement: 'opened' },
    arrived: { id: stepId(21), steps: [] },
    timedOut: { id: stepId(22), steps: [] },
    unknown: { id: stepId(23), steps: [] },
  }
  const root = rootOf([email(10, 'Hi'), opened, email(30, '{{ awaited.opened.action }}')])

  expect(awaitedBefore(root, stepId(30))).toEqual([])
  expect(keptWarnings(root)).toEqual(new Map([[stepId(30), ['awaited.opened']]]))
})

// An HTTP step named `name`, posting body.
function call(id: number, name: string, body = '{}'): AutomationStep {
  return { id: stepId(id), type: 'http', name, body, headers: [], hasSecret: false }
}

// A Branch path's test of the code the HTTP step `name` kept (ADR 0161).
function responseTest(name: string): BranchCondition {
  return {
    kind: 'group',
    combinator: 'and',
    entries: [
      {
        kind: 'property',
        of: { kind: 'http', name },
        key: 'code',
        test: { kind: 'text', operator: 'eq', value: 'X1' },
      },
    ],
  }
}

test('a Step is offered the HTTP steps that can come before it, apart from the Wait for events', () => {
  const root = rootOf([
    call(10, 'coupon'),
    waitFor(20, 'purchase', { arrived: [call(30, 'loyalty')] }),
    email(40, 'Bye'),
  ])

  expect(httpBefore(root, stepId(20))).toEqual(['coupon'])
  expect(httpBefore(root, stepId(40))).toEqual(['coupon', 'loyalty'])
  expect(awaitedBefore(root, stepId(40))).toEqual(['purchase'])
})

test("a read of an HTTP step's response a path before it skips is warned of, in content and in a Branch", () => {
  const root = rootOf([
    branch(10, [call(20, 'coupon')]),
    email(30, '<p>{{ http.coupon.body.code }}</p>'),
    branch(40, [TAG], { condition: responseTest('coupon') }),
    call(50, 'loyalty'),
    email(60, '{{ http.loyalty.status }}'),
  ])

  expect(keptWarnings(root)).toEqual(
    new Map([
      [stepId(30), ['http.coupon']],
      [stepId(40), ['http.coupon']],
    ]),
  )
})
