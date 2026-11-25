import { graphlib, layout } from '@dagrejs/dagre'
import type { Edge, Node } from '@xyflow/react'

import type { Flow, FlowPath, FlowStep, ForkStep } from './definition.ts'

// The canvas is drawn from the flow on every change, laid out top to bottom by dagre: the Trigger,
// each Step of the root path, and an insertion point on the edge below each of them, where a new
// Step goes. A fork's paths hang side by side under it, each headed by a node of its own, and
// rejoin at the insertion point after the fork. Nothing here is stored or moved by hand: the nodes
// are neither dragged nor connected.

export const TRIGGER_NODE_ID = 'trigger'

// Every node has a fixed size, so the layout needs no measuring.
const NODE_WIDTH = 280
const NODE_HEIGHT = 64
const PATH_WIDTH = 200
const PATH_HEIGHT = 36
const INSERT_SIZE = 28
// The room between two nodes along an edge, and between two paths side by side.
const RANK_GAP = 24
const PATH_GAP = 32

type TriggerNode = Node<Record<string, never>, 'trigger'>
export type StepNode = Node<{ step: FlowStep }, 'step'>
// What heads a fork's path: a Branch's listed path by its place in the order (1 is tested first),
// `otherwise`, the unknown path, or a Split's path by its weight.
export type PathRole =
  | { kind: 'condition'; index: number }
  | { kind: 'otherwise' }
  | { kind: 'unknown' }
  | { kind: 'weight'; index: number; weight: number | '' }
// The head of a fork's path: a click opens the fork; it is where the path's figures go.
export type PathNode = Node<{ fork: ForkStep; pathId: string; role: PathRole }, 'path'>
// An insertion point puts a new Step at `at` of the path of the id: 0 is its start. `end` says it
// is the path's last place, the one place an End may go.
export type InsertNode = Node<{ path: string; at: number; end: boolean }, 'insert'>
export type FlowNode = TriggerNode | StepNode | PathNode | InsertNode

const unplaced = { position: { x: 0, y: 0 } }

// The drawing of a part of the flow: its nodes and edges in order, and the nodes whose edges go
// on to what follows it (none after an End).
interface Drawn {
  nodes: FlowNode[]
  edges: Edge[]
  exits: string[]
}

function edge(source: string, target: string): Edge {
  return { id: `edge-${source}-${target}`, type: 'straight', source, target }
}

// The roles of a fork's paths, in forkPaths' order.
function pathRoles(step: ForkStep): { path: FlowPath; role: PathRole }[] {
  if (step.kind === 'split') {
    return step.paths.map((path, index) => ({
      path,
      role: { kind: 'weight', index, weight: path.weight },
    }))
  }
  return [
    ...step.paths.map((path, index): { path: FlowPath; role: PathRole } => ({
      path,
      role: { kind: 'condition', index },
    })),
    { path: step.otherwise, role: { kind: 'otherwise' } },
    ...(step.unknown ? [{ path: step.unknown, role: { kind: 'unknown' } as const }] : []),
  ]
}

function insertNode(path: string, at: number, end: boolean): InsertNode {
  return {
    ...unplaced,
    id: `insert-${path}-${at}`,
    type: 'insert',
    data: { path, at, end },
    width: INSERT_SIZE,
    height: INSERT_SIZE,
  }
}

// drawPath draws a path's Steps from the node `from`: an insertion point before each Step and
// after the last, but none after an End, which finishes the path.
function drawPath(path: FlowPath, from: string): Drawn {
  const nodes: FlowNode[] = []
  const edges: Edge[] = []
  let exits = [from]
  const link = (target: string) => {
    edges.push(...exits.map((source) => edge(source, target)))
  }
  const place = (at: number) => {
    const node = insertNode(path.id, at, at === path.steps.length)
    link(node.id)
    nodes.push(node)
    exits = [node.id]
  }
  for (const [at, step] of path.steps.entries()) {
    place(at)
    const drawn = drawStep(step)
    link(step.id)
    nodes.push(...drawn.nodes)
    edges.push(...drawn.edges)
    exits = drawn.exits
  }
  if (path.steps.at(-1)?.kind !== 'end') {
    place(path.steps.length)
  }
  return { nodes, edges, exits }
}

// drawStep draws a Step, and a fork's paths under it, each from its head.
function drawStep(step: FlowStep): Drawn {
  const node: StepNode = {
    ...unplaced,
    id: step.id,
    type: 'step',
    data: { step },
    width: NODE_WIDTH,
    height: NODE_HEIGHT,
  }
  if (step.kind === 'end') {
    return { nodes: [node], edges: [], exits: [] }
  }
  if (step.kind !== 'branch' && step.kind !== 'split') {
    return { nodes: [node], edges: [], exits: [step.id] }
  }
  const drawn: Drawn = { nodes: [node], edges: [], exits: [] }
  for (const { path, role } of pathRoles(step)) {
    const head: PathNode = {
      ...unplaced,
      id: `path-${path.id}`,
      type: 'path',
      data: { fork: step, pathId: path.id, role },
      width: PATH_WIDTH,
      height: PATH_HEIGHT,
    }
    const inner = drawPath(path, head.id)
    drawn.nodes.push(head, ...inner.nodes)
    drawn.edges.push(edge(step.id, head.id), ...inner.edges)
    drawn.exits.push(...inner.exits)
  }
  // Every path ends: nothing reaches the Step after the fork but through the fork itself.
  if (drawn.exits.length === 0) {
    drawn.exits.push(step.id)
  }
  return drawn
}

// layoutFlow draws the flow: its nodes placed top to bottom, and an edge between each two.
export function layoutFlow(flow: Flow): { nodes: FlowNode[]; edges: Edge[] } {
  const trigger: TriggerNode = {
    ...unplaced,
    id: TRIGGER_NODE_ID,
    type: 'trigger',
    data: {},
    width: NODE_WIDTH,
    height: NODE_HEIGHT,
  }
  const root = drawPath(flow.path, TRIGGER_NODE_ID)
  const chain: FlowNode[] = [trigger, ...root.nodes]
  const { edges } = root

  const graph = new graphlib.Graph()
  graph.setGraph({ rankdir: 'TB', ranksep: RANK_GAP, nodesep: PATH_GAP })
  graph.setDefaultEdgeLabel(() => ({}))
  for (const node of chain) {
    graph.setNode(node.id, { width: node.width ?? 0, height: node.height ?? 0 })
  }
  for (const { source, target } of edges) {
    graph.setEdge(source, target)
  }
  // A fork's paths stand side by side in their order, which means something (a Branch tests its
  // Conditions in it, a Split compares its paths with the first): dagre keeps the order the nodes
  // are added in, without reordering them to cross fewer edges, which a tree never has.
  layout(graph, { disableOptimalOrderHeuristic: true })

  // dagre places a node by its center, the canvas by its top-left corner.
  const nodes = chain.map((node): FlowNode => {
    const { x = 0, y = 0 } = graph.node(node.id)
    const position = { x: x - (node.width ?? 0) / 2, y: y - (node.height ?? 0) / 2 }
    return { ...node, position }
  })
  return { nodes, edges }
}
