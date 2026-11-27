import type { AnyCondition, ConditionDraft } from '../../components/conditions/condition.ts'
import type {
  AutomationHttpHeader,
  AutomationPath,
  AutomationPathInput,
  AutomationStep,
  AutomationStepInput,
  BranchConditionInput,
  AutomationTriggerInput,
  AutomationVersion,
  DateFieldInput,
  DateOccurrence,
  DateOccurrenceInput,
  DateRecurrence,
  EditorDocument,
  EntryPolicy,
  EventFilterInput,
  EventMatcher,
  EventMatcherInput,
  TelegramButton,
  WaitForEventConditionInput,
  WaitForEventStep as WaitForEventStepRead,
  WaitUntilMoment,
  WaitUntilMomentInput,
  Weekday,
} from '../../generated/site/types.gen.ts'
import {
  zBranchConditionInput,
  zDateOccurrenceInput,
  zEntryPolicy,
  zEventFilterInput,
  zWaitForEventConditionInput,
  zWaitUntilTimeOfDayInput,
} from '../../generated/site/zod.gen.ts'
import { jsonText } from '../../utils/jsonText.ts'
import { completeButtons } from '../../utils/telegramContent.ts'

// The editor's model is the version's Step tree (ADR 0122): the Trigger with its exit and goal
// events, and the root path of Steps, each Step with the id it keeps across versions. The canvas
// draws it and the side panel edits it; nothing else holds the flow.

// Whether an offset counts days before the date (-1) or after it (1).
export type OffsetSign = -1 | 1

// A date a Contact holds, as the date form edits it (ADR 0126): the one model of the date Trigger
// and of a Wait until on a date. The field is null until one is chosen, the offset is whole days
// ('' while blank) before or after the date, by its sign, and the local time is `HH:MM` ('' while
// blank).
export interface DateDraft {
  field: DateFieldInput | null
  recurrence: DateRecurrence
  days: number | ''
  sign: OffsetSign
  localTime: string
}

// A Wait until's time of day as its form edits it: the local time ('' while blank) and the
// weekdays it counts on, none being every day.
export interface TimeOfDayDraft {
  localTime: string
  weekdays: Weekday[]
}

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
  // The form the Wait until waits on, and the draft of each form: switching keeps the other's.
  wait_until: {
    form: WaitUntilMomentInput['form']
    timeOfDay: TimeOfDayDraft
    date: DateDraft
  }
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

// A fork whose listed paths a person adds, removes and gives a Condition or a weight.
export type ListedFork = BranchStep | SplitStep

// What a Wait for event's panel edits (ADR 0124): its name, fixed once the Step is saved (ADR 0136);
// its timeout in seconds ('' while blank); the Event matchers it awaits (ADR 0180), each as an exit's
// draft; and its Condition's text ('' for none, else its JSON, complete or still being typed).
export interface WaitForEventDraft {
  name: string
  timeoutSeconds: number | ''
  matchers: EndingDraft[]
  conditionText: string
}

// A Wait for event on an Event: what its panel edits, and its two paths, `arrived` and `timedOut`.
export interface WaitForEventStep {
  id: string
  kind: 'wait_for_event'
  draft: WaitForEventDraft
  arrived: FlowPath
  timedOut: FlowPath
}

// A Step whose paths rejoin after it.
export type ForkStep = ListedFork | WaitForEventStep

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

// An Event matcher as a panel holds it (ADR 0133): an exit or goal event of the Trigger node, or one a
// Wait for event awaits. Its action, and its filter's text as the builder leaves it (filterText: ''
// for none, complete or still being typed).
export type EndingDraft = Pick<EventMatcherInput, 'action'> & { filterText: string }

// The Trigger as its panel edits it, by kind: an Event matcher that enrolls a Contact (ADR 0133),
// with its Entry policy and cooldown in seconds (ADR 0127, null for the server's default) and its
// Enrollment key ('' for none, ADR 0165); or a date the Contact holds (ADR 0126), which has none of
// them. Switching the kind keeps the other kind's draft. The events that end an Enrollment early or
// at its goal (#751) are either kind's.
export interface TriggerDraft {
  kind: AutomationTriggerInput['kind']
  action: string
  filterText: string
  entryPolicy: EntryPolicy
  cooldown: number | null
  enrollmentKey: string
  date: DateDraft
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
    case 'wait_until':
      return { id, kind: 'wait_until', draft: waitUntilDraftOf(step.until) }
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
    case 'wait_for_event':
      return waitForEventOf(step)
    default:
      return { id, kind: 'kept', step }
  }
}

// A stored Wait for event on an Event as the editor holds it: what its panel edits, and its two
// paths. The editor does not edit one on an open or click of a message yet: it is kept.
function waitForEventOf(step: WaitForEventStepRead): WaitForEventStep | KeptStep {
  if (step.awaits.kind !== 'event') {
    return { id: step.id, kind: 'kept', step }
  }
  const { condition, matchers } = step.awaits
  return {
    id: step.id,
    kind: 'wait_for_event',
    draft: {
      name: step.name,
      timeoutSeconds: step.timeoutSeconds,
      matchers: matchers.map((matcher) => endingDraft(matcher)),
      conditionText: condition ? filterText(condition) : '',
    },
    arrived: flowPathOf(step.arrived),
    timedOut: flowPathOf(step.timedOut),
  }
}

// A date a new Trigger or Wait until starts with: no field and no local time chosen yet, every
// year, on the date itself.
export function newDateDraft(): DateDraft {
  return { field: null, recurrence: 'yearly', days: 0, sign: 1, localTime: '' }
}

// A new Wait until's time of day: no local time chosen yet, every day.
export function newTimeOfDayDraft(): TimeOfDayDraft {
  return { localTime: '', weekdays: [] }
}

// The draft of a stored date: its offset as days before or after the date.
function dateDraftOf({ field, recurrence, offsetDays, localTime }: DateOccurrence): DateDraft {
  return {
    field,
    recurrence,
    days: Math.abs(offsetDays),
    sign: offsetDays < 0 ? -1 : 1,
    localTime,
  }
}

// The draft of a stored Wait until's moment: its own form's, and the other form's new.
function waitUntilDraftOf(until: WaitUntilMoment): StepDrafts['wait_until'] {
  if (until.form === 'date') {
    return { form: 'date', timeOfDay: newTimeOfDayDraft(), date: dateDraftOf(until) }
  }
  const { localTime, weekdays } = until
  return { form: 'time_of_day', timeOfDay: { localTime, weekdays }, date: newDateDraft() }
}

// The date a draft saves, or null while it is not one the contract takes (no field, no local
// time, a blank offset): such a draft is never saved.
function dateOccurrenceOf(draft: DateDraft): DateOccurrenceInput | null {
  const { field, recurrence, days, sign, localTime } = draft
  // A zero offset is the date itself, whatever its sign.
  const offsetDays = days === 0 ? 0 : sign * Number(days)
  const parsed = zDateOccurrenceInput.safeParse({
    field,
    recurrence,
    offsetDays: days === '' ? undefined : offsetDays,
    localTime,
  })
  return parsed.success ? parsed.data : null
}

// The moment a Wait until's draft saves, by its form; null as for dateOccurrenceOf.
function waitUntilMomentOf(draft: StepDrafts['wait_until']): WaitUntilMomentInput | null {
  if (draft.form === 'date') {
    const date = dateOccurrenceOf(draft.date)
    return date === null ? null : { form: 'date', ...date }
  }
  const parsed = zWaitUntilTimeOfDayInput.safeParse({ form: 'time_of_day', ...draft.timeOfDay })
  return parsed.success ? parsed.data : null
}

// The Trigger of a version as its panel edits it: the stored kind's draft, and the other kind's
// new.
function triggerDraftOf(version: AutomationVersion): TriggerDraft {
  const { trigger } = version
  const endings = {
    exits: version.exitEvents.map((exit) => endingDraft(exit)),
    goal: version.goalEvent === null ? null : endingDraft(version.goalEvent),
  }
  if (trigger.kind === 'date') {
    return {
      kind: 'date',
      action: '',
      filterText: '',
      entryPolicy: 'once',
      cooldown: null,
      enrollmentKey: '',
      date: dateDraftOf(trigger),
      ...endings,
    }
  }
  return {
    kind: 'event',
    action: trigger.action,
    filterText: trigger.filter ? filterText(trigger.filter) : '',
    entryPolicy: trigger.entryPolicy,
    cooldown: trigger.cooldown,
    enrollmentKey: trigger.enrollmentKey ?? '',
    date: newDateDraft(),
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
  return step.kind === 'branch' || step.kind === 'split' || step.kind === 'wait_for_event'
}

// forkPaths are the paths of a fork, in the order the canvas draws them: a Branch's listed paths,
// `otherwise`, then the unknown path; a Split's paths; a Wait for event's `arrived`, then
// `timedOut`. Any other Step has none.
export function forkPaths(step: FlowStep): FlowPath[] {
  if (step.kind === 'branch') {
    return [...step.paths, step.otherwise, ...(step.unknown ? [step.unknown] : [])]
  }
  if (step.kind === 'wait_for_event') {
    return [step.arrived, step.timedOut]
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

// stepsBefore are the Steps an Enrollment may have passed when it reaches the Step of the id (ADR
// 0122), in the order allSteps gives them: those before it in its own path and in every path around
// it, each with the Steps of its forks' paths, and the forks it stands in; never the Step itself, one
// after it, one of its own paths, or one of another path of a fork it stands in. None when the path
// holds no Step of the id. A Branch may test the message of a send Step among them.
export function stepsBefore(path: FlowPath, id: string): FlowStep[] {
  return before(path, id, []) ?? []
}

// before is stepsBefore within the path, passed being what the Enrollment may have passed when it
// entered it; undefined when the path holds no Step of the id.
function before(path: FlowPath, id: string, passed: FlowStep[]): FlowStep[] | undefined {
  let sofar = passed
  for (const step of path.steps) {
    if (step.id === id) {
      return sofar
    }
    const into = [...sofar, step]
    for (const nested of forkPaths(step)) {
      const found = before(nested, id, into)
      if (found !== undefined) {
        return found
      }
    }
    sofar = [...into, ...forkPaths(step).flatMap((nested) => allSteps(nested))]
  }
  return undefined
}

// storedNames are the names of the Wait for event Steps the versions hold, at any depth, by Step id:
// a name stored under its Step's id is fixed (ADR 0136).
export function storedNames(versions: (AutomationVersion | null)[]): Map<string, string> {
  return new Map(
    versions.flatMap((version) =>
      version === null
        ? []
        : allSteps(flowPathOf(version.steps)).flatMap((step) =>
            step.kind === 'wait_for_event' ? [[step.id, step.draft.name] as const] : [],
          ),
    ),
  )
}

// sendsMessage tells whether the Step sends a message on its Channel: the report counts what such a
// Step sent and skipped, by its id.
export function sendsMessage(step: FlowStep): boolean {
  return step.kind === 'email' || step.kind === 'telegram' || step.kind === 'sms'
}

// holdsKept tells whether the flow holds, at any depth, a Step the editor does not edit: the
// version is then read-only here.
export function holdsKept(flow: Flow): boolean {
  return allSteps(flow.path).some((step) => step.kind === 'kept')
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

// triggerOf reads the Trigger off its draft. An Event Trigger is its matcher (matcherOf), its
// Entry policy, its cooldown, none being the server's default, and its Enrollment key trimmed, a
// blank one being none. A date Trigger is its date alone (dateOccurrenceOf), null while it is not
// one to save.
function triggerOf(draft: TriggerDraft): {
  trigger: AutomationTriggerInput | null
  incomplete: boolean
} {
  if (draft.kind === 'date') {
    const date = dateOccurrenceOf(draft.date)
    return { trigger: date === null ? null : { kind: 'date', ...date }, incomplete: false }
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

// The Trigger a flow saves; null while its date is not one to save (triggerOf).
export function triggerInputOf(flow: Flow): AutomationTriggerInput | null {
  return triggerOf(flow.trigger).trigger
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
// server's default. Null for a Wait until whose moment is not one to save (waitUntilMomentOf).
function stepInputOf(step: EditedStep): AutomationStepInput | null {
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
    case 'wait_until': {
      const until = waitUntilMomentOf(step.draft)
      return until === null ? null : { id, type: 'wait_until', until }
    }
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

// The path a flow's path saves, or null when it holds what is never saved: a kept Step, a Branch
// path whose Condition is missing or still being typed, a Wait until whose moment is not set, or a
// Wait for event not one to save (waitForEventInput).
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
  if (step.kind === 'wait_for_event') {
    return waitForEventInput(step)
  }
  return step.kind === 'split' ? splitInput(step) : stepInputOf(step)
}

// The Condition a Wait for event's text holds: undefined for none (no text, or a group that tests
// nothing), null while it is still being typed, which is not one to save.
export function waitForEventConditionOf(
  text: string,
): WaitForEventConditionInput | null | undefined {
  if (text === '') {
    return undefined
  }
  const parsed = jsonText.pipe(zWaitForEventConditionInput).safeParse(text)
  if (!parsed.success) {
    return null
  }
  return parsed.data.entries.length > 0 ? parsed.data : undefined
}

// The matchers a Wait for event's drafts save (matcherOf): a row whose action is blank is none.
export function awaitedMatchersOf(
  drafts: EndingDraft[],
): { matcher: EventMatcherInput; incomplete: boolean }[] {
  return drafts.flatMap((draft) => endingRead(draft))
}

// The Wait for event a draft saves; null while it is not one to save: its timeout blank, no
// matcher, a filter or its Condition still being typed, or a path not one to save.
function waitForEventInput(step: WaitForEventStep): AutomationStepInput | null {
  const { name, timeoutSeconds, matchers, conditionText } = step.draft
  const read = awaitedMatchersOf(matchers)
  const condition = waitForEventConditionOf(conditionText)
  const arrived = pathInput(step.arrived)
  const timedOut = pathInput(step.timedOut)
  if (
    timeoutSeconds === '' ||
    read.length === 0 ||
    read.some((matcher) => matcher.incomplete) ||
    condition === null ||
    arrived === null ||
    timedOut === null
  ) {
    return null
  }
  return {
    id: step.id,
    type: 'wait_for_event',
    name,
    timeoutSeconds,
    awaits: {
      kind: 'event',
      matchers: read.map(({ matcher }) => matcher),
      ...(condition === undefined ? {} : { condition }),
    },
    arrived,
    timedOut,
  }
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

// A part of a flow as Save sends it (`sends`), or as the editor holds the part (`holds`) while it is
// not one to save, which tells one such flow from another.
type Sendable<Input, Draft> = { sends: Input } | { holds: Draft }

// A flow's definition: the Trigger, its exit and goal events and the root path, each Step and path
// with its id. `filterIncomplete` says a filter of the Trigger, an exit or the goal is still being
// typed, and is left out. The Trigger is held while its date is not set (triggerInputOf), the root
// path while it holds a kept Step, a Branch path's Condition missing or being typed, a Wait until
// not set, or a Wait for event not one to save (pathInputOf).
export interface FlowDefinition {
  trigger: Sendable<AutomationTriggerInput, TriggerDraft>
  filterIncomplete: boolean
  exitEvents: EventMatcherInput[]
  goalEvent: EventMatcherInput | null
  steps: Sendable<AutomationPathInput, FlowPath>
}

// definitionOf is the flow's definition (FlowDefinition): what Save sends, and what the editor
// holds of a part it cannot send yet.
export function definitionOf(flow: Flow): FlowDefinition {
  const { trigger, incomplete } = triggerOf(flow.trigger)
  const exits = flow.trigger.exits.flatMap((draft) => endingRead(draft))
  const [goal] = endingRead(flow.trigger.goal)
  const steps = pathInputOf(flow)
  return {
    trigger: trigger === null ? { holds: flow.trigger } : { sends: trigger },
    filterIncomplete: incomplete || [...exits, ...(goal ? [goal] : [])].some((e) => e.incomplete),
    exitEvents: exits.map(({ matcher }) => matcher),
    goalEvent: goal?.matcher ?? null,
    steps: steps === null ? { holds: flow.path } : { sends: steps },
  }
}

// The root path a flow saves; null when it holds a kept Step, which the editor never saves, or a
// Branch path whose Condition is missing or still being typed, a Wait until not set, or a Wait for
// event not one to save (waitForEventInput), which the Save stops on first.
export function pathInputOf(flow: Flow): AutomationPathInput | null {
  return pathInput(flow.path)
}
