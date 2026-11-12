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
import { graphToSteps, linearGraphNodes, stepsToGraph } from './definition.ts'
import type { AutomationGraph } from './definition.ts'
import { automationSnapshot } from './leaveSnapshot.ts'
import { HTTP_NODE_TYPE } from './nodes.ts'

type Path = SiteAutomationsUpdateData['path']
export interface AutomationContent {
  name: string
  graph: AutomationGraph
}
interface Submission {
  // The revision is the session's base, added when the Save is sent (ADR 0119).
  body: Omit<SiteAutomationsUpdateData['body'], 'revision'> & { name: string }
  graph: AutomationGraph
  fingerprint: string
  dropped: number
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
  const { dropped, triggerEvent, ...definition } = graphToSteps(captured)
  if (triggerEvent === null) {
    return null
  }
  return {
    body: structuredClone({ name: name.trim(), triggerEvent, ...definition }),
    graph: captured,
    fingerprint: automationSnapshot(name, captured),
    dropped,
  }
}

interface HttpIdentity {
  nodeId: string
  previous: unknown
  assigned: string
}

// A receipt can enrich still-present submitted nodes, never a replacement node or editable data.
export function applyHttpIdentities(nodes: AutomationGraph['nodes'], identities: HttpIdentity[]) {
  const byNode = new Map(identities.map((identity) => [identity.nodeId, identity]))
  return nodes.map((node) => {
    const identity = byNode.get(node.id)
    if (
      !identity ||
      node.data.type !== HTTP_NODE_TYPE ||
      node.data.properties.stepId !== identity.previous
    ) {
      return node
    }
    return {
      ...node,
      data: { ...node.data, properties: { ...node.data.properties, stepId: identity.assigned } },
    }
  })
}

function acknowledge(submission: Submission, resource: SiteAutomationResource) {
  const version = resource.draft ?? resource.live
  const httpSteps = version?.steps.filter((step) => step.type === 'http') ?? []
  const httpNodes = linearGraphNodes(submission.graph).filter(
    (node) => node.data.type === HTTP_NODE_TYPE,
  )
  const identities = httpNodes.flatMap((node, index): HttpIdentity[] => {
    const assigned = httpSteps[index]?.id
    return assigned ? [{ nodeId: node.id, previous: node.data.properties.stepId, assigned }] : []
  })
  const graph = {
    ...submission.graph,
    nodes: applyHttpIdentities(submission.graph.nodes, identities),
  }
  return {
    resource,
    fingerprint:
      identities.length === 0
        ? submission.fingerprint
        : automationSnapshot(submission.body.name, graph),
    identities,
  }
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
  // A receipt enriches still-present submitted nodes with their stored identity. Newer fields,
  // positions and edges stay as edited, and the canvas is not reloaded.
  fold: (current: AutomationContent, receipt: ReturnType<typeof acknowledge>) => ({
    content:
      receipt.identities.length === 0
        ? current
        : {
            ...current,
            graph: {
              ...current.graph,
              nodes: applyHttpIdentities(current.graph.nodes, receipt.identities),
            },
          },
    replaces: false,
  }),
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
