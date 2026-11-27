import { zWaitForEventName } from '../../generated/site/zod.gen.ts'
import { allSteps, forkPaths, stepsBefore } from './definition.ts'
import type { FlowPath, FlowStep } from './definition.ts'

// The names of the Wait for events whose arrived Event the Enrollment keeps (ADR 0136): later Steps
// read it as `awaited.<name>` in their content and test it in a Condition. Where no copy is kept, on
// a path that never passed the Wait for event, it renders nothing and a test of it fails, so the
// editor offers only a name that can come before a Step and warns of one not every path before it
// passes. The warning refuses nothing.

// keptName is the name a Wait for event on an Event keeps its arrived Event under; null for any
// other Step, for one on an open or click, which keeps none (ADR 0136), and for a name not of the
// form the contract takes, which no later Step can read.
function keptName(step: FlowStep): string | null {
  if (step.kind !== 'wait_for_event' || step.draft.form !== 'event') {
    return null
  }
  const { name } = step.draft
  return zWaitForEventName.safeParse(name).success ? name : null
}

// awaitedBefore are the names of the Wait for events an Enrollment may have passed when it reaches
// the Step of the id (stepsBefore), in the canvas's order: the ones its merge tags and its
// Conditions may read.
export function awaitedBefore(root: FlowPath, id: string): string[] {
  return stepsBefore(root, id).flatMap((step) => {
    const name = keptName(step)
    return name === null ? [] : [name]
  })
}

// A Liquid output or tag, `{{ … }}` or `{% … %}`, whose text a merge tag lives in.
const LIQUID = /\{\{([\s\S]*?)\}\}|\{%([\s\S]*?)%\}/g
// `awaited.<name>` within it, not as a part of a longer path.
const AWAITED = /(?<![\w.])awaited\.([a-z][a-z0-9_]*)/g

// namesInText are the names a text's Liquid reads as `awaited.<name>`.
function namesInText(text: string): string[] {
  return [...text.matchAll(LIQUID)].flatMap((liquid) =>
    [...(liquid[1] ?? liquid[2] ?? '').matchAll(AWAITED)].map((match) => match[1] ?? ''),
  )
}

// namesInCondition are the names a Condition's text (complete or still being typed) tests a
// property of the awaited Event of, at any depth.
function namesInCondition(text: string): string[] {
  if (text === '') {
    return []
  }
  const names: string[] = []
  // A Condition's text is JSON the editor wrote: each property test of an awaited Event holds its
  // source `{"kind":"awaited","name":…}`, at whatever depth.
  JSON.parse(text, (key, value: unknown) => {
    if (
      typeof value === 'object' &&
      value !== null &&
      Reflect.get(value, 'kind') === 'awaited' &&
      typeof Reflect.get(value, 'name') === 'string'
    ) {
      names.push(String(Reflect.get(value, 'name')))
    }
    return value
  })
  return names
}

// namesUsed are the names a Step reads: in the content a send or HTTP step renders, in a Branch
// path's Condition and in a Wait for event's own, each once, in the order it first appears.
function namesUsed(step: FlowStep): string[] {
  let names: string[]
  switch (step.kind) {
    case 'email':
    case 'telegram':
    case 'sms':
    case 'http':
      // Every text the draft holds is content its Step renders, the merge tags in it included.
      names = namesInText(JSON.stringify(step.draft))
      break
    case 'branch':
      names = step.paths.flatMap((path) => namesInCondition(path.conditionText))
      break
    case 'wait_for_event':
      // Only the Event form saves its Condition.
      names = step.draft.form === 'event' ? namesInCondition(step.draft.conditionText) : []
      break
    case 'wait':
    case 'wait_until':
    case 'apply_tag':
    case 'remove_tag':
    case 'split':
    case 'end':
    case 'kept':
      names = []
      break
  }
  return [...new Set(names)]
}

// Where the walk of a path stands: it found the Step, with the names every path to it passes, or it
// left the path with the names every path through it passes, none (null) when every way through it
// ends.
type Walk = { found: Set<string> } | { out: Set<string> | null }

// passed walks the path for the Step of the id, `sure` being the names every path into it passes. A
// fork's own name is passed in each of its paths; after the fork, a name is passed when every path
// that goes on past it passed it.
function passed(path: FlowPath, id: string, sure: Set<string>): Walk {
  let sofar = sure
  for (const step of path.steps) {
    if (step.id === id) {
      return { found: sofar }
    }
    if (step.kind === 'end') {
      return { out: null }
    }
    const name = keptName(step)
    const inside = name === null ? sofar : new Set([...sofar, name])
    const outs: Set<string>[] = []
    for (const nested of forkPaths(step)) {
      const walk = passed(nested, id, inside)
      if ('found' in walk) {
        return walk
      }
      if (walk.out !== null) {
        outs.push(walk.out)
      }
    }
    // A Step that is no fork, or a fork every path of which ends, leaves what was passed into it.
    const [first] = outs
    sofar =
      first === undefined
        ? inside
        : new Set([...first].filter((one) => outs.every((out) => out.has(one))))
  }
  return { out: sofar }
}

// awaitedWarnings are, by Step id, the names a Step reads that not every path before it passes: on
// a path that skips its Wait for event, the Step renders the name empty and a test of it fails.
// A Step with nothing to warn of is not listed.
export function awaitedWarnings(root: FlowPath): Map<string, string[]> {
  const warnings = new Map<string, string[]>()
  for (const step of allSteps(root)) {
    const used = namesUsed(step)
    if (used.length > 0) {
      const walk = passed(root, step.id, new Set())
      const sure = 'found' in walk ? walk.found : new Set<string>()
      const missing = used.filter((name) => !sure.has(name))
      if (missing.length > 0) {
        warnings.set(step.id, missing)
      }
    }
  }
  return warnings
}
