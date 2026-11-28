import { zStepName } from '../../generated/site/zod.gen.ts'
import { allSteps, forkPaths, stepsBefore } from './definition.ts'
import type { FlowPath, FlowStep } from './definition.ts'

// What an Enrollment keeps for its later Steps, under the names of the Steps that received it: the
// Event a Wait for event arrived on, read as `awaited.<name>` (ADR 0136), and the response an HTTP
// step got, read as `http.<name>` (ADR 0161), in content and in a Condition. Where nothing is kept,
// on a path that never passed the Step, it renders nothing and a test of it fails, so the editor
// offers only a name that can come before a Step and warns of one not every path before it passes.
// The warning refuses nothing.

// The prefixes later Steps read what a Step kept under: a Wait for event's and an HTTP step's.
type KeptKind = 'awaited' | 'http'

// keptRef is what a Step keeps for later Steps, as they read it (`awaited.<name>`, `http.<name>`);
// null for a Step that keeps nothing, a Wait for event on an open or click among them (ADR 0136),
// and for a name not of the form the contract takes, which no later Step can read.
function keptRef(step: FlowStep): string | null {
  let kept: { kind: KeptKind; name: string }
  if (step.kind === 'wait_for_event' && step.draft.form === 'event') {
    kept = { kind: 'awaited', name: step.draft.name }
  } else if (step.kind === 'http') {
    kept = { kind: 'http', name: step.draft.name }
  } else {
    return null
  }
  return zStepName.safeParse(kept.name).success ? `${kept.kind}.${kept.name}` : null
}

// keptBefore are the names under the prefix that an Enrollment may have passed a Step keeping when
// it reaches the Step of the id (stepsBefore), in the canvas's order.
function keptBefore(root: FlowPath, id: string, kind: KeptKind): string[] {
  return stepsBefore(root, id).flatMap((step) => {
    const ref = keptRef(step)
    return ref?.startsWith(`${kind}.`) ? [ref.slice(kind.length + 1)] : []
  })
}

// awaitedBefore are the names of the Wait for events an Enrollment may have passed when it reaches
// the Step of the id, in the canvas's order: the ones its merge tags and its Conditions may read.
export function awaitedBefore(root: FlowPath, id: string): string[] {
  return keptBefore(root, id, 'awaited')
}

// httpBefore are the names of the HTTP steps an Enrollment may have passed when it reaches the Step
// of the id, in the canvas's order: the ones its merge tags and a Branch's Conditions may read.
export function httpBefore(root: FlowPath, id: string): string[] {
  return keptBefore(root, id, 'http')
}

// A Liquid output or tag, `{{ … }}` or `{% … %}`, whose text a merge tag lives in.
const LIQUID = /\{\{([\s\S]*?)\}\}|\{%([\s\S]*?)%\}/g
// `awaited.<word>` or `http.<word>` within it, not as a part of a longer path: a word that is a name
// of the form the contract takes (zStepName) is a name read.
const KEPT = /(?<![\w.])(awaited|http)\.(\w+)/g

// refsInText are what a text's Liquid reads as `awaited.<name>` or `http.<name>`.
function refsInText(text: string): string[] {
  return [...text.matchAll(LIQUID)].flatMap((liquid) =>
    [...(liquid[1] ?? liquid[2] ?? '').matchAll(KEPT)]
      .filter((match) => zStepName.safeParse(match[2] ?? '').success)
      .map((match) => `${match[1] ?? ''}.${match[2] ?? ''}`),
  )
}

// refsInCondition are what a Condition's text (complete or still being typed) tests a property of,
// at any depth: an awaited Event or an HTTP step's response.
function refsInCondition(text: string): string[] {
  if (text === '') {
    return []
  }
  const refs: string[] = []
  // A Condition's text is JSON the editor wrote: each property test of a kept Event or response
  // holds its source `{"kind":"awaited"|"http","name":…}`, at whatever depth.
  JSON.parse(text, (key, value: unknown) => {
    if (typeof value === 'object' && value !== null) {
      const kind: unknown = Reflect.get(value, 'kind')
      const name: unknown = Reflect.get(value, 'name')
      if ((kind === 'awaited' || kind === 'http') && typeof name === 'string') {
        refs.push(`${kind}.${name}`)
      }
    }
    return value
  })
  return refs
}

// refsUsed are what a Step reads of what earlier Steps kept: in the content a send or HTTP step
// renders, in a Branch path's Condition and in a Wait for event's own, each once, in the order it
// first appears.
function refsUsed(step: FlowStep): string[] {
  let names: string[]
  switch (step.kind) {
    case 'email':
    case 'telegram':
    case 'sms':
    case 'http':
      // Every text the draft holds is content its Step renders, the merge tags in it included.
      names = refsInText(JSON.stringify(step.draft))
      break
    case 'branch':
      names = step.paths.flatMap((path) => refsInCondition(path.conditionText))
      break
    case 'wait_for_event':
      // Only the Event form saves its Condition.
      names = step.draft.form === 'event' ? refsInCondition(step.draft.conditionText) : []
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

// Where the walk of a path stands: it found the Step, with what every path to it keeps, or it left
// the path with what every path through it keeps, none (null) when every way through it ends.
type Walk = { found: Set<string> } | { out: Set<string> | null }

// passed walks the path for the Step of the id, `sure` being what every path into it keeps. What a
// fork keeps is kept in each of its paths; after the fork, it is kept when every path that goes on
// past it kept it.
function passed(path: FlowPath, id: string, sure: Set<string>): Walk {
  let sofar = sure
  for (const step of path.steps) {
    if (step.id === id) {
      return { found: sofar }
    }
    if (step.kind === 'end') {
      return { out: null }
    }
    const name = keptRef(step)
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

// keptWarnings are, by Step id, what a Step reads that not every path before it keeps
// (`awaited.<name>`, `http.<name>`): on a path that skips the Step keeping it, the Step renders it
// empty and a test of it fails. A Step with nothing to warn of is not listed.
export function keptWarnings(root: FlowPath): Map<string, string[]> {
  const warnings = new Map<string, string[]>()
  for (const step of allSteps(root)) {
    const used = refsUsed(step)
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
