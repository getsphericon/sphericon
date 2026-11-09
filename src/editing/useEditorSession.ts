import { useMachine } from '@xstate/react'
import { useLatest } from 'ahooks'
import { useCallback, useEffect } from 'react'
import { waitFor } from 'xstate'

import type {
  defineEditorSession,
  EditorAdapter,
  EditorEffects,
  EditorHandle,
  EditorReceipt,
  EditorSubmission,
} from './editorSession.ts'

type EditorSessionMachine<
  TSubject extends { resource: TResource },
  TResource,
  TContent,
  TSubmission extends EditorSubmission,
  TReceipt extends EditorReceipt<TResource>,
  TOperation extends string,
> = ReturnType<
  typeof defineEditorSession<TSubject, TResource, TContent, TSubmission, TReceipt, TOperation>
>

// Runs an editor session for a mounted editor: the page reads the current content through its
// handle and never fingerprints or captures it itself. The handles and effects are read on each
// call from the latest render, so the session never acts through a stale form, toast or query
// client. A new `resource` from the query updates the session's metadata only.
export function useEditorSession<
  TSubject extends { resource: TResource },
  TResource,
  TContent,
  TSubmission extends EditorSubmission,
  TReceipt extends EditorReceipt<TResource>,
  TOperation extends string,
>(
  adapter: EditorAdapter<TSubject, TResource, TContent, TSubmission, TReceipt, TOperation>,
  machine: EditorSessionMachine<TSubject, TResource, TContent, TSubmission, TReceipt, TOperation>,
  {
    subject,
    ...handles
  }: {
    subject: TSubject
    editor: EditorHandle<TContent>
    effects: EditorEffects<TResource, TOperation>
  },
) {
  const latest = useLatest(handles)
  const [snapshot, send, actor] = useMachine(machine, {
    input: {
      ...subject,
      editor: {
        read: () => latest.current.editor.read(),
        write: (content) => latest.current.editor.write(content),
      },
      effects: {
        store: (resource) => latest.current.effects.store(resource),
        accepted: (operation) => latest.current.effects.accepted(operation),
        failed: (operation, error) => latest.current.effects.failed(operation, error),
        invalidate: () => latest.current.effects.invalidate(),
      },
    },
  })

  const { resource } = subject
  useEffect(() => {
    send({ type: 'REFRESH', resource })
  }, [send, resource])

  // Whether content the page holds (a value it re-renders on) differs from the acknowledged one.
  const { saved } = snapshot.context
  const dirty = (content: TContent) => adapter.fingerprint(content) !== saved

  // An operation's event carries the current content's fingerprint for its guard.
  const operationEvent = (type: TOperation) => ({
    type,
    current: adapter.fingerprint(latest.current.editor.read()),
  })
  const can = (type: TOperation) => snapshot.can(operationEvent(type))
  const run = (type: TOperation) => {
    send(operationEvent(type))
  }

  // Captures the current content and sends it, resolving with the outcome: `busy` when another
  // operation is pending (nothing is captured), `error` when the adapter or `review` refuses the
  // submission, or when the session stopped before the acknowledgement, which then changes nothing.
  // `review` shows the page's own refusals (validation, a warning) and may hold the Save back.
  const save = useCallback(
    async (
      review: (submission: TSubmission | null) => boolean,
    ): Promise<'success' | 'error' | 'busy'> => {
      if (!actor.getSnapshot().matches('idle')) {
        return 'busy'
      }
      const submission = adapter.submit(latest.current.editor.read())
      const accepted = review(submission)
      if (!submission || !accepted) {
        return 'error'
      }
      actor.send({ type: 'SAVE', submission })
      try {
        const settled = await waitFor(actor, (state) => state.matches('idle'))
        return settled.context.result
      } catch {
        return 'error'
      }
    },
    [actor, adapter, latest],
  )

  return { snapshot, dirty, can, run, save }
}
