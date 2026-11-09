import { assign, enqueueActions, fromPromise, setup } from 'xstate'

import { draftState } from '../components/draftState.ts'

// The editor session of a saved resource; its rules are docs/standards/ui.md, "Editor sessions".
// The session keeps the acknowledged content's fingerprint as `saved`, and the content it placed
// itself in `content`, whose revision is what stateful field editors re-read.

export interface EditorSubmission {
  // The fingerprint of the content the submission captured.
  fingerprint: string
}

// What a Save's acknowledgement tells the editor: the resource as stored, the fingerprint of what
// the server now holds (from the submission, never from the response), and whatever the adapter
// folds into content that may have moved on. Its details are emitted with `accepted`.
export interface EditorReceipt<TResource> {
  resource: TResource
  fingerprint: string
}

interface EditorOperation<TSubject, TResource, TContent> {
  request: (call: { subject: TSubject; signal: AbortSignal }) => Promise<TResource>
  // Whether the operation may start. `dirty` is false only when the event carried the current
  // fingerprint and it equals the acknowledged one.
  available: (state: { resource: TResource; dirty: boolean }) => boolean
  // The content an operation such as Revert puts in the editor, from its response.
  replaces?: (resource: TResource, subject: TSubject) => TContent
}

// What an editor of one kind of resource supplies. The mounted editor's content is reached through
// an EditorHandle given to each session.
export interface EditorAdapter<
  TSubject extends { resource: TResource },
  TResource,
  TContent,
  TSubmission extends EditorSubmission,
  TReceipt extends EditorReceipt<TResource>,
  TOperation extends string,
> {
  id: string
  // The content the editor opens with.
  open: (subject: TSubject) => TContent
  fingerprint: (content: TContent) => string
  // The submission Save sends, captured before any asynchronous work; null refuses invalid content.
  submit: (content: TContent) => TSubmission | null
  save: (call: {
    subject: TSubject
    submission: TSubmission
    signal: AbortSignal
  }) => Promise<TResource>
  acknowledge: (submission: TSubmission, resource: TResource) => TReceipt
  // Folds an acknowledgement into the current content, which may have moved on since the
  // submission. `replaces` is set when stateful field editors must re-read the result.
  fold: (current: TContent, receipt: TReceipt) => { content: TContent; replaces: boolean }
  // The other operations, named by their event (PUBLISH, REVERT, …). They act on stored content,
  // so the editor is read-only while one is pending.
  operations: Record<TOperation, EditorOperation<TSubject, TResource, TContent>>
}

// The mounted editor's current content.
export interface EditorHandle<TContent> {
  read: () => TContent
  write: (content: TContent) => void
}

// What the page does on an operation's outcome, run once the write is acknowledged or refused.
// Invalidation starts after the acknowledgement and is never part of the pending operation.
export interface EditorEffects<TResource, TOperation extends string> {
  store: (resource: TResource) => void
  accepted: (operation: TOperation | 'SAVE') => void
  failed: (operation: TOperation | 'SAVE', error: unknown) => void
  invalidate: () => void
}

// ADR 0065 for a resource with a draft above a live version (Automation, Rule).
export interface DraftResource {
  status: string
  live: object | null
  draft: object | null
}

function draftOf(resource: DraftResource, dirty: boolean) {
  return draftState({
    active: resource.status === 'active',
    hasLive: resource.live !== null,
    hasDraft: resource.draft !== null,
    dirty,
  })
}

export const draftGuards = {
  publishable: ({ resource, dirty }: { resource: DraftResource; dirty: boolean }) =>
    draftOf(resource, dirty).publishable,
  // A pending draft can be reverted; the editor asks first when it holds unsaved edits.
  revertible: ({ resource }: { resource: DraftResource }) => draftOf(resource, false).pending,
}

export interface EditorSessionContext<TSubject, TResource, TContent, TOperation extends string> {
  subject: TSubject
  resource: TResource
  // The fingerprint of the acknowledged content.
  saved: string
  content: { value: TContent; revision: number }
  operation: TOperation | null
  result: 'success' | 'error'
  editor: EditorHandle<TContent> | undefined
  effects: EditorEffects<TResource, TOperation> | undefined
}

// An operation's event (PUBLISH, REVERT, …) may carry the current fingerprint, which its guard
// compares with the acknowledged one. Its type is any string to xstate, which cannot type a
// machine's transitions over a generic event type; an event no operation names is ignored.
interface EditorOperationEvent {
  type: string
  current?: string
}

export type EditorSessionEvent<TResource, TSubmission> =
  | { type: 'SAVE'; submission: TSubmission }
  | { type: 'REFRESH'; resource: TResource }
  | EditorOperationEvent

export type EditorNotice<TResource, TReceipt, TOperation extends string> =
  | ({ type: 'accepted'; operation: TOperation | 'SAVE'; resource: TResource } & Partial<
      Omit<TReceipt, 'resource' | 'fingerprint'>
    >)
  | { type: 'failed'; operation: TOperation | 'SAVE'; error: unknown }

export function defineEditorSession<
  TSubject extends { resource: TResource },
  TResource,
  TContent,
  TSubmission extends EditorSubmission,
  TReceipt extends EditorReceipt<TResource>,
  TOperation extends string,
>(adapter: EditorAdapter<TSubject, TResource, TContent, TSubmission, TReceipt, TOperation>) {
  type Context = EditorSessionContext<TSubject, TResource, TContent, TOperation>
  type Event = EditorSessionEvent<TResource, TSubmission>
  type Notice = EditorNotice<TResource, TReceipt, TOperation>
  type Input = TSubject & {
    editor?: EditorHandle<TContent>
    effects?: EditorEffects<TResource, TOperation>
  }

  const isOperation = (type: string): type is TOperation => Object.hasOwn(adapter.operations, type)
  const operationOf = (event: Event): TOperation | null =>
    'submission' in event || 'resource' in event || !isOperation(event.type) ? null : event.type

  const types: { context?: Context; input?: Input; events?: Event; emitted?: Notice } = {}

  return setup({
    types,
    actors: {
      save: fromPromise(
        async ({
          input,
          signal,
        }: {
          input: { subject: TSubject; submission: TSubmission }
          signal: AbortSignal
        }) => {
          const resource = await adapter.save({ ...input, signal })
          return adapter.acknowledge(input.submission, resource)
        },
      ),
      operation: fromPromise(
        async ({
          input,
          signal,
        }: {
          input: { subject: TSubject; operation: TOperation }
          signal: AbortSignal
        }) => adapter.operations[input.operation].request({ subject: input.subject, signal }),
      ),
    },
    guards: {
      available: ({ context, event }) => {
        const operation = operationOf(event)
        return (
          operation !== null &&
          adapter.operations[operation].available({
            resource: context.resource,
            dirty: !('current' in event) || event.current !== context.saved,
          })
        )
      },
    },
    actions: {
      fail: enqueueActions(({ context, event, enqueue }) => {
        const operation = context.operation ?? 'SAVE'
        const error = 'error' in event ? event.error : undefined
        enqueue.assign({ result: 'error', operation: null })
        enqueue(() => context.effects?.failed(operation, error))
        enqueue.emit({ type: 'failed', operation, error } as Notice)
      }),
    },
  }).createMachine({
    id: adapter.id,
    initial: 'idle',
    context: ({ input }) => {
      const value = adapter.open(input)
      return {
        subject: input,
        resource: input.resource,
        saved: adapter.fingerprint(value),
        content: { value, revision: 0 },
        operation: null,
        result: 'error',
        editor: input.editor,
        effects: input.effects,
      }
    },
    // A query refresh updates the resource's metadata, never the content or its receipt.
    on: {
      REFRESH: {
        actions: assign(({ event }) => ('resource' in event ? { resource: event.resource } : {})),
      },
    },
    states: {
      idle: {
        on: {
          SAVE: 'saving',
          '*': {
            target: 'operating',
            guard: 'available',
            actions: assign(({ event }) => ({ operation: operationOf(event) })),
          },
        },
      },
      // Edits stay enabled while Save is pending: its submission is already captured.
      saving: {
        invoke: {
          src: 'save',
          input: ({ context, event }) => {
            if (!('submission' in event)) {
              throw new Error(`Save started by ${event.type}`)
            }
            return { subject: context.subject, submission: event.submission }
          },
          onDone: {
            target: 'idle',
            actions: enqueueActions(({ context, event, enqueue }) => {
              const receipt = event.output
              const { editor, effects } = context
              const current = editor?.read()
              const folded = current === undefined ? null : adapter.fold(current, receipt)
              enqueue.assign({
                resource: receipt.resource,
                saved: receipt.fingerprint,
                result: 'success',
                ...(folded?.replaces && {
                  content: { value: folded.content, revision: context.content.revision + 1 },
                }),
              })
              if (folded && folded.content !== current) {
                enqueue(() => editor?.write(folded.content))
              }
              enqueue(() => effects?.store(receipt.resource))
              enqueue(() => effects?.accepted('SAVE'))
              const { fingerprint: _fingerprint, ...details } = receipt
              enqueue.emit({ type: 'accepted', operation: 'SAVE', ...details })
              enqueue(() => effects?.invalidate())
            }),
          },
          onError: { target: 'idle', actions: 'fail' },
        },
      },
      // The other operations act on stored content, so the editor is read-only meanwhile.
      operating: {
        tags: 'readOnly',
        invoke: {
          src: 'operation',
          input: ({ context }) => {
            if (context.operation === null) {
              throw new Error('No operation is pending')
            }
            return { subject: context.subject, operation: context.operation }
          },
          onDone: {
            target: 'idle',
            actions: enqueueActions(({ context, event, enqueue }) => {
              const resource = event.output
              const { editor, effects, operation } = context
              if (operation === null) {
                return
              }
              const value = adapter.operations[operation].replaces?.(resource, context.subject)
              enqueue.assign({
                resource,
                result: 'success',
                operation: null,
                ...(value !== undefined && {
                  saved: adapter.fingerprint(value),
                  content: { value, revision: context.content.revision + 1 },
                }),
              })
              if (value !== undefined) {
                enqueue(() => editor?.write(value))
              }
              enqueue(() => effects?.store(resource))
              enqueue(() => effects?.accepted(operation))
              enqueue.emit({ type: 'accepted', operation, resource } as Notice)
              enqueue(() => effects?.invalidate())
            }),
          },
          onError: { target: 'idle', actions: 'fail' },
        },
      },
    },
  })
}
