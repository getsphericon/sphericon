import type { WorkflowBuilderEdge, WorkflowBuilderNode } from '@workflowbuilder/sdk'
import { expect, test } from 'vitest'

import { graphToSteps, stepsToGraph } from './definition.ts'
import type { AutomationStep } from './definition.ts'
import { automationSnapshot } from './leaveSnapshot.ts'

const STEPS: AutomationStep[] = [
  { type: 'email', subject: 'Hello', body: '<p></p>' },
  { type: 'wait', seconds: 3600 },
  { type: 'apply_tag', tag: 'vip' },
  { type: 'remove_tag', tag: 'lead' },
]

function node(id: string, type: string, properties: Record<string, unknown>): WorkflowBuilderNode {
  return {
    id,
    type: 'node',
    position: { x: 0, y: 0 },
    data: { type, icon: 'Tag', segments: [], properties },
  }
}

const TRIGGER = node('trigger', 'trigger', { triggerEvent: 'email.opened' })

// A Trigger on the event, with no exit and no goal event.
function on(triggerEvent: string) {
  return { triggerEvent, exitEvents: [], goalEvent: null }
}

// A canvas of the Trigger followed by the nodes in a chain.
function chain(...nodes: WorkflowBuilderNode[]) {
  const all = [TRIGGER, ...nodes]
  return { nodes: all, edges: all.slice(1).map((n, i) => edge(all[i]?.id ?? '', n.id)) }
}

function edge(source: string, target: string): WorkflowBuilderEdge {
  return { id: `${source}-${target}`, source, target }
}

test('stepsToGraph starts the chain with the Trigger node, linked to each step', () => {
  const { nodes, edges } = stepsToGraph(on('email.opened'), STEPS, {})

  expect(nodes.map((n) => n.id)).toEqual(['trigger', 'step-0', 'step-1', 'step-2', 'step-3'])
  expect(nodes.map((n) => n.position.y)).toEqual([0, 160, 320, 480, 640])
  expect(nodes[0]?.data).toMatchObject({
    type: 'trigger',
    isStartNode: true,
    properties: { triggerEvent: 'email.opened' },
  })
  expect(nodes[0]?.deletable).toBe(false)
  expect(nodes.slice(1).map((n) => n.data.properties)).toEqual([
    { subject: 'Hello', body: '<p></p>', doc: '' },
    { seconds: 3600 },
    { tag: 'vip' },
    { tag: 'lead' },
  ])
  expect(edges.map((e) => [e.source, e.target])).toEqual([
    ['trigger', 'step-0'],
    ['step-0', 'step-1'],
    ['step-1', 'step-2'],
    ['step-2', 'step-3'],
  ])
})

test('stepsToGraph defaults missing step fields', () => {
  const { nodes } = stepsToGraph(
    on('contact.created'),
    [{ type: 'email' }, { type: 'wait' }, { type: 'apply_tag' }],
    {},
  )

  expect(nodes.slice(1).map((n) => n.data.properties)).toEqual([
    { subject: '', body: '', doc: '' },
    { seconds: 0 },
    { tag: '' },
  ])
})

test('stepsToGraph of no steps is the Trigger alone', () => {
  const { nodes, edges } = stepsToGraph(on('contact.created'), [], {})

  expect(nodes.map((n) => n.id)).toEqual(['trigger'])
  expect(edges).toEqual([])
})

test('graphToSteps round-trips a stored trigger and chain', () => {
  expect(graphToSteps(stepsToGraph(on('email.clicked'), STEPS, {}))).toEqual({
    triggerEvent: 'email.clicked',
    exitEvents: [],
    goalEvent: null,
    steps: STEPS,
    dropped: 0,
  })
})

test('an email step written in the editor is saved as its document alone', () => {
  const doc = { type: 'doc', content: [{ type: 'paragraph' }] }
  const stored: AutomationStep = {
    type: 'email',
    subject: 'Hi',
    body: '<p>built by the server</p>',
    doc,
    editorVersion: '1.7.14',
  }

  // The canvas holds the document as text beside the body the server built...
  expect(stepsToGraph(on('contact.created'), [stored], {}).nodes[1]?.data.properties).toEqual({
    subject: 'Hi',
    body: '<p>built by the server</p>',
    doc: JSON.stringify(doc),
  })
  // ...and resaving sends the document only: a body beside it is a 422.
  expect(graphToSteps(stepsToGraph(on('contact.created'), [stored], {})).steps).toEqual([
    { type: 'email', subject: 'Hi', doc },
  ])
})

test('an email step whose document was cleared is saved as its body alone', () => {
  const graph = chain(node('a', 'email', { subject: 'Hi', body: '<p>source</p>', doc: '' }))

  expect(graphToSteps(graph).steps).toEqual([
    { type: 'email', subject: 'Hi', body: '<p>source</p>' },
  ])
})

test('graphToSteps of a canvas without exactly one Trigger has no trigger event', () => {
  const none = { triggerEvent: null, exitEvents: [], goalEvent: null, steps: [], dropped: 0 }

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

  expect(graphToSteps(graph).steps).toEqual([
    { type: 'email', subject: 's', body: 'b' },
    { type: 'wait', seconds: 5 },
  ])
})

test('graphToSteps coerces wait seconds and non-text values', () => {
  const graph = chain(
    node('a', 'wait', { seconds: '90.7' }),
    node('b', 'wait', { seconds: 'abc' }),
    node('c', 'apply_tag', { tag: 42 }),
  )

  expect(graphToSteps(graph).steps).toEqual([
    { type: 'wait', seconds: 90 },
    { type: 'wait', seconds: 0 },
    { type: 'apply_tag', tag: '' },
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
    triggerEvent: 'email.opened',
    exitEvents: [],
    goalEvent: null,
    steps: [
      { type: 'apply_tag', tag: 'x' },
      { type: 'apply_tag', tag: 'y' },
    ],
    dropped: 1,
  })
})

test('graphToSteps skips unknown node types and ignores edges to missing nodes', () => {
  const graph = {
    nodes: [TRIGGER, node('a', 'mystery', {}), node('b', 'remove_tag', { tag: 't' })],
    edges: [edge('trigger', 'a'), edge('a', 'b'), edge('b', 'ghost')],
  }

  expect(graphToSteps(graph).steps).toEqual([{ type: 'remove_tag', tag: 't' }])
})

test('an HTTP step keeps its id and header names but never carries a secret into the canvas', () => {
  const stored: AutomationStep = {
    type: 'http',
    id: '01J',
    body: '{"a":1}',
    headers: [{ name: 'Authorization' }, { name: 'X-Key' }],
    hasSecret: true,
  }
  const { nodes } = stepsToGraph(on('contact.created'), [stored], {})

  expect(nodes[1]?.data.properties).toEqual({
    stepId: '01J',
    url: '',
    headers: '[{"name":"Authorization"},{"name":"X-Key"}]',
    body: '{"a":1}',
    secret: '',
  })
  // Resaving untouched resends no URL, header value or secret, so the server keeps them.
  expect(graphToSteps(stepsToGraph(on('contact.created'), [stored], {}))).toEqual({
    triggerEvent: 'contact.created',
    exitEvents: [],
    goalEvent: null,
    steps: [
      {
        type: 'http',
        id: '01J',
        body: '{"a":1}',
        headers: [{ name: 'Authorization' }, { name: 'X-Key' }],
      },
    ],
    dropped: 0,
  })
})

test('an HTTP node with a new URL, header value and secret sends them', () => {
  const graph = chain(
    node('a', 'http', {
      stepId: '',
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

  expect(graphToSteps(graph).steps).toEqual([
    {
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
  const { nodes } = stepsToGraph(on('contact.created'), [{ type: 'http', hasSecret: false }], {})

  expect(nodes[1]?.data.properties).toEqual({
    stepId: '',
    url: '',
    headers: '[]',
    body: '',
    secret: '',
  })
  expect(
    graphToSteps(
      chain(
        node('a', 'http', {
          stepId: '',
          url: '',
          headers: '[{"name":"X-Key","value":""}]',
          body: '',
          secret: '',
        }),
      ),
    ).steps,
  ).toEqual([{ type: 'http', body: '', headers: [{ name: 'X-Key' }] }])
})

test('graphToSteps stops on a cycle instead of looping', () => {
  const graph = {
    nodes: [TRIGGER, node('a', 'apply_tag', { tag: 'x' }), node('b', 'apply_tag', { tag: 'y' })],
    edges: [edge('trigger', 'a'), edge('a', 'b'), edge('b', 'a')],
  }

  expect(graphToSteps(graph)).toEqual({
    triggerEvent: 'email.opened',
    exitEvents: [],
    goalEvent: null,
    steps: [
      { type: 'apply_tag', tag: 'x' },
      { type: 'apply_tag', tag: 'y' },
    ],
    dropped: 0,
  })
})

test('stepsToGraph names each node by its kind', () => {
  const { nodes } = stepsToGraph(on('contact.created'), [{ type: 'wait', seconds: 5 }], {
    trigger: 'Trigger',
    wait: 'Wait',
  })

  expect(nodes.map((n) => n.data.properties.label)).toEqual(['Trigger', 'Wait'])
})

test('the saved snapshot changes with the name, the Integration, the trigger or the steps, and not with a re-layout', () => {
  const graph = stepsToGraph(on('contact.created'), STEPS, {})
  const saved = automationSnapshot('Welcome', null, graph)

  expect(automationSnapshot('  Welcome ', null, graph)).toBe(saved)
  expect(automationSnapshot('Welcome 2', null, graph)).not.toBe(saved)
  expect(automationSnapshot('Welcome', '21', graph)).not.toBe(saved)
  expect(automationSnapshot('Welcome', null, stepsToGraph(on('email.opened'), STEPS, {}))).not.toBe(
    saved,
  )
  expect(
    automationSnapshot('Welcome', null, stepsToGraph(on('contact.created'), STEPS.slice(1), {})),
  ).not.toBe(saved)
  const moved = {
    ...graph,
    nodes: graph.nodes.map((n) => ({ ...n, position: { x: 50, y: n.position.y } })),
  }
  expect(automationSnapshot('Welcome', null, moved)).toBe(saved)
})

test('the Trigger node carries the exit and goal events, which round-trip', () => {
  const trigger = {
    triggerEvent: 'payment_failed',
    exitEvents: ['account_closed', 'plan_downgraded'],
    goalEvent: 'payment_succeeded',
  }
  const graph = stepsToGraph(trigger, STEPS, {})

  expect(graph.nodes[0]?.data.properties).toMatchObject({
    exitEvents: 'account_closed\nplan_downgraded',
    goalEvent: 'payment_succeeded',
  })
  expect(graphToSteps(graph)).toEqual({ ...trigger, steps: STEPS, dropped: 0 })
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
