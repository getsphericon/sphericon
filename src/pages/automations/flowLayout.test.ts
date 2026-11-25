import { expect, test } from 'vitest'

import type { AutomationStep, AutomationVersion } from '../../generated/site/types.gen.ts'
import { stepId } from '../../test/payloads.ts'
import { flowOf } from './definition.ts'
import { layoutFlow, TRIGGER_NODE_ID } from './flowLayout.ts'

function version(steps: AutomationStep[]): AutomationVersion {
  return {
    number: 1,
    trigger: { kind: 'event', action: 'paid', entryPolicy: 'once', cooldown: 3600 },
    exitEvents: [],
    goalEvent: null,
    steps: { id: stepId(1), steps },
  }
}

const STEPS: AutomationStep[] = [
  { id: stepId(11), type: 'wait', seconds: 60 },
  { id: stepId(12), type: 'apply_tag', tag: 'vip' },
  { id: stepId(13), type: 'end' },
]

test('a linear flow is laid out top to bottom: the Trigger, then each Step in order', () => {
  const { nodes } = layoutFlow(flowOf(version(STEPS)))

  const drawn = nodes.filter((node) => node.type !== 'insert')
  const ys = nodes.map((node) => node.position.y)
  const centers = new Set(nodes.map((node) => node.position.x + (node.width ?? 0) / 2))

  expect(drawn.map((node) => node.id)).toEqual([
    TRIGGER_NODE_ID,
    stepId(11),
    stepId(12),
    stepId(13),
  ])
  expect(ys.toSorted((a, b) => a - b)).toEqual(ys)
  expect(new Set(ys).size).toBe(ys.length)
  expect(centers.size).toBe(1)
})

test('an insertion point sits on the edge below the Trigger and below each Step, at its place', () => {
  const { nodes, edges } = layoutFlow(flowOf(version(STEPS)))

  const order = nodes.map((node) => (node.type === 'insert' ? `+${node.data.at}` : node.id))
  const links = edges.map((edge) => [edge.source, edge.target])
  const chain = nodes.slice(1).map((node, i) => [nodes[i]?.id, node.id])

  expect(order).toEqual([
    TRIGGER_NODE_ID,
    '+0',
    stepId(11),
    '+1',
    stepId(12),
    '+2',
    stepId(13),
    '+3',
  ])
  expect(links).toEqual(chain)
})

test('a flow with no Step has one insertion point, under the Trigger', () => {
  const { nodes } = layoutFlow(flowOf(version([])))

  expect(nodes.map((node) => (node.type === 'insert' ? `+${node.data.at}` : node.id))).toEqual([
    TRIGGER_NODE_ID,
    '+0',
  ])
})
