import { isPlainObject } from 'es-toolkit'

import { getFieldErrors } from '../../utils/apiErrors.ts'
import { TRIGGER_NODE_ID } from './flowLayout.ts'

// A refused Save places each error the server lists by field (the problem's `errors`) on the node it
// is about, by the field's location in the body the Save sent: `trigger…` is the Trigger's, and a
// location in `steps` the deepest Step on its way (`steps.steps.1`, `steps.steps.0.paths.1.steps.2`,
// `steps.steps.1.until.localTime`), found by the id the Step was sent with.

// The id of the Step a location in the sent body lies in, if any.
function stepAt(sent: unknown, location: string): string | null {
  let node: unknown = sent
  let step: string | null = null
  for (const key of location.split('.')) {
    node = isPlainObject(node) || Array.isArray(node) ? Reflect.get(node, key) : undefined
    if (isPlainObject(node) && typeof node.id === 'string' && 'type' in node) {
      step = node.id
    }
  }
  return step
}

// A refused Save as one node shows it: the refusal, and the server's first message about the node.
export interface NodeRefusal {
  error: unknown
  message: string
}

// refusedNodes is each node a refused Save's errors are about, in the order the server listed them;
// null when the refusal lists none, or one no node holds (the name's), which the page then shows as
// it shows any refusal.
export function refusedNodes(error: unknown, sent: unknown): Map<string, NodeRefusal> | null {
  const errors = Object.entries(getFieldErrors(error) ?? {})
  const placed = new Map<string, NodeRefusal>()
  for (const [location, [message] = []] of errors) {
    const node =
      location === 'trigger' || location.startsWith('trigger.')
        ? TRIGGER_NODE_ID
        : stepAt(sent, location)
    if (node === null || message === undefined) {
      return null
    }
    placed.set(node, { error, message })
  }
  return placed.size === 0 ? null : placed
}
