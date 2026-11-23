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
import { graphToSteps, stepsToGraph } from './definition.ts'
import type { AutomationGraph } from './definition.ts'
import { automationSnapshot } from './leaveSnapshot.ts'

type Path = SiteAutomationsUpdateData['path']
export interface AutomationContent {
  name: string
  graph: AutomationGraph
}
interface Submission {
  // The revision is the session's base, added when the Save is sent (ADR 0119).
  body: Omit<SiteAutomationsUpdateData['body'], 'revision'> & { name: string }
  fingerprint: string
  dropped: number
  // The Trigger's filter is still being typed: there is no filter to save yet.
  filterIncomplete: boolean
}

// Capture the persisted representation before any asynchronous work. SDK graph data can contain
// nested references; neither a later edit nor a query refresh may change this request or its receipt.
export function automationSubmission(name: string, graph: AutomationGraph): Submission | null {
  const captured = {
    nodes: graph.nodes.map((node) => ({
      ...node,
      data: { ...node.data, properties: structuredClone(node.data.properties) },
    })),
    edges: graph.edges.map((edge) => ({ ...edge })),
  }
  const { dropped, trigger, filterIncomplete, ...definition } = graphToSteps(captured)
  if (trigger === null) {
    return null
  }
  return {
    body: structuredClone({ name: name.trim(), trigger, ...definition }),
    fingerprint: automationSnapshot(name, captured),
    dropped,
    filterIncomplete,
  }
}

// The receipt of a Save: the stored Automation and the fingerprint of what was sent. Every Step
// was sent with the id it keeps (its node's), so the receipt adds nothing to the canvas.
function acknowledge(submission: Submission, resource: SiteAutomationResource) {
  return { resource, fingerprint: submission.fingerprint }
}

interface AutomationSubject {
  path: Path
  resource: SiteAutomationResource
  // The version the canvas opens with, as a graph, and the node labels Revert rebuilds one with.
  graph: AutomationGraph
  labels: Record<string, string>
}

// The canvas adapter: the content is the name in the bar and the graph on the canvas.
export const automationAdapter = {
  id: 'automationEditor',
  open: ({ resource, graph }: AutomationSubject): AutomationContent => ({
    name: resource.name,
    graph,
  }),
  fingerprint: ({ name, graph }: AutomationContent) => automationSnapshot(name, graph),
  submit: ({ name, graph }: AutomationContent) => automationSubmission(name, graph),
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
  // Newer fields, positions and edges stay as edited, and the canvas is not reloaded.
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
      replaces: (
        resource: SiteAutomationResource,
        subject: AutomationSubject,
      ): AutomationContent => {
        const live = resource.live
        if (!live) {
          throw new Error('Revert returned an Automation without a live version')
        }
        return { name: resource.name, graph: stepsToGraph(live, live.steps, subject.labels) }
      },
    },
  },
}

// Current content belongs to the bar and the canvas, the submission to the pending Save, and the
// acknowledged fingerprint to this session. Only explicit Revert replaces the canvas.
export const editorSession = defineEditorSession(automationAdapter)

export type AutomationOperation = keyof typeof automationAdapter.operations
