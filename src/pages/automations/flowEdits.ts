import type { EditedKind, EditedStep, Flow, FlowStep, TriggerDraft } from './definition.ts'

// The edits the canvas and the side panel make to the flow. Each returns a new flow; the editor
// replaces its content with it, and the canvas is laid out again from it.

// A new wait pauses an hour.
const NEW_WAIT_SECONDS = 3600

// A new Step of the kind, with what its kind starts with. The client mints a new Step's id (ADR
// 0122); the server keeps it.
function newStep(kind: EditedKind): EditedStep {
  const id = crypto.randomUUID()
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
    case 'apply_tag':
    case 'remove_tag':
    default:
      return { id, kind, draft: { tag: '' } }
  }
}

function withSteps(flow: Flow, steps: FlowStep[]): Flow {
  return { ...flow, path: { ...flow.path, steps } }
}

// insertStep puts a new Step of the kind at the place `at` of the root path: 0 is right after the
// Trigger. It answers the flow and the new Step's id.
export function insertStep(flow: Flow, at: number, kind: EditedKind): { flow: Flow; id: string } {
  const step = newStep(kind)
  const steps = flow.path.steps.toSpliced(at, 0, step)
  return { flow: withSteps(flow, steps), id: step.id }
}

// removeStep takes the Step of the id out of its path.
export function removeStep(flow: Flow, id: string): Flow {
  return withSteps(
    flow,
    flow.path.steps.filter((step) => step.id !== id),
  )
}

// copyStep puts a copy of the Step of the id right after it, with an id of its own: two copies
// never share figures. A copied HTTP step is not stored: the server keeps a step's URL, secret and
// header values under its id, which the copy does not have.
export function copyStep(flow: Flow, of: string): { flow: Flow; id: string } {
  const at = flow.path.steps.findIndex((step) => step.id === of)
  const original = flow.path.steps[at]
  if (original === undefined || original.kind === 'kept') {
    return { flow, id: of }
  }
  const id = crypto.randomUUID()
  const copy: EditedStep =
    original.kind === 'http'
      ? { ...original, id, draft: { ...structuredClone(original.draft), stored: false } }
      : { ...structuredClone(original), id }
  return { flow: withSteps(flow, flow.path.steps.toSpliced(at + 1, 0, copy)), id: copy.id }
}

// updateStep replaces the Step of its id with the one given, as its panel edited it.
export function updateStep(flow: Flow, edited: EditedStep): Flow {
  return withSteps(
    flow,
    flow.path.steps.map((step) => (step.id === edited.id ? edited : step)),
  )
}

// updateTrigger replaces the Trigger with the draft its panel edited.
export function updateTrigger(flow: Flow, trigger: TriggerDraft): Flow {
  return { ...flow, trigger }
}
