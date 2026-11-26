import type { AnyCondition, ConditionDraft } from '../../components/conditions/condition.ts'
import type {
  AutomationHttpHeader,
  AutomationPath,
  AutomationPathInput,
  AutomationStep,
  AutomationStepInput,
  BranchConditionInput,
  AutomationTriggerInput,
  DateTrigger,
  AutomationVersion,
  EditorDocument,
  EntryPolicy,
  EventFilterInput,
  EventMatcher,
  EventMatcherInput,
  TelegramButton,
} from '../../generated/site/types.gen.ts'
import {
  zBranchConditionInput,
  zEntryPolicy,
  zEventFilterInput,
} from '../../generated/site/zod.gen.ts'
import { jsonText } from '../../utils/jsonText.ts'
import { completeButtons } from '../../utils/telegramContent.ts'

// The editor's model is the version's Step tree (ADR 0122): the Trigger with its exit and goal
// events, and the root path of Steps, each Step with the id it keeps across versions. The canvas
// draws it and the side panel edits it; nothing else holds the flow.

// What the side panel edits of each kind: every field a person types, as the form holds it.
export interface StepDrafts {
  // `integrationId` '' is the Channel's default (ADR 0081). `doc` is the editor's document, null
  // for a message written as HTML (its body).
  email: { integrationId: string; subject: string; body: string; doc: EditorDocument | null }
  telegram: {
    integrationId: string
    text: string
    unsubscribeButton: string
    imageUrl: string
    buttons: TelegramButton[]
    linkPreview: boolean
  }
  sms: { integrationId: string; text: string; linkTracking: boolean }
  // The server never returns an HTTP step's URL, header values or signing secret: a `stored` step
  // holds them blank, and a blank one keeps what is stored under the Step's id.
  http: {
    stored: boolean
    url: string
    headers: AutomationHttpHeader[]
    body: string
    secret: string
  }
  wait: { seconds: number | '' }
  apply_tag: { tag: string }
  remove_tag: { tag: string }
}

// The kinds the side panel edits in a form of their own: those it has a draft of.
export type EditedKind = keyof StepDrafts

// A Step the editor edits in its kind's form, by kind.
export type EditedStep = {
  [K in EditedKind]: { id: string; kind: K; draft: StepDrafts[K] }
}[EditedKind]

// A path of Steps, with the id it keeps across versions: the root, or a path of a fork.
export type FlowPath = Pick<AutomationPath, 'id'> & { steps: FlowStep[] }

// A Branch's listed path (ADR 0124): its Condition as the builder leaves it (conditionText: '' for
// none, else its JSON, complete or still being typed) and its Steps.
export interface BranchPathDraft extends FlowPath {
  conditionText: string
}

// A Split's path: its weight as typed ('' while blank) and its Steps.
export interface SplitPathDraft extends FlowPath {
  weight: number | ''
}

// A Branch: its listed paths in the order their Conditions are tested, `otherwise`, and the unknown
// path, there only while a Condition can be unknown (the server says when).
export interface BranchStep {
  id: string
  kind: 'branch'
  paths: BranchPathDraft[]
  otherwise: FlowPath
  unknown: FlowPath | null
}

// A Split: its weighted paths, in order (the first is compared with the others, ADR 0192).
export interface SplitStep {
  id: string
  kind: 'split'
  paths: SplitPathDraft[]
}

export type ForkStep = BranchStep | SplitStep

// An End: it finishes its path, and is its last Step.
type EndStep = Pick<AutomationPath, 'id'> & { kind: 'end' }

// A Step of a kind newer than the editor, kept as stored: the version holding it is read-only here
// (ADR 0124), so no edit or save can drop it.
interface KeptStep {
  id: string
  kind: 'kept'
  step: AutomationStep
}

export type FlowStep = EditedStep | ForkStep | EndStep | KeptStep

// The kinds a person puts on the canvas.
export type InsertedKind = EditedKind | ForkStep['kind'] | EndStep['kind']

// An exit or goal event as the Trigger node holds it (ADR 0133): its action, and its filter's text
// as the builder leaves it (filterText: '' for none, complete or still being typed).
export type EndingDraft = Pick<EventMatcherInput, 'action'> & { filterText: string }

// The Trigger as its panel edits it: the Event matcher that enrolls a Contact (ADR 0133), its Entry
// policy and cooldown in seconds (ADR 0127, null for the server's default), its Enrollment key (''
// for none, ADR 0165), and the events that end an Enrollment early or at its goal (#751). A date
// Trigger (ADR 0126), which the editor does not edit yet, is kept as stored: the version holding it
// is read-only here, so no edit or save can drop it.
export interface TriggerDraft {
  kept: DateTrigger | null
  action: string
  filterText: string
  entryPolicy: EntryPolicy
  cooldown: number | null
  enrollmentKey: string
  exits: EndingDraft[]
  goal: EndingDraft | null
}

export interface Flow {
  trigger: TriggerDraft
  path: FlowPath
}

// A stored path as the editor holds it.
function flowPathOf(path: AutomationPath): FlowPath {
  return { id: path.id, steps: path.steps.map((step) => flowStepOf(step)) }
}

// flowStepOf is a stored Step as the editor holds it: what the stored Step leaves out is blank, and
// a Step of a kind newer than the editor is kept.
function flowStepOf(step: AutomationStep): FlowStep {
  const { id } = step
  switch (step.type) {
    case 'email':
      return {
        id,
        kind: 'email',
        draft: {
          integrationId: step.integrationId ?? '',
          subject: step.subject,
          body: step.body,
          doc: step.doc ?? null,
        },
      }
    case 'telegram':
      return {
        id,
        kind: 'telegram',
        draft: {
          integrationId: step.integrationId ?? '',
          text: step.text,
          unsubscribeButton: step.unsubscribeButton ?? '',
          imageUrl: step.imageUrl ?? '',
          buttons: step.buttons,
          linkPreview: step.linkPreview,
        },
      }
    case 'sms':
      return {
        id,
        kind: 'sms',
        draft: {
          integrationId: step.integrationId ?? '',
          text: step.text,
          linkTracking: step.linkTracking,
        },
      }
    case 'http':
      return {
        id,
        kind: 'http',
        draft: {
          stored: true,
          url: '',
          headers: step.headers.map(({ name }) => ({ name })),
          body: step.body,
          secret: '',
        },
      }
    case 'wait':
      return { id, kind: 'wait', draft: { seconds: step.seconds } }
    case 'apply_tag':
      return { id, kind: 'apply_tag', draft: { tag: step.tag } }
    case 'remove_tag':
      return { id, kind: 'remove_tag', draft: { tag: step.tag } }
    case 'end':
      return { id, kind: 'end' }
    case 'branch':
      return {
        id,
        kind: 'branch',
        paths: step.paths.map((path) => ({
          ...flowPathOf(path),
          conditionText: filterText(path.condition),
        })),
        otherwise: flowPathOf(step.otherwise),
        unknown: step.unknown ? flowPathOf(step.unknown) : null,
      }
    case 'split':
      return {
        id,
        kind: 'split',
        paths: step.paths.map((path) => ({ ...flowPathOf(path), weight: path.weight })),
      }
    // Until the editor knows a Wait until (#1613's editor slice), a version holding one is kept.
    case 'wait_until':
    default:
      return { id, kind: 'kept', step }
  }
}

// The Trigger of a version as its panel edits it.
function triggerDraftOf(version: AutomationVersion): TriggerDraft {
  const { trigger } = version
  const endings = {
    exits: version.exitEvents.map((exit) => endingDraft(exit)),
    goal: version.goalEvent === null ? null : endingDraft(version.goalEvent),
  }
  if (trigger.kind !== 'event') {
    return {
      kept: trigger,
      action: '',
      filterText: '',
      entryPolicy: 'once',
      cooldown: null,
      enrollmentKey: '',
      ...endings,
    }
  }
  return {
    kept: null,
    action: trigger.action,
    filterText: trigger.filter ? filterText(trigger.filter) : '',
    entryPolicy: trigger.entryPolicy,
    cooldown: trigger.cooldown,
    enrollmentKey: trigger.enrollmentKey ?? '',
    ...endings,
  }
}

// flowOf is the version the editor opens: its Trigger and its root path, each Step by its id.
export function flowOf(version: AutomationVersion): Flow {
  return {
    trigger: triggerDraftOf(version),
    path: flowPathOf(version.steps),
  }
}

// Whether the Step is a fork: a Step whose paths rejoin after it.
export function isFork(step: FlowStep): step is ForkStep {
  return step.kind === 'branch' || step.kind === 'split'
}

// forkPaths are the paths of a fork, in the order the canvas draws them: a Branch's listed paths,
// `otherwise`, then the unknown path; a Split's paths. Any other Step has none.
export function forkPaths(step: FlowStep): FlowPath[] {
  if (step.kind === 'branch') {
    return [...step.paths, step.otherwise, ...(step.unknown ? [step.unknown] : [])]
  }
  return step.kind === 'split' ? step.paths : []
}

// allSteps are the Steps of the path and of every path nested in it, each fork before its paths'
// Steps.
export function allSteps(path: FlowPath): FlowStep[] {
  return path.steps.flatMap((step) => [
    step,
    ...forkPaths(step).flatMap((nested) => allSteps(nested)),
  ])
}

// sendsMessage tells whether the Step sends a message on its Channel: the report counts what such a
// Step sent and skipped, by its id.
export function sendsMessage(step: FlowStep): boolean {
  return step.kind === 'email' || step.kind === 'telegram' || step.kind === 'sms'
}

// holdsKept tells whether the flow holds a Trigger, or at any depth a Step, the editor does not
// edit: the version is then read-only here.
export function holdsKept(flow: Flow): boolean {
  return flow.trigger.kept !== null || allSteps(flow.path).some((step) => step.kind === 'kept')
}

// endingDraft is the draft of a stored exit or goal event.
function endingDraft(matcher: EventMatcher): EndingDraft {
  return { action: matcher.action, filterText: matcher.filter ? filterText(matcher.filter) : '' }
}

// filterText is the text of a filter as the builder edits it (complete or not): '' for an empty
// group, which tests nothing and is no filter, else its JSON.
export function filterText(filter: AnyCondition | ConditionDraft): string {
  return filter.entries.length === 0 && filter.not !== true ? '' : JSON.stringify(filter)
}

// The filter a text holds, as the builder loads it: a complete filter, or none for '' and for a
// filter still being typed, which the builder then starts over.
export function filterOf(text: string): EventFilterInput | undefined {
  if (text === '') {
    return undefined
  }
  const parsed = jsonText.pipe(zEventFilterInput).safeParse(text)
  return parsed.success ? parsed.data : undefined
}

// entryPolicyOf reads an Entry policy off a picker's value: `once` for anything that is none.
export function entryPolicyOf(value: unknown): EntryPolicy {
  const parsed = zEntryPolicy.safeParse(value)
  return parsed.success ? parsed.data : 'once'
}

const SECONDS_PER_HOUR = 3600

// cooldownHours shows a cooldown in seconds in hours, '' while none is set.
export function cooldownHours(seconds: number | null): number | '' {
  return seconds === null ? '' : seconds / SECONDS_PER_HOUR
}

// cooldownSeconds turns the hours a person typed into seconds, null for none.
export function cooldownSeconds(hours: number | string): number | null {
  return typeof hours === 'number' ? hours * SECONDS_PER_HOUR : null
}

// withoutHalfTypedFilter is the draft with a filter still being typed dropped: the builder loads
// only a complete filter, so the panel drops the rest too when it opens again.
export function withoutHalfTypedFilter(draft: EndingDraft): EndingDraft {
  return draft.filterText !== '' && filterOf(draft.filterText) === undefined
    ? { ...draft, filterText: '' }
    : draft
}

// matcherOf reads an Event matcher off its action and its filter's text: the action trimmed and,
// when the text holds a complete filter, that filter. incomplete says the text holds a filter still
// being typed (a blank key or number), which is no filter to save.
function matcherOf(
  action: string,
  text: string,
): { matcher: EventMatcherInput; incomplete: boolean } {
  const matcher: EventMatcherInput = { action: action.trim() }
  if (text === '') {
    return { matcher, incomplete: false }
  }
  const parsed = jsonText.pipe(zEventFilterInput).safeParse(text)
  if (!parsed.success) {
    return { matcher, incomplete: true }
  }
  return { matcher: { ...matcher, filter: parsed.data }, incomplete: false }
}

// endingRead is an exit or goal event read off its draft (matcherOf): none for no draft or a blank
// action.
function endingRead(
  draft: EndingDraft | null,
): { matcher: EventMatcherInput; incomplete: boolean }[] {
  return draft === null || draft.action.trim() === ''
    ? []
    : [matcherOf(draft.action, draft.filterText)]
}

// triggerOf reads the Trigger off its draft: its matcher (matcherOf), its Entry policy, its
// cooldown, none being the server's default, and its Enrollment key trimmed, a blank one being
// none.
function triggerOf(draft: TriggerDraft): { trigger: AutomationTriggerInput; incomplete: boolean } {
  if (draft.kept !== null) {
    const { field, recurrence, offsetDays, localTime } = draft.kept
    return {
      trigger: { kind: 'date', field, recurrence, offsetDays, localTime },
      incomplete: false,
    }
  }
  const { matcher, incomplete } = matcherOf(draft.action, draft.filterText)
  const trigger: AutomationTriggerInput = {
    kind: 'event',
    ...matcher,
    entryPolicy: entryPolicyOf(draft.entryPolicy),
  }
  if (draft.cooldown !== null) {
    trigger.cooldown = draft.cooldown
  }
  const key = draft.enrollmentKey.trim()
  if (key !== '') {
    trigger.enrollmentKey = key
  }
  return { trigger, incomplete }
}

// stepHeaders are the headers a step saves: a row left wholly blank is dropped, and a header
// whose value is empty keeps the stored value of the header with that name.
function stepHeaders(headers: AutomationHttpHeader[]): AutomationHttpHeader[] {
  return headers
    .map(({ name, value }) => ({ name: name.trim(), value: value ?? '' }))
    .filter(({ name, value }) => name !== '' || value !== '')
    .map(({ name, value }) => (value === '' ? { name } : { name, value }))
}

// An Integration a send step names: '' names none, and the step sends through the default.
function via(integrationId: string) {
  return integrationId === '' ? {} : { integrationId }
}

// The Step a draft saves, with its id: what is left blank is left out, a switch left on is the
// server's default.
function stepInputOf(step: EditedStep): AutomationStepInput {
  const { id } = step
  switch (step.kind) {
    case 'email': {
      // A message written in the editor is saved as its document alone (the server builds the
      // body; sending both is a 422), and one written as HTML as its body alone.
      const { integrationId, subject, body, doc } = step.draft
      return doc === null
        ? { id, type: 'email', ...via(integrationId), subject, body }
        : { id, type: 'email', ...via(integrationId), subject, doc }
    }
    case 'telegram': {
      const { draft } = step
      const imageUrl = draft.imageUrl.trim()
      const buttons = completeButtons(draft.buttons)
      return {
        id,
        type: 'telegram',
        text: draft.text,
        ...via(draft.integrationId),
        ...(draft.unsubscribeButton === '' ? {} : { unsubscribeButton: draft.unsubscribeButton }),
        ...(imageUrl === '' ? {} : { imageUrl }),
        ...(buttons.length === 0 ? {} : { buttons }),
        ...(draft.linkPreview ? {} : { linkPreview: false }),
      }
    }
    case 'sms':
      return {
        id,
        type: 'sms',
        text: step.draft.text,
        ...via(step.draft.integrationId),
        ...(step.draft.linkTracking ? {} : { linkTracking: false }),
      }
    case 'http': {
      const url = step.draft.url.trim()
      const { secret } = step.draft
      return {
        id,
        type: 'http',
        body: step.draft.body,
        headers: stepHeaders(step.draft.headers),
        ...(url === '' ? {} : { url }),
        ...(secret === '' ? {} : { secret }),
      }
    }
    case 'wait':
      return { id, type: 'wait', seconds: Math.trunc(Number(step.draft.seconds)) || 0 }
    case 'apply_tag':
    case 'remove_tag':
    default:
      return { id, type: step.kind, tag: step.draft.tag }
  }
}

// The Condition a Branch path's text holds when it is complete and tests something: a path without
// one refuses the save (ADR 0124).
export function branchConditionOf(text: string): BranchConditionInput | undefined {
  if (text === '') {
    return undefined
  }
  const parsed = jsonText.pipe(zBranchConditionInput).safeParse(text)
  return parsed.success && parsed.data.entries.length > 0 ? parsed.data : undefined
}

// The path a flow's path saves, or null when it holds what is never saved: a kept Step, or a Branch
// path whose Condition is missing or still being typed.
function pathInput(path: FlowPath): AutomationPathInput | null {
  const steps: AutomationStepInput[] = []
  for (const step of path.steps) {
    const input = stepInput(step)
    if (input === null) {
      return null
    }
    steps.push(input)
  }
  return { id: path.id, steps }
}

// The Step a flow's Step saves, its paths with it; null as for pathInput.
function stepInput(step: FlowStep): AutomationStepInput | null {
  if (step.kind === 'kept') {
    return null
  }
  if (step.kind === 'end') {
    return { id: step.id, type: 'end' }
  }
  if (step.kind === 'branch') {
    return branchInput(step)
  }
  return step.kind === 'split' ? splitInput(step) : stepInputOf(step)
}

function splitInput(step: SplitStep): AutomationStepInput | null {
  const paths = []
  for (const path of step.paths) {
    const input = pathInput(path)
    if (input === null) {
      return null
    }
    paths.push({ ...input, weight: Math.trunc(Number(path.weight)) || 0 })
  }
  return { id: step.id, type: 'split', paths }
}

function branchInput(step: BranchStep): AutomationStepInput | null {
  const paths = []
  for (const path of step.paths) {
    const condition = branchConditionOf(path.conditionText)
    const input = pathInput(path)
    if (condition === undefined || input === null) {
      return null
    }
    paths.push({ ...input, condition })
  }
  const otherwise = pathInput(step.otherwise)
  const unknown = step.unknown === null ? undefined : pathInput(step.unknown)
  if (otherwise === null || unknown === null) {
    return null
  }
  return {
    id: step.id,
    type: 'branch',
    paths,
    otherwise,
    ...(unknown === undefined ? {} : { unknown }),
  }
}

// What a flow saves as the version's definition: the Trigger, its exit and goal events and the root
// path, each Step and path with its id. `filterIncomplete` says a filter of the Trigger, an exit or
// the goal is still being typed, and is left out. A flow that is never saved as it stands (a kept
// Step, a Branch path's Condition missing or being typed: pathInputOf) shows its root path as the
// editor holds it, which tells one such flow from another.
export function definitionOf(flow: Flow): {
  trigger: AutomationTriggerInput
  filterIncomplete: boolean
  exitEvents: EventMatcherInput[]
  goalEvent: EventMatcherInput | null
  steps: AutomationPathInput | FlowPath
} {
  const { trigger, incomplete } = triggerOf(flow.trigger)
  const exits = flow.trigger.exits.flatMap((draft) => endingRead(draft))
  const [goal] = endingRead(flow.trigger.goal)
  return {
    trigger,
    filterIncomplete: incomplete || [...exits, ...(goal ? [goal] : [])].some((e) => e.incomplete),
    exitEvents: exits.map(({ matcher }) => matcher),
    goalEvent: goal?.matcher ?? null,
    steps: pathInputOf(flow) ?? flow.path,
  }
}

// The root path a flow saves; null when it holds a kept Step, which the editor never saves, or a
// Branch path whose Condition is missing or still being typed, which the Save stops on first.
export function pathInputOf(flow: Flow): AutomationPathInput | null {
  return pathInput(flow.path)
}
