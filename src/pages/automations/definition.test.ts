import type { WorkflowBuilderEdge, WorkflowBuilderNode } from '@workflowbuilder/sdk'
import { expect, test } from 'vitest'

import type { AutomationPath, EventFilterInput } from '../../generated/site/types.gen.ts'
import { stepId, testId } from '../../test/payloads.ts'
import { cooldownHours, cooldownSeconds, graphToSteps, stepsToGraph } from './definition.ts'
import type { AutomationStep, AutomationTrigger } from './definition.ts'
import { automationSnapshot } from './leaveSnapshot.ts'

const ROOT = stepId(1)

const STEPS: AutomationStep[] = [
  { id: stepId(11), type: 'email', subject: 'Hello', body: '<p></p>' },
  { id: stepId(12), type: 'wait', seconds: 3600 },
  { id: stepId(13), type: 'apply_tag', tag: 'vip' },
  { id: stepId(14), type: 'remove_tag', tag: 'lead' },
]

// The root path of the steps.
function path(steps: AutomationStep[]): AutomationPath {
  return { id: ROOT, steps }
}

function node(id: string, type: string, properties: Record<string, unknown>): WorkflowBuilderNode {
  return {
    id,
    type: 'node',
    position: { x: 0, y: 0 },
    data: { type, icon: 'Tag', segments: [], properties },
  }
}

const TRIGGER = node('trigger', 'trigger', { triggerEvent: 'email.opened', pathId: ROOT })

// A Trigger on the event, with no exit and no goal event.
function on(action: string): AutomationTrigger {
  return {
    trigger: { kind: 'event', action, entryPolicy: 'once', cooldown: 3600 },
    exitEvents: [],
    goalEvent: null,
  }
}

// A canvas of the Trigger followed by the nodes in a chain.
function chain(...nodes: WorkflowBuilderNode[]) {
  const all = [TRIGGER, ...nodes]
  return { nodes: all, edges: all.slice(1).map((n, i) => edge(all[i]?.id ?? '', n.id)) }
}

function edge(source: string, target: string): WorkflowBuilderEdge {
  return { id: `${source}-${target}`, source, target }
}

test('stepsToGraph starts the chain with the Trigger node, linked to each step by its id', () => {
  const { nodes, edges } = stepsToGraph(on('email.opened'), path(STEPS), {})

  expect(nodes.map((n) => n.id)).toEqual([
    'trigger',
    stepId(11),
    stepId(12),
    stepId(13),
    stepId(14),
  ])
  expect(nodes.map((n) => n.position.y)).toEqual([0, 160, 320, 480, 640])
  expect(nodes[0]?.data).toMatchObject({
    type: 'trigger',
    isStartNode: true,
    properties: { triggerEvent: 'email.opened', pathId: ROOT },
  })
  expect(nodes[0]?.deletable).toBe(false)
  expect(nodes.slice(1).map((n) => n.data.properties)).toEqual([
    { integrationId: '', subject: 'Hello', body: '<p></p>', doc: '' },
    { seconds: 3600 },
    { tag: 'vip' },
    { tag: 'lead' },
  ])
  expect(edges.map((e) => [e.source, e.target])).toEqual([
    ['trigger', stepId(11)],
    [stepId(11), stepId(12)],
    [stepId(12), stepId(13)],
    [stepId(13), stepId(14)],
  ])
})

test('stepsToGraph fills in what a stored step leaves out', () => {
  const { nodes } = stepsToGraph(
    on('contact.created'),
    path([
      { id: stepId(11), type: 'telegram', text: 'Hi', buttons: [], linkPreview: true },
      { id: stepId(12), type: 'telegram', text: 'Hi', buttons: [], linkPreview: false },
      { id: stepId(13), type: 'sms', text: 'Hi', linkTracking: true },
      { id: stepId(14), type: 'sms', text: 'Hi', linkTracking: false },
    ]),
    {},
  )

  expect(nodes.slice(1).map((n) => n.data.properties)).toEqual([
    {
      integrationId: '',
      text: 'Hi',
      unsubscribeButton: '',
      imageUrl: '',
      buttons: '',
      linkPreview: '',
    },
    {
      integrationId: '',
      text: 'Hi',
      unsubscribeButton: '',
      imageUrl: '',
      buttons: '',
      linkPreview: 'off',
    },
    { integrationId: '', text: 'Hi', linkTracking: '' },
    { integrationId: '', text: 'Hi', linkTracking: 'off' },
  ])
})

test('stepsToGraph of no steps is the Trigger alone', () => {
  const { nodes, edges } = stepsToGraph(on('contact.created'), path([]), {})

  expect(nodes.map((n) => n.id)).toEqual(['trigger'])
  expect(edges).toEqual([])
})

test('graphToSteps round-trips a stored trigger and chain, each Step and the path keeping its id', () => {
  expect(graphToSteps(stepsToGraph(on('email.clicked'), path(STEPS), {}))).toEqual({
    trigger: { kind: 'event', action: 'email.clicked', entryPolicy: 'once', cooldown: 3600 },
    filterIncomplete: false,
    exitEvents: [],
    goalEvent: null,
    steps: path(STEPS),
    dropped: 0,
  })
})

test('an End the canvas reads is saved back with its id: a save never drops it', () => {
  const ended = path([...STEPS, { id: stepId(15), type: 'end' }])

  const { steps } = graphToSteps(stepsToGraph(on('email.opened'), ended, {}))

  expect(steps).toEqual(ended)
})

test('a node dropped on the canvas is saved with its node id, beside the stored steps', () => {
  const graph = stepsToGraph(on('contact.created'), path(STEPS.slice(0, 1)), {})
  const added = node(stepId(99), 'wait', { seconds: 60 })
  const extended = {
    nodes: [...graph.nodes, added],
    edges: [...graph.edges, edge(stepId(11), stepId(99))],
  }

  expect(graphToSteps(extended).steps).toEqual({
    id: ROOT,
    steps: [STEPS[0], { id: stepId(99), type: 'wait', seconds: 60 }],
  })
})

test('a Trigger with no path id saves the steps without one: the server gives it', () => {
  const graph = { nodes: [node('trigger', 'trigger', { triggerEvent: 'paid' })], edges: [] }

  expect(graphToSteps(graph).steps).toEqual({ steps: [] })
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

  // The canvas holds the document as text beside the body the server built...
  expect(stepsToGraph(on('contact.created'), path([stored]), {}).nodes[1]?.data.properties).toEqual(
    {
      integrationId: '',
      subject: 'Hi',
      body: '<p>built by the server</p>',
      doc: JSON.stringify(doc),
    },
  )
  // ...and resaving sends the document only: a body beside it is a 422.
  expect(graphToSteps(stepsToGraph(on('contact.created'), path([stored]), {})).steps.steps).toEqual(
    [{ id: stepId(11), type: 'email', subject: 'Hi', doc }],
  )
})

test('an email step whose document was cleared is saved as its body alone', () => {
  const graph = chain(node('a', 'email', { subject: 'Hi', body: '<p>source</p>', doc: '' }))

  expect(graphToSteps(graph).steps.steps).toEqual([
    { id: 'a', type: 'email', subject: 'Hi', body: '<p>source</p>' },
  ])
})

test('graphToSteps of a canvas without exactly one Trigger has no trigger event', () => {
  const none = {
    trigger: null,
    filterIncomplete: false,
    exitEvents: [],
    goalEvent: null,
    steps: { steps: [] },
    dropped: 0,
  }

  expect(graphToSteps({ nodes: [], edges: [] })).toEqual(none)
  expect(graphToSteps({ nodes: [node('a', 'wait', { seconds: 5 })], edges: [] })).toEqual(none)
  expect(graphToSteps({ nodes: [TRIGGER, { ...TRIGGER, id: 'second' }], edges: [] })).toEqual(none)
})

test('graphToSteps follows edges from the Trigger regardless of node order', () => {
  const graph = {
    nodes: [
      node('b', 'wait', { seconds: 5 }),
      node('a', 'email', { subject: 's', body: 'b' }),
      TRIGGER,
    ],
    edges: [edge('trigger', 'a'), edge('a', 'b')],
  }

  expect(graphToSteps(graph).steps.steps).toEqual([
    { id: 'a', type: 'email', subject: 's', body: 'b' },
    { id: 'b', type: 'wait', seconds: 5 },
  ])
})

test('graphToSteps coerces wait seconds and non-text values', () => {
  const graph = chain(
    node('a', 'wait', { seconds: '90.7' }),
    node('b', 'wait', { seconds: 'abc' }),
    node('c', 'apply_tag', { tag: 42 }),
  )

  expect(graphToSteps(graph).steps.steps).toEqual([
    { id: 'a', type: 'wait', seconds: 90 },
    { id: 'b', type: 'wait', seconds: 0 },
    { id: 'c', type: 'apply_tag', tag: '' },
  ])
})

test('graphToSteps reports nodes off the main chain as dropped', () => {
  const graph = {
    nodes: [
      TRIGGER,
      node('a', 'apply_tag', { tag: 'x' }),
      node('b', 'apply_tag', { tag: 'y' }),
      node('c', 'apply_tag', { tag: 'z' }),
    ],
    edges: [edge('trigger', 'a'), edge('a', 'b'), edge('a', 'c')],
  }

  expect(graphToSteps(graph)).toEqual({
    trigger: { kind: 'event', action: 'email.opened', entryPolicy: 'once' },
    filterIncomplete: false,
    exitEvents: [],
    goalEvent: null,
    steps: {
      id: ROOT,
      steps: [
        { id: 'a', type: 'apply_tag', tag: 'x' },
        { id: 'b', type: 'apply_tag', tag: 'y' },
      ],
    },
    dropped: 1,
  })
})

test('graphToSteps skips unknown node types and ignores edges to missing nodes', () => {
  const graph = {
    nodes: [TRIGGER, node('a', 'mystery', {}), node('b', 'remove_tag', { tag: 't' })],
    edges: [edge('trigger', 'a'), edge('a', 'b'), edge('b', 'ghost')],
  }

  expect(graphToSteps(graph).steps.steps).toEqual([{ id: 'b', type: 'remove_tag', tag: 't' }])
})

test('an HTTP step keeps its id and header names but never carries a secret into the canvas', () => {
  const stored: AutomationStep = {
    id: stepId(11),
    type: 'http',
    body: '{"a":1}',
    headers: [{ name: 'Authorization' }, { name: 'X-Key' }],
    hasSecret: true,
  }
  const { nodes } = stepsToGraph(on('contact.created'), path([stored]), {})

  expect(nodes[1]?.id).toBe(stepId(11))
  expect(nodes[1]?.data.properties).toEqual({
    stored: 'stored',
    url: '',
    headers: '[{"name":"Authorization"},{"name":"X-Key"}]',
    body: '{"a":1}',
    secret: '',
  })
  // Resaving untouched resends no URL, header value or secret, so the server keeps them, matched
  // by the Step's id.
  expect(graphToSteps(stepsToGraph(on('contact.created'), path([stored]), {}))).toEqual({
    trigger: { kind: 'event', action: 'contact.created', entryPolicy: 'once', cooldown: 3600 },
    filterIncomplete: false,
    exitEvents: [],
    goalEvent: null,
    steps: {
      id: ROOT,
      steps: [
        {
          id: stepId(11),
          type: 'http',
          body: '{"a":1}',
          headers: [{ name: 'Authorization' }, { name: 'X-Key' }],
        },
      ],
    },
    dropped: 0,
  })
})

test('an HTTP node with a new URL, header value and secret sends them', () => {
  const graph = chain(
    node('a', 'http', {
      stored: '',
      url: ' https://hooks.example.com/x ',
      headers: JSON.stringify([
        { name: 'Authorization', value: 'Bearer a:b' },
        { name: '', value: '' },
        { name: ' X-Key ', value: 'k' },
      ]),
      body: '{}',
      secret: 'whsec_x',
    }),
  )

  expect(graphToSteps(graph).steps.steps).toEqual([
    {
      id: 'a',
      type: 'http',
      body: '{}',
      url: 'https://hooks.example.com/x',
      headers: [
        { name: 'Authorization', value: 'Bearer a:b' },
        { name: 'X-Key', value: 'k' },
      ],
      secret: 'whsec_x',
    },
  ])
})

test('a bare HTTP step opens blank and a header with no value keeps its stored one', () => {
  const { nodes } = stepsToGraph(
    on('contact.created'),
    path([{ id: stepId(11), type: 'http', body: '', headers: [], hasSecret: false }]),
    {},
  )

  expect(nodes[1]?.data.properties).toEqual({
    stored: 'stored',
    url: '',
    headers: '[]',
    body: '',
    secret: '',
  })
  expect(
    graphToSteps(
      chain(
        node('a', 'http', {
          stored: '',
          url: '',
          headers: '[{"name":"X-Key","value":""}]',
          body: '',
          secret: '',
        }),
      ),
    ).steps.steps,
  ).toEqual([{ id: 'a', type: 'http', body: '', headers: [{ name: 'X-Key' }] }])
})

test('an HTTP node whose headers text is not a header list sends none', () => {
  expect(
    graphToSteps(
      chain(
        node('a', 'http', { stored: '', url: '', headers: 'X-Key: 1', body: '', secret: '' }),
        node('b', 'http', { stored: '', url: '', headers: '{"name":"X"}', body: '', secret: '' }),
      ),
    ).steps.steps,
  ).toEqual([
    { id: 'a', type: 'http', body: '', headers: [] },
    { id: 'b', type: 'http', body: '', headers: [] },
  ])
})

test('graphToSteps stops on a cycle instead of looping', () => {
  const graph = {
    nodes: [TRIGGER, node('a', 'apply_tag', { tag: 'x' }), node('b', 'apply_tag', { tag: 'y' })],
    edges: [edge('trigger', 'a'), edge('a', 'b'), edge('b', 'a')],
  }

  expect(graphToSteps(graph)).toEqual({
    trigger: { kind: 'event', action: 'email.opened', entryPolicy: 'once' },
    filterIncomplete: false,
    exitEvents: [],
    goalEvent: null,
    steps: {
      id: ROOT,
      steps: [
        { id: 'a', type: 'apply_tag', tag: 'x' },
        { id: 'b', type: 'apply_tag', tag: 'y' },
      ],
    },
    dropped: 0,
  })
})

test('stepsToGraph names each node by its kind', () => {
  const { nodes } = stepsToGraph(
    on('contact.created'),
    path([{ id: stepId(11), type: 'wait', seconds: 5 }]),
    { trigger: 'Trigger', wait: 'Wait' },
  )

  expect(nodes.map((n) => n.data.properties.label)).toEqual(['Trigger', 'Wait'])
})

test("the saved snapshot changes with the name, the trigger, the steps or a step's Integration, and not with a re-layout", () => {
  const graph = stepsToGraph(on('contact.created'), path(STEPS), {})
  const saved = automationSnapshot('Welcome', graph)
  const rest = STEPS.slice(1)
  const throughRelay: AutomationStep[] = [
    { id: stepId(11), type: 'email', subject: 'Hello', body: '<p></p>', integrationId: testId(21) },
    ...rest,
  ]

  expect(automationSnapshot('  Welcome ', graph)).toBe(saved)
  expect(automationSnapshot('Welcome 2', graph)).not.toBe(saved)
  expect(automationSnapshot('Welcome', stepsToGraph(on('email.opened'), path(STEPS), {}))).not.toBe(
    saved,
  )
  expect(
    automationSnapshot('Welcome', stepsToGraph(on('contact.created'), path(STEPS.slice(1)), {})),
  ).not.toBe(saved)
  expect(
    automationSnapshot('Welcome', stepsToGraph(on('contact.created'), path(throughRelay), {})),
  ).not.toBe(saved)
  const moved = {
    ...graph,
    nodes: graph.nodes.map((n) => ({ ...n, position: { x: 50, y: n.position.y } })),
  }
  expect(automationSnapshot('Welcome', moved)).toBe(saved)
})

test('each email step keeps the Integration it names, and one that names none sends through the default', () => {
  const steps: AutomationStep[] = [
    {
      id: stepId(11),
      type: 'email',
      integrationId: testId(21),
      subject: 'First',
      body: '<p>1</p>',
    },
    { id: stepId(12), type: 'wait', seconds: 60 },
    { id: stepId(13), type: 'email', subject: 'Second', body: '<p>2</p>' },
  ]
  const graph = stepsToGraph(on('contact.created'), path(steps), {})

  expect(graph.nodes.map((n) => n.data.properties.integrationId)).toEqual([
    undefined,
    testId(21),
    undefined,
    '',
  ])
  expect(graphToSteps(graph).steps).toEqual(path(steps))
})

test('the Trigger node carries the exit and goal events, which round-trip', () => {
  const trigger: AutomationTrigger = {
    trigger: { kind: 'event', action: 'payment_failed', entryPolicy: 'once', cooldown: 3600 },
    exitEvents: ['account_closed', 'plan_downgraded'],
    goalEvent: 'payment_succeeded',
  }
  const graph = stepsToGraph(trigger, path(STEPS), {})

  expect(graph.nodes[0]?.data.properties).toMatchObject({
    exitEvents: 'account_closed\nplan_downgraded',
    goalEvent: 'payment_succeeded',
  })
  expect(graphToSteps(graph)).toEqual({
    ...trigger,
    filterIncomplete: false,
    steps: path(STEPS),
    dropped: 0,
  })
})

test('blank exit lines and a blank goal are none', () => {
  const root = node('trigger', 'trigger', {
    triggerEvent: 'paid',
    exitEvents: ' refunded \n\n  ',
    goalEvent: ' ',
  })
  const graph = { nodes: [root], edges: [] }

  expect(graphToSteps(graph)).toMatchObject({ exitEvents: ['refunded'], goalEvent: null })
})

const PAID: EventFilterInput = {
  kind: 'group',
  combinator: 'and',
  entries: [
    { kind: 'property', key: 'status', test: { kind: 'text', operator: 'eq', value: 'paid' } },
  ],
}

test("the Trigger's filter rides on its node as JSON text and round-trips into the saved Trigger", () => {
  const trigger: AutomationTrigger = {
    trigger: {
      kind: 'event',
      action: 'order.completed',
      filter: PAID,
      entryPolicy: 'once',
      cooldown: 3600,
    },
    exitEvents: [],
    goalEvent: null,
  }
  const graph = stepsToGraph(trigger, path([]), {})

  expect(graph.nodes[0]?.data.properties).toMatchObject({
    triggerEvent: 'order.completed',
    triggerFilter: JSON.stringify(PAID),
  })
  expect(graphToSteps(graph)).toMatchObject({ trigger: trigger.trigger, filterIncomplete: false })
})

test('a Trigger with an empty filter, or none, saves no filter', () => {
  const empty = node('trigger', 'trigger', { triggerEvent: 'order.completed', triggerFilter: '' })

  expect(graphToSteps({ nodes: [empty], edges: [] })).toMatchObject({
    trigger: { kind: 'event', action: 'order.completed', entryPolicy: 'once' },
    filterIncomplete: false,
  })
  expect(graphToSteps({ nodes: [empty], edges: [] }).trigger).not.toHaveProperty('filter')
})

test('a filter still being typed is flagged and left out of the saved Trigger', () => {
  const blankKey = {
    ...PAID,
    entries: [{ kind: 'property', test: { kind: 'text', operator: 'eq', value: 'paid' } }],
  }
  const typing = node('trigger', 'trigger', {
    triggerEvent: 'order.completed',
    triggerFilter: JSON.stringify(blankKey),
  })

  const saved = graphToSteps({ nodes: [typing], edges: [] })

  expect(saved.filterIncomplete).toBe(true)
  expect(saved.trigger).toEqual({ kind: 'event', action: 'order.completed', entryPolicy: 'once' })
})

test("the Trigger's Entry policy and cooldown ride on its node and round-trip into the saved Trigger", () => {
  const trigger: AutomationTrigger = {
    trigger: { kind: 'event', action: 'order.completed', entryPolicy: 'again', cooldown: 7200 },
    exitEvents: [],
    goalEvent: null,
  }
  const graph = stepsToGraph(trigger, path([]), {})

  expect(graph.nodes[0]?.data.properties).toMatchObject({ entryPolicy: 'again', cooldown: '7200' })
  expect(graphToSteps(graph).trigger).toEqual(trigger.trigger)
})

test('a Trigger node without a cooldown leaves it to the server, and an unknown policy is once', () => {
  const bare = node('trigger', 'trigger', {
    triggerEvent: 'order.completed',
    entryPolicy: 'sometimes',
    cooldown: '',
  })

  expect(graphToSteps({ nodes: [bare], edges: [] }).trigger).toEqual({
    kind: 'event',
    action: 'order.completed',
    entryPolicy: 'once',
  })
})

test('the cooldown is typed in hours and held in seconds', () => {
  expect(cooldownSeconds(3)).toBe('10800')
  expect(cooldownHours('10800')).toBe(3)
  expect(cooldownSeconds('')).toBe('')
  expect(cooldownHours('')).toBe('')
})
