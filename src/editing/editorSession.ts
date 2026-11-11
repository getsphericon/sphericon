import { assign, enqueueActions, fromPromise, setup } from 'xstate'

import { draftState } from '../components/draftState.ts'
import { isRevisionChangedError } from '../utils/apiErrors.ts'

// The editor session of a saved resource; its rules are docs/standards/ui.md, "Editor sessions".
// The session keeps the acknowledged content's fingerprint as `saved`, and the content it placed
// itself in `content`, whose revision is what stateful field editors re-read. A resource with a
// revision (ADR 0119) keeps a base: the revision of what the editor shows, which every write names.

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

// A write is never aborted: it outlives the editor that started it (docs/standards/ui.md).
export interface EditorOperation<TSubject, TResource, TContent> {
  // `input` is what the operation's event carried (the time Schedule sends at), unchecked: an
  // operation that takes one reads it with its request's schema.
  request: (call: {
    subject: TSubject
    input: unknown
    revision: number | undefined
  }) => Promise<TResource>
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
  // `revision` is the base the edit was made on, for a resource that has one.
  save: (call: {
    subject: TSubject
    submission: TSubmission
    revision: number | undefined
  }) => Promise<TResource>
  // The resource's revision (ADR 0119), or undefined for a resource without one.
  revision?: (resource: TResource) => number | undefined
  acknowledge: (submission: TSubmission, resource: TResource) => TReceipt
  // Folds an acknowledgement into the current content, which may have moved on since the
  // submission. `replaces` is set when stateful field editors must re-read the result: the content
  // is then the server's form of the submission, and becomes the acknowledged content.
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
// Invalidation starts after the acknowledgement and is never part of the pending operation. The
// cache's two, `store` and `invalidate`, belong to the write and run even after the editor closed;
// the others only while it is open.
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

// The base revision a write of a resource with a revision names (ADR 0119): the session always
// holds one for such a resource, so its absence is a fault of the wiring.
export function based(revision: number | undefined): number {
  if (revision === undefined) {
    throw new Error('A resource with a revision is written with the revision it was read with')
  }
  return revision
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
  // The fingerprint of the acknowledged content, and of the submission a pending Save carries.
  saved: string
  submitted: string | null
  content: { value: TContent; revision: number }
  // The revision of the stored state the editor shows, which a write names (ADR 0119): moved only
  // when an acknowledgement is folded in or the session replaces the content, never by a refresh.
  base: number | undefined
  // Whether the resource changed since the editor showed it: a write was refused with
  // revision_changed, or a refresh brought a newer revision than the base.
  changed: boolean
  operation: TOperation | null
  input: unknown
  result: 'success' | 'error'
  editor: EditorHandle<TContent> | undefined
  effects: EditorEffects<TResource, TOperation> | undefined
}

// An operation's event (PUBLISH, REVERT, …) may carry the current fingerprint, which its guard
// compares with the acknowledged one, and the operation's input. Its type is any string to xstate,
// which cannot type a machine's transitions over a generic event type; an event no operation names
// is ignored.
interface EditorOperationEvent {
  type: string
  current?: string
  input?: unknown
}

export type EditorSessionEvent<TResource, TSubmission> =
  | { type: 'SAVE'; submission: TSubmission }
  | { type: 'REFRESH'; resource: TResource }
  // Load the saved version: the content becomes the resource's, read again by the page.
  | { type: 'RELOAD'; resource: TResource }
  // Save mine over it: the submission is saved on the revision of the resource read again.
  | { type: 'OVERWRITE'; submission: TSubmission; resource: TResource }
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
  const revisionOf = (resource: TResource) => adapter.revision?.(resource)
  const operationOf = (event: Event): TOperation | null =>
    'submission' in event || 'resource' in event || !isOperation(event.type) ? null : event.type

  const types: { context?: Context; input?: Input; events?: Event; emitted?: Notice } = {}

  return setup({
    types,
    actors: {
      // A write updates the cache itself, so its acknowledgement reaches the cache even when the
      // session has stopped and its outcome is ignored.
      save: fromPromise(
        async ({
          input: { subject, submission, revision, effects },
        }: {
          input: {
            subject: TSubject
            submission: TSubmission
            revision: number | undefined
            effects: EditorEffects<TResource, TOperation> | undefined
          }
        }) => {
          const resource = await adapter.save({ subject, submission, revision })
          effects?.store(resource)
          effects?.invalidate()
          return adapter.acknowledge(submission, resource)
        },
      ),
      operation: fromPromise(
        async ({
          input: { subject, operation, input, revision, effects },
        }: {
          input: {
            subject: TSubject
            operation: TOperation
            input: unknown
            revision: number | undefined
            effects: EditorEffects<TResource, TOperation> | undefined
          }
        }) => {
          const resource = await adapter.operations[operation].request({
            subject,
            input,
            revision,
          })
          effects?.store(resource)
          effects?.invalidate()
          return resource
        },
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
        enqueue.assign({ result: 'error', operation: null, input: undefined, submitted: null })
        // A write over a change the editor has not seen: the content stays, unsaved, and the page
        // offers to load the saved version or save over it instead of a failure.
        if (isRevisionChangedError(error)) {
          enqueue.assign({ changed: true })
          return
        }
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
        submitted: null,
        content: { value, revision: 0 },
        base: revisionOf(input.resource),
        changed: false,
        operation: null,
        input: undefined,
        result: 'error',
        editor: input.editor,
        effects: input.effects,
      }
    },
    // A query refresh updates the resource's metadata, never the content, its receipt or the base;
    // a newer revision than the base says the resource changed since the editor showed it.
    on: {
      REFRESH: {
        actions: assign(({ context, event }) => {
          if (!('resource' in event)) {
            return {}
          }
          const refreshed = revisionOf(event.resource)
          const newer =
            refreshed !== undefined && context.base !== undefined && refreshed > context.base
          return { resource: event.resource, ...(newer && { changed: true }) }
        }),
      },
    },
    states: {
      idle: {
        on: {
          SAVE: {
            target: 'saving',
            actions: assign(({ event }) =>
              'submission' in event ? { submitted: event.submission.fingerprint } : {},
            ),
          },
          // The saved version replaces the content, which becomes the acknowledged one.
          RELOAD: {
            actions: enqueueActions(({ context, event, enqueue }) => {
              if (!('resource' in event)) {
                return
              }
              const value = adapter.open({ ...context.subject, resource: event.resource })
              enqueue.assign({
                resource: event.resource,
                base: revisionOf(event.resource),
                changed: false,
                saved: adapter.fingerprint(value),
                content: { value, revision: context.content.revision + 1 },
              })
              enqueue(() => context.editor?.write(value))
            }),
          },
          // The submission is saved on the revision of the resource read again.
          OVERWRITE: {
            target: 'saving',
            actions: assign(({ event }) =>
              'submission' in event && 'resource' in event
                ? {
                    resource: event.resource,
                    base: revisionOf(event.resource),
                    changed: false,
                    submitted: event.submission.fingerprint,
                  }
                : {},
            ),
          },
          '*': {
            target: 'operating',
            guard: 'available',
            actions: assign(({ event }) => ({
              operation: operationOf(event),
              input: 'input' in event ? event.input : undefined,
            })),
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
            return {
              subject: context.subject,
              submission: event.submission,
              revision: context.base,
              effects: context.effects,
            }
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
                base: revisionOf(receipt.resource),
                changed: false,
                saved: receipt.fingerprint,
                submitted: null,
                result: 'success',
                ...(folded?.replaces && {
                  saved: adapter.fingerprint(folded.content),
                  content: { value: folded.content, revision: context.content.revision + 1 },
                }),
              })
              if (folded && folded.content !== current) {
                enqueue(() => editor?.write(folded.content))
              }
              enqueue(() => effects?.accepted('SAVE'))
              const { fingerprint: _fingerprint, ...details } = receipt
              enqueue.emit({ type: 'accepted', operation: 'SAVE', ...details })
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
            return {
              subject: context.subject,
              operation: context.operation,
              input: context.input,
              revision: context.base,
              effects: context.effects,
            }
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
                base: revisionOf(resource),
                result: 'success',
                operation: null,
                input: undefined,
                ...(value !== undefined && {
                  saved: adapter.fingerprint(value),
                  content: { value, revision: context.content.revision + 1 },
                }),
              })
              if (value !== undefined) {
                enqueue(() => editor?.write(value))
              }
              enqueue(() => effects?.accepted(operation))
              enqueue.emit({ type: 'accepted', operation, resource } as Notice)
            }),
          },
          onError: { target: 'idle', actions: 'fail' },
        },
      },
    },
  })
}
