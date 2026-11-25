import { based, defineEditorSession, draftGuards } from '../../editing/editorSession.ts'
import {
  siteAutomationsPublish,
  siteAutomationsRevert,
  siteAutomationsUpdate,
} from '../../generated/site/sdk.gen.ts'
import type {
  SiteAutomationResource,
  SiteAutomationsUpdateData,
} from '../../generated/site/types.gen.ts'
import { definitionOf, flowOf, holdsKept, pathInputOf } from './definition.ts'
import type { Flow } from './definition.ts'
import { automationSnapshot } from './leaveSnapshot.ts'

type Path = SiteAutomationsUpdateData['path']
// The editor's content: the name in the bar and the flow the canvas draws.
export interface AutomationContent {
  name: string
  flow: Flow
}
interface Submission {
  // The revision is the session's base, added when the Save is sent (ADR 0119).
  body: Omit<SiteAutomationsUpdateData['body'], 'revision'> & { name: string }
  fingerprint: string
  // The Trigger's filter is still being typed: there is no filter to save yet.
  filterIncomplete: boolean
}

// Capture the persisted representation before any asynchronous work: neither a later edit nor a
// query refresh may change this request or its receipt. A flow holding a Step the editor does not
// edit (a Step or a Trigger of a kind newer than the editor) is never saved, whoever asks: nothing
// may drop it.
export function automationSubmission(name: string, flow: Flow): Submission | null {
  const steps = pathInputOf(flow)
  if (steps === null || holdsKept(flow)) {
    return null
  }
  const { trigger, filterIncomplete, exitEvents, goalEvent } = definitionOf(flow)
  return {
    body: structuredClone({ name: name.trim(), trigger, exitEvents, goalEvent, steps }),
    fingerprint: automationSnapshot(name, flow),
    filterIncomplete,
  }
}

// The receipt of a Save: the stored Automation and the fingerprint of what was sent. Every Step
// was sent with the id it keeps, so the receipt adds nothing to the flow.
function acknowledge(submission: Submission, resource: SiteAutomationResource) {
  return { resource, fingerprint: submission.fingerprint }
}

interface AutomationSubject {
  path: Path
  resource: SiteAutomationResource
}

// The flow of the version the editor edits: the draft, else the live one (ADR 0065). A new
// Automation is created with a draft and Publish only moves it to live, so one is always there.
export function editedFlow(resource: SiteAutomationResource): Flow {
  const edited = resource.draft ?? resource.live
  if (!edited) {
    throw new Error('An Automation without a draft or a live version')
  }
  return flowOf(edited)
}

// The canvas adapter: the content is the name in the bar and the flow on the canvas.
export const automationAdapter = {
  id: 'automationEditor',
  open: ({ resource }: AutomationSubject): AutomationContent => ({
    name: resource.name,
    flow: editedFlow(resource),
  }),
  fingerprint: ({ name, flow }: AutomationContent) => automationSnapshot(name, flow),
  submit: ({ name, flow }: AutomationContent) => automationSubmission(name, flow),
  revision: (resource: SiteAutomationResource) => resource.revision,
  save: async ({
    subject,
    submission,
    revision,
  }: {
    subject: AutomationSubject
    submission: Submission
    revision: number | undefined
  }) => {
    const { data } = await siteAutomationsUpdate({
      path: subject.path,
      body: { ...submission.body, revision: based(revision) },
      throwOnError: true,
    })
    return data
  },
  acknowledge,
  // Newer edits stay as made, and the canvas is not reloaded.
  fold: (current: AutomationContent) => ({ content: current, replaces: false }),
  operations: {
    PUBLISH: {
      request: async ({
        subject,
        revision,
      }: {
        subject: AutomationSubject
        revision: number | undefined
      }) => {
        const { data } = await siteAutomationsPublish({
          path: subject.path,
          body: { revision: based(revision) },
          throwOnError: true,
        })
        return data
      },
      available: draftGuards.publishable,
    },
    REVERT: {
      request: async ({
        subject,
        revision,
      }: {
        subject: AutomationSubject
        revision: number | undefined
      }) => {
        const { data } = await siteAutomationsRevert({
          path: subject.path,
          body: { revision: based(revision) },
          throwOnError: true,
        })
        return data
      },
      available: draftGuards.revertible,
      // Revert is the one operation that replaces the canvas: with the live version.
      replaces: (resource: SiteAutomationResource): AutomationContent => {
        const live = resource.live
        if (!live) {
          throw new Error('Revert returned an Automation without a live version')
        }
        return { name: resource.name, flow: flowOf(live) }
      },
    },
  },
}

// Current content belongs to the bar and the canvas, the submission to the pending Save, and the
// acknowledged fingerprint to this session. Only explicit Revert replaces the canvas.
export const editorSession = defineEditorSession(automationAdapter)

export type AutomationOperation = keyof typeof automationAdapter.operations
