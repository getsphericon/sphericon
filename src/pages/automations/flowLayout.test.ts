import { expect, test } from 'vitest'

import type {
  AutomationStep,
  AutomationVersion,
  BranchCondition,
} from '../../generated/site/types.gen.ts'
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

test('an insertion point sits on the edge below the Trigger and below each Step but an End', () => {
  const { nodes, edges } = layoutFlow(flowOf(version(STEPS)))

  const order = nodes.map((node) => (node.type === 'insert' ? `+${node.data.at}` : node.id))
  const links = edges.map((edge) => [edge.source, edge.target])
  const chain = nodes.slice(1).map((node, i) => [nodes[i]?.id, node.id])

  expect(order).toEqual([TRIGGER_NODE_ID, '+0', stepId(11), '+1', stepId(12), '+2', stepId(13)])
  expect(links).toEqual(chain)
})

test('a flow with no Step has one insertion point, under the Trigger', () => {
  const { nodes } = layoutFlow(flowOf(version([])))

  expect(nodes.map((node) => (node.type === 'insert' ? `+${node.data.at}` : node.id))).toEqual([
    TRIGGER_NODE_ID,
    '+0',
  ])
})

const VIP: BranchCondition = {
  kind: 'group',
  combinator: 'and',
  entries: [{ kind: 'tag', name: 'vip', operator: 'has' }],
}

// A Branch of two listed paths, `otherwise` and the unknown path, then a Step after it.
const FORKED: AutomationStep[] = [
  {
    id: stepId(20),
    type: 'branch',
    paths: [
      { id: stepId(21), condition: VIP, steps: [{ id: stepId(22), type: 'wait', seconds: 60 }] },
      {
        id: stepId(23),
        condition: VIP,
        steps: [
          {
            id: stepId(30),
            type: 'split',
            paths: [
              { id: stepId(31), weight: 40, steps: [] },
              { id: stepId(32), weight: 30, steps: [] },
              { id: stepId(33), weight: 30, steps: [] },
            ],
          },
          { id: stepId(24), type: 'end' },
        ],
      },
    ],
    otherwise: { id: stepId(25), steps: [] },
    unknown: { id: stepId(26), steps: [] },
  },
  { id: stepId(27), type: 'apply_tag', tag: 'done' },
]

test("a fork's paths hang under it side by side in their order, and rejoin at the place after it", () => {
  const { nodes, edges } = layoutFlow(flowOf(version(FORKED)))
  const byId = new Map(nodes.map((node) => [node.id, node]))
  const at = (id: string) => byId.get(id)?.position ?? { x: Number.NaN, y: Number.NaN }
  const heads = [stepId(21), stepId(23), stepId(25), stepId(26)].map((id) => at(`path-${id}`))
  const after = nodes.find(
    (node) => node.type === 'insert' && node.data.path === stepId(1) && node.data.at === 1,
  )
  const into = edges.filter((e) => e.target === after?.id).map((e) => e.source)

  const nested = [stepId(31), stepId(32), stepId(33)].map((id) => at(`path-${id}`).x)

  expect(heads.map((head) => head.x)).toEqual(heads.map((head) => head.x).toSorted((a, b) => a - b))
  expect(nested).toEqual(nested.toSorted((a, b) => a - b))
  expect(heads.every((head) => head.y > at(stepId(20)).y)).toBe(true)
  expect(at(after?.id ?? '').y).toBeGreaterThan(at(stepId(22)).y)
  // The path that ends rejoins nothing; the others rejoin from their last place, an empty path
  // from its only one.
  expect(into.toSorted()).toEqual(
    [`insert-${stepId(21)}-1`, `insert-${stepId(25)}-0`, `insert-${stepId(26)}-0`].toSorted(),
  )
})

test('an End is offered only at the last place of a path', () => {
  const { nodes } = layoutFlow(flowOf(version(FORKED)))

  const ends = nodes.flatMap((node) =>
    node.type === 'insert' && node.data.end ? [`${node.data.path}:${node.data.at}`] : [],
  )

  expect(ends.toSorted()).toEqual(
    [
      `${stepId(21)}:1`,
      `${stepId(25)}:0`,
      `${stepId(26)}:0`,
      `${stepId(31)}:0`,
      `${stepId(32)}:0`,
      `${stepId(33)}:0`,
      `${stepId(1)}:2`,
    ].toSorted(),
  )
})

// A Wait for event (ADR 0124) hangs its `arrived` path, then its `timedOut` path, under it, each
// headed by a node that names it and reads its figures by the Step's and the path's ids.
test("a Wait for event's arrived and timed out paths hang under it and rejoin after it", () => {
  const wait: AutomationStep = {
    id: stepId(40),
    type: 'wait_for_event',
    name: 'purchase',
    timeoutSeconds: 3600,
    awaits: { kind: 'event', matchers: [{ action: 'order.created' }] },
    arrived: { id: stepId(41), steps: [{ id: stepId(43), type: 'apply_tag', tag: 'buyer' }] },
    timedOut: { id: stepId(42), steps: [] },
  }
  const { nodes, edges } = layoutFlow(flowOf(version([wait])))

  const heads = nodes.flatMap((node) =>
    node.type === 'path' ? [[node.data.fork.id, node.data.pathId, node.data.role.kind]] : [],
  )
  const after = `insert-${stepId(1)}-1`
  const into = edges.filter((e) => e.target === after).map((e) => e.source)

  expect(heads).toEqual([
    [stepId(40), stepId(41), 'arrived'],
    [stepId(40), stepId(42), 'timedOut'],
  ])
  expect(into.toSorted()).toEqual([`insert-${stepId(41)}-1`, `insert-${stepId(42)}-0`].toSorted())
})
