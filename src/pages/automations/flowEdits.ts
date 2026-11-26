import { allSteps, forkPaths, isFork, newDateDraft, newTimeOfDayDraft } from './definition.ts'
import type {
  BranchPathDraft,
  EditedKind,
  EditedStep,
  Flow,
  FlowPath,
  FlowStep,
  ForkStep,
  InsertedKind,
  SplitPathDraft,
  TriggerDraft,
} from './definition.ts'

// The edits the canvas and the side panel make to the flow. Each returns a new flow; the editor
// replaces its content with it, and the canvas is laid out again from it. A Step is found by its id
// and a place by its path's id, at any depth of the tree.

// A new wait pauses an hour.
const NEW_WAIT_SECONDS = 3600
// A new Split shares its Contacts evenly between its two paths.
const NEW_SPLIT_WEIGHT = 50

// The client mints the id of a new Step or path (ADR 0122); the server keeps it.
const newId = () => crypto.randomUUID()

function newPath(): FlowPath {
  return { id: newId(), steps: [] }
}

function newBranchPath(): BranchPathDraft {
  return { ...newPath(), conditionText: '' }
}

// A new Step of a kind edited in its own form, with what its kind starts with.
function newEditedStep(kind: EditedKind): EditedStep {
  const id = newId()
  switch (kind) {
    case 'email':
      return { id, kind, draft: { integrationId: '', subject: '', body: '', doc: null } }
    case 'telegram':
      return {
        id,
        kind,
        draft: {
          integrationId: '',
          text: '',
          unsubscribeButton: '',
          imageUrl: '',
          buttons: [],
          linkPreview: true,
        },
      }
    case 'sms':
      return { id, kind, draft: { integrationId: '', text: '', linkTracking: true } }
    case 'http':
      return { id, kind, draft: { stored: false, url: '', headers: [], body: '{}', secret: '' } }
    case 'wait':
      return { id, kind, draft: { seconds: NEW_WAIT_SECONDS } }
    // A new Wait until waits on a time of day, which it asks for.
    case 'wait_until':
      return {
        id,
        kind,
        draft: { form: 'time_of_day', timeOfDay: newTimeOfDayDraft(), date: newDateDraft() },
      }
    case 'apply_tag':
    case 'remove_tag':
    default:
      return { id, kind, draft: { tag: '' } }
  }
}

// A new Step of the kind. A Branch starts with one path to give a Condition and an empty
// `otherwise`, a Split with two even paths.
function newStep(kind: InsertedKind): FlowStep {
  if (kind === 'branch') {
    return { id: newId(), kind, paths: [newBranchPath()], otherwise: newPath(), unknown: null }
  }
  if (kind === 'split') {
    const paths = [newPath(), newPath()].map((path) => ({ ...path, weight: NEW_SPLIT_WEIGHT }))
    return { id: newId(), kind, paths }
  }
  return kind === 'end' ? { id: newId(), kind } : newEditedStep(kind)
}

// withPaths is the fork with each of its paths replaced by what `change` makes of it.
function withPaths(step: ForkStep, change: (path: FlowPath) => FlowPath): ForkStep {
  switch (step.kind) {
    case 'branch':
      return {
        ...step,
        paths: step.paths.map((path) => ({ ...path, ...change(path) })),
        otherwise: change(step.otherwise),
        unknown: step.unknown === null ? null : change(step.unknown),
      }
    case 'split':
    default:
      return { ...step, paths: step.paths.map((path) => ({ ...path, ...change(path) })) }
  }
}

// mapPath rebuilds the path and every path nested in it, each from its own Steps down: `change`
// sees a path after the paths nested in its Steps were rebuilt.
function mapPath(path: FlowPath, change: (path: FlowPath) => FlowPath): FlowPath {
  const steps = path.steps.map((step) =>
    isFork(step) ? withPaths(step, (nested) => mapPath(nested, change)) : step,
  )
  return change({ ...path, steps })
}

function mapFlow(flow: Flow, change: (path: FlowPath) => FlowPath): Flow {
  return { ...flow, path: mapPath(flow.path, change) }
}

// mapStep replaces the Step of the id, wherever it is, with what `change` makes of it.
function mapStep(flow: Flow, id: string, change: (step: FlowStep) => FlowStep): Flow {
  return mapFlow(flow, (path) =>
    path.steps.some((step) => step.id === id)
      ? { ...path, steps: path.steps.map((step) => (step.id === id ? change(step) : step)) }
      : path,
  )
}

// mapFork changes the fork of the id; a Step of another kind is left as it is.
function mapFork(flow: Flow, id: string, change: (step: ForkStep) => ForkStep): Flow {
  return mapStep(flow, id, (step) => (isFork(step) ? change(step) : step))
}

// A place a Step goes: the path's id, and the index in it (0 is its start).
export interface Place {
  path: string
  at: number
}

// insertStep puts a new Step of the kind at the place. It answers the flow and the new Step's id.
export function insertStep(
  flow: Flow,
  place: Place,
  kind: InsertedKind,
): { flow: Flow; id: string } {
  const step = newStep(kind)
  const next = mapFlow(flow, (path) =>
    path.id === place.path ? { ...path, steps: path.steps.toSpliced(place.at, 0, step) } : path,
  )
  return { flow: next, id: step.id }
}

// removeStep takes the Step of the id out of its path, a fork with its paths and what they hold.
export function removeStep(flow: Flow, id: string): Flow {
  return mapFlow(flow, (path) => ({ ...path, steps: path.steps.filter((step) => step.id !== id) }))
}

// nestedSteps counts the Steps a fork's paths hold, at any depth: what removing it removes with it.
export function nestedSteps(step: FlowStep): number {
  return forkPaths(step).reduce((count, path) => count + allSteps(path).length, 0)
}

// copyOf is a copy of the Step, it and every path and Step nested in it with ids of their own: two
// copies never share figures. A copied HTTP step is not stored: the server keeps a step's URL,
// secret and header values under its id, which the copy does not have.
function copyOf(step: FlowStep): FlowStep {
  const id = newId()
  if (step.kind === 'http') {
    return { ...step, id, draft: { ...structuredClone(step.draft), stored: false } }
  }
  if (isFork(step)) {
    return { ...withPaths(step, copyPath), id }
  }
  return { ...structuredClone(step), id }
}

function copyPath(path: FlowPath): FlowPath {
  return { id: newId(), steps: path.steps.map((step) => copyOf(step)) }
}

// copyStep puts a copy of the Step of the id right after it (copyOf). A kept Step is not copied,
// nor an End, which is its path's last Step.
export function copyStep(flow: Flow, of: string): { flow: Flow; id: string } {
  const original = allSteps(flow.path).find((step) => step.id === of)
  if (original === undefined || original.kind === 'kept' || original.kind === 'end') {
    return { flow, id: of }
  }
  const copy = copyOf(original)
  const next = mapFlow(flow, (path) => {
    const at = path.steps.findIndex((step) => step.id === of)
    return at === -1 ? path : { ...path, steps: path.steps.toSpliced(at + 1, 0, copy) }
  })
  return { flow: next, id: copy.id }
}

// updateStep replaces the Step of its id with the one given, as its panel edited it.
export function updateStep(flow: Flow, edited: EditedStep): Flow {
  return mapStep(flow, edited.id, () => edited)
}

// What a fork's panel edits of its paths, each by its id: a Branch path's Condition, a Split
// path's weight.
export type PathPatch =
  | Pick<BranchPathDraft, 'id' | 'conditionText'>
  | Pick<SplitPathDraft, 'id' | 'weight'>

// updateFork sets what the panel edited on the fork's paths, by their ids; the Steps they hold are
// the flow's own, which the canvas may have changed meanwhile.
export function updateFork(flow: Flow, id: string, patches: PathPatch[]): Flow {
  const byId = new Map(patches.map((patch) => [patch.id, patch]))
  const patched = <P extends FlowPath>(path: P): P => {
    const patch = byId.get(path.id)
    return patch === undefined ? path : { ...path, ...patch }
  }
  return mapFork(flow, id, (step) =>
    step.kind === 'branch'
      ? { ...step, paths: step.paths.map((path) => patched(path)) }
      : { ...step, paths: step.paths.map((path) => patched(path)) },
  )
}

// addPath adds a path to the fork, empty: a Branch's goes last of its listed paths, before
// `otherwise`, with a Condition to give; a Split's last, with a weight to give. It answers the
// flow and the new path's id.
export function addPath(flow: Flow, id: string): { flow: Flow; id: string } {
  const branchPath = newBranchPath()
  const splitPath: SplitPathDraft = { ...newPath(), weight: '' }
  const next = mapFork(flow, id, (step) =>
    step.kind === 'branch'
      ? { ...step, paths: [...step.paths, branchPath] }
      : { ...step, paths: [...step.paths, splitPath] },
  )
  const added = allSteps(next.path).find((step) => step.id === id)
  return { flow: next, id: added?.kind === 'split' ? splitPath.id : branchPath.id }
}

// removePath takes the path of the id out of the fork, with the Steps it holds.
export function removePath(flow: Flow, id: string, pathId: string): Flow {
  return mapFork(flow, id, (step) =>
    step.kind === 'branch'
      ? { ...step, paths: step.paths.filter((path) => path.id !== pathId) }
      : { ...step, paths: step.paths.filter((path) => path.id !== pathId) },
  )
}

// movePath moves a Branch's listed path up (-1) or down (1) the order its Conditions are tested
// in. `otherwise` and the unknown path stay where they are.
export function movePath(
  flow: Flow,
  id: string,
  { pathId, by }: { pathId: string; by: -1 | 1 },
): Flow {
  return mapFork(flow, id, (step) => {
    if (step.kind !== 'branch') {
      return step
    }
    const at = step.paths.findIndex((path) => path.id === pathId)
    const to = at + by
    const moved = step.paths[at]
    if (moved === undefined || to < 0 || to >= step.paths.length) {
      return step
    }
    return { ...step, paths: step.paths.toSpliced(at, 1).toSpliced(to, 0, moved) }
  })
}

// setUnknownPath gives the Branch an empty unknown path, or takes it away with what it holds.
export function setUnknownPath(flow: Flow, id: string, present: boolean): Flow {
  return mapFork(flow, id, (step) => {
    if (step.kind !== 'branch' || present === (step.unknown !== null)) {
      return step
    }
    return { ...step, unknown: present ? newPath() : null }
  })
}

// updateTrigger replaces the Trigger with the draft its panel edited.
export function updateTrigger(flow: Flow, trigger: TriggerDraft): Flow {
  return { ...flow, trigger }
}
