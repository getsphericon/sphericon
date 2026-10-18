import type { WorkflowBuilderEdge, WorkflowBuilderNode } from '@workflowbuilder/sdk'
import { expect, test } from 'vitest'

import { type AutomationStep, graphToSteps, stepsToGraph } from './definition.ts'

const STEPS: AutomationStep[] = [
  { type: 'email', subject: 'Hello', body: '<mjml/>' },
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

function edge(source: string, target: string): WorkflowBuilderEdge {
  return { id: `${source}-${target}`, source, target }
}

test('stepsToGraph builds a top-to-bottom chain linked by edges', () => {
  const { nodes, edges } = stepsToGraph(STEPS)

  expect(nodes.map((n) => n.id)).toEqual(['step-0', 'step-1', 'step-2', 'step-3'])
  expect(nodes.map((n) => n.position.y)).toEqual([0, 160, 320, 480])
  expect(nodes.map((n) => n.data.properties)).toEqual([
    { subject: 'Hello', body: '<mjml/>' },
    { seconds: 3600 },
    { tag: 'vip' },
    { tag: 'lead' },
  ])
  expect(edges.map((e) => [e.source, e.target])).toEqual([
    ['step-0', 'step-1'],
    ['step-1', 'step-2'],
    ['step-2', 'step-3'],
  ])
})

test('stepsToGraph defaults missing step fields', () => {
  const { nodes } = stepsToGraph([{ type: 'email' }, { type: 'wait' }, { type: 'apply_tag' }])

  expect(nodes.map((n) => n.data.properties)).toEqual([
    { subject: '', body: '' },
    { seconds: 0 },
    { tag: '' },
  ])
})

test('stepsToGraph of nothing is an empty graph', () => {
  expect(stepsToGraph([])).toEqual({ nodes: [], edges: [] })
})

test('graphToSteps round-trips a stored chain', () => {
  expect(graphToSteps(stepsToGraph(STEPS))).toEqual({ steps: STEPS, dropped: 0 })
})

test('graphToSteps of an empty graph has no steps', () => {
  expect(graphToSteps({ nodes: [], edges: [] })).toEqual({ steps: [], dropped: 0 })
})

test('graphToSteps follows edges from the entry node regardless of node order', () => {
  const graph = {
    nodes: [node('b', 'wait', { seconds: 5 }), node('a', 'email', { subject: 's', body: 'b' })],
    edges: [edge('a', 'b')],
  }

  expect(graphToSteps(graph).steps).toEqual([
    { type: 'email', subject: 's', body: 'b' },
    { type: 'wait', seconds: 5 },
  ])
})

test('graphToSteps coerces wait seconds and non-text values', () => {
  const graph = {
    nodes: [
      node('a', 'wait', { seconds: '90.7' }),
      node('b', 'wait', { seconds: 'abc' }),
      node('c', 'apply_tag', { tag: 42 }),
    ],
    edges: [edge('a', 'b'), edge('b', 'c')],
  }

  expect(graphToSteps(graph).steps).toEqual([
    { type: 'wait', seconds: 90 },
    { type: 'wait', seconds: 0 },
    { type: 'apply_tag', tag: '' },
  ])
})

test('graphToSteps reports nodes off the main chain as dropped', () => {
  const graph = {
    nodes: [
      node('a', 'apply_tag', { tag: 'x' }),
      node('b', 'apply_tag', { tag: 'y' }),
      node('c', 'apply_tag', { tag: 'z' }),
    ],
    edges: [edge('a', 'b'), edge('a', 'c')],
  }

  expect(graphToSteps(graph)).toEqual({
    steps: [
      { type: 'apply_tag', tag: 'x' },
      { type: 'apply_tag', tag: 'y' },
    ],
    dropped: 1,
  })
})

test('graphToSteps skips unknown node types and ignores edges to missing nodes', () => {
  const graph = {
    nodes: [node('a', 'mystery', {}), node('b', 'remove_tag', { tag: 't' })],
    edges: [edge('a', 'b'), edge('b', 'ghost')],
  }

  expect(graphToSteps(graph).steps).toEqual([{ type: 'remove_tag', tag: 't' }])
})

test('graphToSteps stops on a cycle instead of looping', () => {
  const graph = {
    nodes: [node('a', 'apply_tag', { tag: 'x' }), node('b', 'apply_tag', { tag: 'y' })],
    edges: [edge('a', 'b'), edge('b', 'a')],
  }

  expect(graphToSteps(graph)).toEqual({
    steps: [
      { type: 'apply_tag', tag: 'x' },
      { type: 'apply_tag', tag: 'y' },
    ],
    dropped: 0,
  })
})
