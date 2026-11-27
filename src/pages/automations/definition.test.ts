import { expect, test } from 'vitest'

import type {
  AutomationStep,
  AutomationVersion,
  BranchCondition,
  EventFilterInput,
} from '../../generated/site/types.gen.ts'
import { stepId, testId } from '../../test/payloads.ts'
import {
  cooldownHours,
  cooldownSeconds,
  definitionOf,
  flowOf,
  holdsKept,
  pathInputOf,
  stepsBefore,
  withoutHalfTypedFilter,
} from './definition.ts'
import type { Flow, TriggerDraft } from './definition.ts'
import { automationSnapshot } from './leaveSnapshot.ts'

const ROOT = stepId(1)

const STEPS: AutomationStep[] = [
  { id: stepId(11), type: 'email', subject: 'Hello', body: '<p></p>' },
  { id: stepId(12), type: 'wait', seconds: 3600 },
  { id: stepId(13), type: 'apply_tag', tag: 'vip' },
  { id: stepId(14), type: 'remove_tag', tag: 'lead' },
]

// A version on the event, with no exit and no goal event, holding the steps.
function version(steps: AutomationStep[], action = 'email.opened'): AutomationVersion {
  return {
    number: 1,
    trigger: { kind: 'event', action, entryPolicy: 'once', cooldown: 3600 },
    exitEvents: [],
    goalEvent: null,
    steps: { id: ROOT, steps },
  }
}

// A flow whose Trigger is the draft given, over no steps.
function triggered(trigger: Partial<TriggerDraft>): Flow {
  const flow = flowOf(version([], 'paid'))
  return { ...flow, trigger: { ...flow.trigger, ...trigger } }
}

test('a stored version round-trips through the flow, each Step and the path keeping its id', () => {
  expect(definitionOf(flowOf(version(STEPS, 'email.clicked')))).toEqual({
    trigger: {
      sends: { kind: 'event', action: 'email.clicked', entryPolicy: 'once', cooldown: 3600 },
    },
    filterIncomplete: false,
    exitEvents: [],
    goalEvent: null,
    steps: { sends: { id: ROOT, steps: STEPS } },
  })
  expect(pathInputOf(flowOf(version(STEPS)))).toEqual({ id: ROOT, steps: STEPS })
})

test('a stored step opens with what it leaves out filled in, and saves without its defaults', () => {
  const flow = flowOf(
    version([
      { id: stepId(11), type: 'telegram', text: 'Hi', buttons: [], linkPreview: true },
      { id: stepId(12), type: 'telegram', text: 'Hi', buttons: [], linkPreview: false },
      { id: stepId(13), type: 'sms', text: 'Hi', linkTracking: true },
      { id: stepId(14), type: 'sms', text: 'Hi', linkTracking: false },
    ]),
  )

  expect(flow.path.steps.map((step) => ('draft' in step ? step.draft : null))).toEqual([
    {
      integrationId: '',
      text: 'Hi',
      unsubscribeButton: '',
      imageUrl: '',
      buttons: [],
      linkPreview: true,
    },
    {
      integrationId: '',
      text: 'Hi',
      unsubscribeButton: '',
      imageUrl: '',
      buttons: [],
      linkPreview: false,
    },
    { integrationId: '', text: 'Hi', linkTracking: true },
    { integrationId: '', text: 'Hi', linkTracking: false },
  ])
  expect(pathInputOf(flow)?.steps).toEqual([
    { id: stepId(11), type: 'telegram', text: 'Hi' },
    { id: stepId(12), type: 'telegram', text: 'Hi', linkPreview: false },
    { id: stepId(13), type: 'sms', text: 'Hi' },
    { id: stepId(14), type: 'sms', text: 'Hi', linkTracking: false },
  ])
})

const VIP: BranchCondition = {
  kind: 'group',
  combinator: 'and',
  entries: [{ kind: 'tag', name: 'vip', operator: 'has' }],
}

// A tree of every fork kind and an End (ADR 0124): a Branch with an unknown path, a Split nested in
// one of its paths, an End finishing another.
const TREE: AutomationStep[] = [
  {
    id: stepId(20),
    type: 'branch',
    paths: [
      {
        id: stepId(21),
        condition: VIP,
        steps: [
          {
            id: stepId(22),
            type: 'split',
            paths: [
              {
                id: stepId(23),
                weight: 30,
                steps: [{ id: stepId(24), type: 'wait', seconds: 60 }],
              },
              { id: stepId(25), weight: 70, steps: [] },
            ],
          },
        ],
      },
    ],
    otherwise: { id: stepId(26), steps: [{ id: stepId(27), type: 'end' }] },
    unknown: { id: stepId(28), steps: [] },
  },
  { id: stepId(29), type: 'apply_tag', tag: 'done' },
]

test('a version holding forks and an End round-trips, every Step and path keeping its id', () => {
  const flow = flowOf(version(TREE))

  expect(holdsKept(flow)).toBe(false)
  expect(pathInputOf(flow)).toEqual({ id: ROOT, steps: TREE })
  expect(definitionOf(flow).steps).toEqual({ sends: { id: ROOT, steps: TREE } })
})

// A stored Condition is saved back as read, whatever its leaves: the read shape of each leaf is
// the shape a request writes.
test("a Branch path's stored Condition of every leaf kind is saved back as read", () => {
  const condition: BranchCondition = {
    kind: 'group',
    combinator: 'or',
    not: true,
    entries: [
      { kind: 'event', action: 'email.opened', operator: 'atMost', times: 2, withinDays: 7 },
      { kind: 'birthday', test: { kind: 'withinLast', days: 3 } },
      {
        kind: 'customField',
        key: 'renews_at',
        test: { kind: 'moment', operator: 'before', at: '2026-01-01T00:00:00Z' },
      },
      {
        kind: 'attribute',
        attribute: 'email',
        test: { kind: 'text', operator: 'contains', value: '@x' },
      },
      {
        kind: 'engagementTracking',
        channel: 'email',
        test: { kind: 'text', operator: 'eq', value: 'granted' },
      },
      { kind: 'telegram', operator: 'has' },
      {
        kind: 'group',
        combinator: 'and',
        entries: [
          {
            kind: 'property',
            of: { kind: 'trigger' },
            key: 'total',
            test: { kind: 'number', operator: 'gt', value: 100 },
          },
        ],
      },
    ],
  }
  const branch: AutomationStep = {
    id: stepId(20),
    type: 'branch',
    paths: [{ id: stepId(21), condition, steps: [] }],
    otherwise: { id: stepId(22), steps: [] },
  }

  expect(pathInputOf(flowOf(version([branch])))).toEqual({ id: ROOT, steps: [branch] })
})

// A kind newer than the editor is kept as stored, at any depth: the version is shown, never saved,
// so nothing can drop it.
test('a Step of a kind the editor does not know, inside a fork path, keeps the version unsaved', () => {
  // A kind the contract may gain, read as the server would send it.
  const newer: AutomationStep = JSON.parse(
    JSON.stringify({ id: stepId(30), type: 'wait_for_reply' }),
  )
  const split: AutomationStep = {
    id: stepId(15),
    type: 'split',
    paths: [
      { id: stepId(16), weight: 30, steps: [newer] },
      { id: stepId(18), weight: 70, steps: [] },
    ],
  }

  const flow = flowOf(version([...STEPS, split]))

  expect(holdsKept(flow)).toBe(true)
  expect(pathInputOf(flow)).toBeNull()
})

// A Wait for event on an Event (ADR 0124, ADR 0180) round-trips: its name, timeout, matchers with
// their filters, Condition and both paths, each with its id.
test('a Wait for event round-trips with its matchers, Condition and paths', () => {
  const wait: AutomationStep = {
    id: stepId(40),
    type: 'wait_for_event',
    name: 'purchase',
    timeoutSeconds: 259_200,
    awaits: {
      kind: 'event',
      matchers: [
        {
          action: 'order.created',
          filter: {
            kind: 'group',
            combinator: 'and',
            entries: [
              {
                kind: 'property',
                key: 'total',
                test: { kind: 'number', operator: 'gt', value: 100 },
              },
            ],
          },
        },
        { action: 'call.booked' },
      ],
      condition: {
        kind: 'group',
        combinator: 'and',
        entries: [{ kind: 'tag', name: 'vip', operator: 'has' }],
      },
    },
    arrived: { id: stepId(41), steps: [{ id: stepId(43), type: 'apply_tag', tag: 'buyer' }] },
    timedOut: { id: stepId(42), steps: [] },
  }
  const plain: AutomationStep = {
    ...wait,
    id: stepId(44),
    name: 'reply',
    awaits: { kind: 'event', matchers: [{ action: 'call.booked' }] },
    arrived: { id: stepId(45), steps: [] },
    timedOut: { id: stepId(46), steps: [] },
  }

  const flow = flowOf(version([wait, plain]))

  expect(holdsKept(flow)).toBe(false)
  expect(pathInputOf(flow)).toEqual({ id: ROOT, steps: [wait, plain] })
})

// The editor edits a Wait for event on an Event; one on an open or click of a message is kept as
// stored, so nothing can drop it.
test('a Wait for event on an open or click keeps the version unsaved', () => {
  const onOpen: AutomationStep = {
    id: stepId(44),
    type: 'wait_for_event',
    name: 'opened',
    timeoutSeconds: 3600,
    awaits: { kind: 'message', step: stepId(11), engagement: 'opened' },
    arrived: { id: stepId(45), steps: [] },
    timedOut: { id: stepId(46), steps: [] },
    unknown: { id: stepId(47), steps: [] },
  }

  const flow = flowOf(version([...STEPS, onOpen]))

  expect(holdsKept(flow)).toBe(true)
  expect(pathInputOf(flow)).toBeNull()
})

// A Wait until (ADR 0126) round-trips in each of its forms, every Step keeping its id.
test('a Wait until on a time of day or on a date round-trips', () => {
  const waits: AutomationStep[] = [
    {
      id: stepId(31),
      type: 'wait_until',
      until: { form: 'time_of_day', localTime: '09:00', weekdays: ['monday', 'friday'] },
    },
    {
      id: stepId(32),
      type: 'wait_until',
      until: {
        form: 'date',
        field: { kind: 'customField', key: 'trial_ends_at' },
        recurrence: 'once',
        offsetDays: -3,
        localTime: '10:00',
      },
    },
    {
      id: stepId(33),
      type: 'wait_until',
      until: {
        form: 'date',
        field: { kind: 'birthday' },
        recurrence: 'yearly',
        offsetDays: 0,
        localTime: '08:30',
      },
    },
  ]

  const flow = flowOf(version(waits))

  expect(holdsKept(flow)).toBe(false)
  expect(pathInputOf(flow)).toEqual({ id: ROOT, steps: waits })
})

// A Wait until whose moment is not set yet (no local time, no date) is never saved.
test('a Wait until without its local time or its date keeps the version unsaved', () => {
  const flow = flowOf(version([]))
  const pending: Flow = {
    ...flow,
    path: {
      ...flow.path,
      steps: [
        {
          id: stepId(31),
          kind: 'wait_until',
          draft: {
            form: 'time_of_day',
            timeOfDay: { localTime: '', weekdays: [] },
            date: { field: null, recurrence: 'yearly', days: 0, sign: 1, localTime: '' },
          },
        },
      ],
    },
  }

  expect(pathInputOf(pending)).toBeNull()
})

// A date Trigger (ADR 0126) round-trips, its offset read as days before or after the date.
test('a date Trigger round-trips, its offset as days before or after', () => {
  const stored: AutomationVersion = {
    ...version([]),
    trigger: {
      kind: 'date',
      field: { kind: 'customField', key: 'trial_ends_at' },
      recurrence: 'once',
      offsetDays: -3,
      localTime: '10:00',
    },
  }

  expect(flowOf(stored).trigger).toMatchObject({
    kind: 'date',
    date: {
      field: { kind: 'customField', key: 'trial_ends_at' },
      recurrence: 'once',
      days: 3,
      sign: -1,
      localTime: '10:00',
    },
  })
  expect(definitionOf(flowOf(stored))).toMatchObject({
    trigger: { sends: stored.trigger },
    filterIncomplete: false,
  })
})

test('an email step written in the editor is saved as its document alone', () => {
  const doc = { type: 'doc', content: [{ type: 'paragraph' }] }
  const stored: AutomationStep = {
    id: stepId(11),
    type: 'email',
    subject: 'Hi',
    body: '<p>built by the server</p>',
    doc,
    editorVersion: '1.7.14',
  }

  expect(pathInputOf(flowOf(version([stored])))?.steps).toEqual([
    { id: stepId(11), type: 'email', subject: 'Hi', doc },
  ])
})

test('an email step with no document is saved as its body alone', () => {
  const flow = flowOf(
    version([{ id: stepId(11), type: 'email', subject: 'Hi', body: '<p>source</p>' }]),
  )

  expect(pathInputOf(flow)?.steps).toEqual([
    { id: stepId(11), type: 'email', subject: 'Hi', body: '<p>source</p>' },
  ])
})

test('a wait whose seconds are blank or fractional saves whole seconds', () => {
  const flow = flowOf(version([{ id: stepId(11), type: 'wait', seconds: 60 }]))
  const blank: Flow = {
    ...flow,
    path: { ...flow.path, steps: [{ id: stepId(11), kind: 'wait', draft: { seconds: '' } }] },
  }
  const fractional: Flow = {
    ...flow,
    path: { ...flow.path, steps: [{ id: stepId(11), kind: 'wait', draft: { seconds: 90.7 } }] },
  }

  expect(pathInputOf(blank)?.steps).toEqual([{ id: stepId(11), type: 'wait', seconds: 0 }])
  expect(pathInputOf(fractional)?.steps).toEqual([{ id: stepId(11), type: 'wait', seconds: 90 }])
})

test('an HTTP step keeps its id and header names but never carries a secret into the editor', () => {
  const stored: AutomationStep = {
    id: stepId(11),
    type: 'http',
    body: '{"a":1}',
    headers: [{ name: 'Authorization' }, { name: 'X-Key' }],
    hasSecret: true,
  }
  const flow = flowOf(version([stored]))

  expect(flow.path.steps).toEqual([
    {
      id: stepId(11),
      kind: 'http',
      draft: {
        stored: true,
        url: '',
        headers: [{ name: 'Authorization' }, { name: 'X-Key' }],
        body: '{"a":1}',
        secret: '',
      },
    },
  ])
  // Resaving untouched resends no URL, header value or secret, so the server keeps them, matched
  // by the Step's id.
  expect(pathInputOf(flow)?.steps).toEqual([
    {
      id: stepId(11),
      type: 'http',
      body: '{"a":1}',
      headers: [{ name: 'Authorization' }, { name: 'X-Key' }],
    },
  ])
})

test('an HTTP step with a new URL, header value and secret sends them, and drops a blank row', () => {
  const flow = flowOf(version([]))
  const edited: Flow = {
    ...flow,
    path: {
      ...flow.path,
      steps: [
        {
          id: stepId(11),
          kind: 'http',
          draft: {
            stored: false,
            url: ' https://hooks.example.com/x ',
            headers: [
              { name: 'Authorization', value: 'Bearer a:b' },
              { name: '', value: '' },
              { name: ' X-Key ', value: 'k' },
              { name: 'X-Kept', value: '' },
            ],
            body: '{}',
            secret: 'whsec_x',
          },
        },
      ],
    },
  }

  expect(pathInputOf(edited)?.steps).toEqual([
    {
      id: stepId(11),
      type: 'http',
      body: '{}',
      url: 'https://hooks.example.com/x',
      headers: [
        { name: 'Authorization', value: 'Bearer a:b' },
        { name: 'X-Key', value: 'k' },
        { name: 'X-Kept' },
      ],
      secret: 'whsec_x',
    },
  ])
})

test('each email step keeps the Integration it names, and one that names none sends through the default', () => {
  const steps: AutomationStep[] = [
    { id: stepId(11), type: 'email', integrationId: testId(21), subject: 'A', body: '<p>1</p>' },
    { id: stepId(12), type: 'email', subject: 'B', body: '<p>2</p>' },
  ]

  expect(pathInputOf(flowOf(version(steps)))).toEqual({ id: ROOT, steps })
})

test("the saved snapshot changes with the name, the trigger, the steps or a step's Integration", () => {
  const saved = automationSnapshot('Welcome', flowOf(version(STEPS, 'contact.created')))
  const throughRelay: AutomationStep[] = [
    { id: stepId(11), type: 'email', subject: 'Hello', body: '<p></p>', integrationId: testId(21) },
    ...STEPS.slice(1),
  ]

  const trimmed = automationSnapshot('  Welcome ', flowOf(version(STEPS, 'contact.created')))
  const renamed = automationSnapshot('Welcome 2', flowOf(version(STEPS, 'contact.created')))
  const retriggered = automationSnapshot('Welcome', flowOf(version(STEPS, 'email.opened')))
  const shorter = automationSnapshot('Welcome', flowOf(version(STEPS.slice(1), 'contact.created')))
  const relayed = automationSnapshot('Welcome', flowOf(version(throughRelay, 'contact.created')))

  expect(trimmed).toBe(saved)
  expect([renamed, retriggered, shorter, relayed].filter((other) => other === saved)).toEqual([])
})

const PAID: EventFilterInput = {
  kind: 'group',
  combinator: 'and',
  entries: [
    { kind: 'property', key: 'status', test: { kind: 'text', operator: 'eq', value: 'paid' } },
  ],
}
const CANCELLED: EventFilterInput = {
  ...PAID,
  entries: [
    { kind: 'property', key: 'status', test: { kind: 'text', operator: 'eq', value: 'cancelled' } },
  ],
}
const BLANK_KEY = JSON.stringify({
  ...PAID,
  entries: [{ kind: 'property', test: { kind: 'text', operator: 'eq', value: 'paid' } }],
})

test("the Trigger's filter, Entry policy, cooldown and Enrollment key round-trip", () => {
  const stored: AutomationVersion = {
    ...version([], 'order.completed'),
    trigger: {
      kind: 'event',
      action: 'order.completed',
      filter: PAID,
      entryPolicy: 'again',
      cooldown: 7200,
      enrollmentKey: 'order_id',
    },
  }

  expect(flowOf(stored).trigger).toMatchObject({
    filterText: JSON.stringify(PAID),
    entryPolicy: 'again',
    cooldown: 7200,
    enrollmentKey: 'order_id',
  })
  expect(definitionOf(flowOf(stored))).toMatchObject({
    trigger: { sends: stored.trigger },
    filterIncomplete: false,
  })
})

test('a Trigger with no filter, cooldown or Enrollment key leaves them to the server', () => {
  const { trigger } = definitionOf(
    triggered({ filterText: '', cooldown: null, enrollmentKey: '  ' }),
  )

  expect(trigger).toEqual({ sends: { kind: 'event', action: 'paid', entryPolicy: 'once' } })
})

test('a filter still being typed is flagged and left out of the saved Trigger', () => {
  const saved = definitionOf(triggered({ filterText: BLANK_KEY, cooldown: null }))

  expect(saved.filterIncomplete).toBe(true)
  expect(saved.trigger).toEqual({ sends: { kind: 'event', action: 'paid', entryPolicy: 'once' } })
})

test('the cooldown is typed in hours and held in seconds', () => {
  expect(cooldownSeconds(3)).toBe(10_800)
  expect(cooldownHours(10_800)).toBe(3)
  expect(cooldownSeconds('')).toBeNull()
  expect(cooldownHours(null)).toBe('')
})

test('the exit and goal events round-trip as matchers', () => {
  const stored: AutomationVersion = {
    ...version(STEPS, 'order.placed'),
    exitEvents: [{ action: 'account_closed' }, { action: 'order.closed', filter: CANCELLED }],
    goalEvent: { action: 'order.closed', filter: PAID },
  }

  expect(flowOf(stored).trigger).toMatchObject({
    exits: [
      { action: 'account_closed', filterText: '' },
      { action: 'order.closed', filterText: JSON.stringify(CANCELLED) },
    ],
    goal: { action: 'order.closed', filterText: JSON.stringify(PAID) },
  })
  expect(definitionOf(flowOf(stored))).toMatchObject({
    exitEvents: stored.exitEvents,
    goalEvent: stored.goalEvent,
  })
})

test('an exit with a blank action is dropped, a goal with one is none, and actions are trimmed', () => {
  const flow = triggered({
    exits: [
      { action: ' refunded ', filterText: '' },
      { action: '  ', filterText: '' },
    ],
    goal: { action: ' ', filterText: '' },
  })

  expect(definitionOf(flow)).toMatchObject({
    exitEvents: [{ action: 'refunded' }],
    goalEvent: null,
  })
})

test("an exit's or the goal's filter still being typed is flagged and left out of what is saved", () => {
  const exitTyping = triggered({ exits: [{ action: 'order.closed', filterText: BLANK_KEY }] })
  const goalTyping = triggered({ goal: { action: 'order.closed', filterText: BLANK_KEY } })

  expect(definitionOf(exitTyping)).toMatchObject({
    filterIncomplete: true,
    exitEvents: [{ action: 'order.closed' }],
  })
  expect(definitionOf(goalTyping)).toMatchObject({
    filterIncomplete: true,
    goalEvent: { action: 'order.closed' },
  })
})

test('a half-typed filter of an exit or goal is dropped, a complete one kept', () => {
  const halfTyped = { action: 'order.closed', filterText: '{"kind":"group","entries":[{}' }
  const complete = { action: 'order.closed', filterText: JSON.stringify(PAID) }

  expect(withoutHalfTypedFilter(halfTyped)).toEqual({ action: 'order.closed', filterText: '' })
  expect(withoutHalfTypedFilter(complete)).toEqual(complete)
})

// A Trigger switched to a date saves the date alone: the Event's action, its filter (complete or
// half-typed), its Entry policy, cooldown and Enrollment key stay in the draft, unsent.
test('a Trigger switched to a date saves no Event matcher or entry settings', () => {
  const saved = definitionOf(
    triggered({
      kind: 'date',
      filterText: BLANK_KEY,
      entryPolicy: 'again',
      cooldown: 7200,
      enrollmentKey: 'order_id',
      date: {
        field: { kind: 'birthday' },
        recurrence: 'yearly',
        days: 0,
        sign: -1,
        localTime: '10:00',
      },
    }),
  )

  expect(saved.filterIncomplete).toBe(false)
  expect(saved.trigger).toEqual({
    sends: {
      kind: 'date',
      field: { kind: 'birthday' },
      recurrence: 'yearly',
      offsetDays: 0,
      localTime: '10:00',
    },
  })
})

// A tag step of the id, and a Branch of the id with one listed path and otherwise.
function tagOf(id: number): AutomationStep {
  return { id: stepId(id), type: 'apply_tag', tag: 'x' }
}
function forkOf(id: number, listed: AutomationStep[], otherwise: AutomationStep[]): AutomationStep {
  return {
    id: stepId(id),
    type: 'branch',
    paths: [{ id: stepId(id + 100), condition: VIP, steps: listed }],
    otherwise: { id: stepId(id + 200), steps: otherwise },
  }
}

// The Steps a Branch may test the message of are those an Enrollment may have passed on its way
// there (ADR 0122): before the Branch in its path and in every path around it, the Steps of a fork it
// passed included; never its own paths', one after it, or one of another path of a fork it stands in.
test('the Steps before a Step are those an Enrollment may have passed when it reaches it', () => {
  const flow = flowOf(
    version([
      tagOf(40),
      forkOf(41, [tagOf(42)], [tagOf(43)]),
      forkOf(44, [tagOf(45)], [tagOf(46), forkOf(47, [tagOf(48)], [])]),
      tagOf(49),
    ]),
  )
  const ids = (target: number) => stepsBefore(flow.path, stepId(target)).map((step) => step.id)

  expect(ids(47)).toEqual([40, 41, 42, 43, 44, 46].map((id) => stepId(id)))
  expect(ids(48)).toEqual([40, 41, 42, 43, 44, 46, 47].map((id) => stepId(id)))
  expect(ids(49)).toEqual([40, 41, 42, 43, 44, 45, 46, 47, 48].map((id) => stepId(id)))
  expect(ids(40)).toEqual([])
  expect(stepsBefore(flow.path, stepId(99))).toEqual([])
})
