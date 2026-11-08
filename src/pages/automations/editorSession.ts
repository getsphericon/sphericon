import { assertEvent, assign, emit, fromPromise, setup } from 'xstate'

import { draftState } from '../../components/draftState.ts'
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

type Operation = 'save' | 'publish' | 'revert'
type Path = SiteAutomationsUpdateData['path']
interface Submission {
  body: SiteAutomationsUpdateData['body']
  fingerprint: string
  dropped: number
}

// Capture the persisted representation before any asynchronous work. SDK graph data can contain
// nested references; neither a later edit nor a query refresh may change this request or its receipt.
export function automationSubmission(name: string, graph: AutomationGraph): Submission | null {
  const { dropped, triggerEvent, ...definition } = graphToSteps(graph)
  if (triggerEvent === null) {
    return null
  }
  return {
    body: structuredClone({ name: name.trim(), triggerEvent, ...definition }),
    fingerprint: automationSnapshot(name, graph),
    dropped,
  }
}

interface SessionInput {
  path: Path
  resource: SiteAutomationResource
  graph: AutomationGraph
  labels: Record<string, string>
}
interface SessionContext extends Omit<SessionInput, 'graph'> {
  saved: string
  canvas: { graph: AutomationGraph; revision: number }
  result: 'success' | 'error'
}
type SessionEvent =
  | { type: 'SAVE'; submission: Submission }
  | { type: 'PUBLISH'; current: string }
  | { type: 'REVERT' }
  | { type: 'REFRESH'; resource: SiteAutomationResource }
type SessionNotice =
  | { type: 'accepted'; operation: Operation; resource: SiteAutomationResource }
  | { type: 'failed'; operation: Operation; error: unknown }

const types: {
  context?: SessionContext
  input?: SessionInput
  events?: SessionEvent
  emitted?: SessionNotice
} = {}

// Current content belongs to the form/canvas, the submission to the invoked actor, and the
// acknowledged fingerprint to this session. Only explicit Revert replaces the canvas.
export const editorSession = setup({
  types,
  actors: {
    save: fromPromise(
      async ({
        input,
        signal,
      }: {
        input: { path: Path; submission: Submission }
        signal: AbortSignal
      }) => {
        const { data } = await siteAutomationsUpdate({
          path: input.path,
          body: input.submission.body,
          signal,
          throwOnError: true,
        })
        return { resource: data, fingerprint: input.submission.fingerprint }
      },
    ),
    publish: fromPromise(async ({ input, signal }: { input: Path; signal: AbortSignal }) => {
      const { data } = await siteAutomationsPublish({ path: input, signal, throwOnError: true })
      return data
    }),
    revert: fromPromise(async ({ input, signal }: { input: Path; signal: AbortSignal }) => {
      const { data } = await siteAutomationsRevert({ path: input, signal, throwOnError: true })
      return data
    }),
  },
  guards: {
    publishable: ({ context, event }) => {
      assertEvent(event, 'PUBLISH')
      return draftState({
        active: context.resource.status === 'active',
        hasLive: context.resource.live !== null,
        hasDraft: context.resource.draft !== null,
        dirty: event.current !== context.saved,
      }).publishable
    },
    revertible: ({ context }) => context.resource.live !== null && context.resource.draft !== null,
  },
}).createMachine({
  id: 'automationEditor',
  initial: 'idle',
  context: ({ input }) => ({
    path: input.path,
    resource: input.resource,
    labels: input.labels,
    saved: automationSnapshot(input.resource.name, input.graph),
    canvas: { graph: input.graph, revision: 0 },
    result: 'error',
  }),
  // A query refresh may update lifecycle metadata, never the local canvas or its receipt.
  on: { REFRESH: { actions: assign({ resource: ({ event }) => event.resource }) } },
  states: {
    idle: {
      on: {
        SAVE: 'saving',
        PUBLISH: { target: 'publishing', guard: 'publishable' },
        REVERT: { target: 'reverting', guard: 'revertible' },
      },
    },
    saving: {
      invoke: {
        src: 'save',
        input: ({ context, event }) => {
          assertEvent(event, 'SAVE')
          return { path: context.path, submission: event.submission }
        },
        onDone: {
          target: 'idle',
          actions: [
            assign(({ event }) => ({
              resource: event.output.resource,
              saved: event.output.fingerprint,
              result: 'success',
            })),
            emit(({ event }): SessionNotice => ({
              type: 'accepted',
              operation: 'save',
              resource: event.output.resource,
            })),
          ],
        },
        onError: {
          target: 'idle',
          actions: [
            assign({ result: 'error' }),
            emit(({ event }): SessionNotice => ({
              type: 'failed',
              operation: 'save',
              error: event.error,
            })),
          ],
        },
      },
    },
    publishing: {
      tags: 'readOnly',
      invoke: {
        src: 'publish',
        input: ({ context }) => context.path,
        onDone: {
          target: 'idle',
          actions: [
            assign({ resource: ({ event }) => event.output }),
            emit(({ event }): SessionNotice => ({
              type: 'accepted',
              operation: 'publish',
              resource: event.output,
            })),
          ],
        },
        onError: {
          target: 'idle',
          actions: emit(({ event }): SessionNotice => ({
            type: 'failed',
            operation: 'publish',
            error: event.error,
          })),
        },
      },
    },
    reverting: {
      tags: 'readOnly',
      invoke: {
        src: 'revert',
        input: ({ context }) => context.path,
        onDone: {
          target: 'idle',
          actions: [
            assign(({ context, event }) => {
              const live = event.output.live
              if (!live) {
                throw new Error('Revert returned an Automation without a live version')
              }
              const graph = stepsToGraph(live, live.steps, context.labels)
              return {
                resource: event.output,
                saved: automationSnapshot(event.output.name, graph),
                canvas: { graph, revision: context.canvas.revision + 1 },
              }
            }),
            emit(({ event }): SessionNotice => ({
              type: 'accepted',
              operation: 'revert',
              resource: event.output,
            })),
          ],
        },
        onError: {
          target: 'idle',
          actions: emit(({ event }): SessionNotice => ({
            type: 'failed',
            operation: 'revert',
            error: event.error,
          })),
        },
      },
    },
  },
})
