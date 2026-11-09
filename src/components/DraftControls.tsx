import { Badge, Button, Group } from '@mantine/core'
import type { UseMutationOptions } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { useConfirmation } from '../hooks/useConfirmation.tsx'
import { useResourceMutation } from '../hooks/useResourceMutation.ts'
import { draftState } from './draftState.ts'

interface IdPath {
  path: { workspaceId: string; id: string }
}

// DraftControls shows what an Automation or a Rule has beside its live version (ADR 0065): a
// pending-draft badge, Publish and Revert, which run the generated mutations it is given (each
// refreshes its resource's queries). `dirty` holds Publish back while the editor has unsaved
// changes. A never-published draft is not offered Revert.
export function DraftControls<TPublished, TPublishError, TReverted, TRevertError>({
  workspaceId,
  resource,
  dirty = false,
  publishMutation,
  revertMutation,
  messages,
}: {
  workspaceId: string
  resource: { id: string; status: string; live: object | null; draft: object | null }
  dirty?: boolean
  publishMutation: UseMutationOptions<TPublished, TPublishError, IdPath>
  revertMutation: UseMutationOptions<TReverted, TRevertError, IdPath>
  messages: { published: string; reverted: string; errorTitle: string }
}) {
  const path = { workspaceId, id: resource.id }
  const publish = useResourceMutation({
    mutation: publishMutation,
    successMessage: messages.published,
    errorTitle: messages.errorTitle,
  })
  const revert = useResourceMutation({
    mutation: revertMutation,
    successMessage: messages.reverted,
    errorTitle: messages.errorTitle,
  })
  const { pending, publishable } = draftState({
    active: resource.status === 'active',
    hasLive: resource.live !== null,
    hasDraft: resource.draft !== null,
    dirty,
  })

  return (
    <DraftActions
      pending={pending}
      publishable={publishable}
      publishing={publish.isPending}
      reverting={revert.isPending}
      onPublish={() => publish.mutate({ path })}
      onRevert={() => revert.mutate({ path })}
    />
  )
}

// The same draft actions can be owned by a resource mutation or by an editor session. While the
// editor holds unsaved edits, `unsaved` is its leave dialog's message, and Revert asks first in
// those words: it discards them.
export function DraftActions({
  pending,
  publishable,
  publishing,
  reverting,
  disabled = false,
  unsaved,
  onPublish,
  onRevert,
}: {
  pending: boolean
  publishable: boolean
  publishing: boolean
  reverting: boolean
  disabled?: boolean
  unsaved?: string
  onPublish: () => void
  onRevert: () => void
}) {
  const { t } = useTranslation()
  const confirm = useConfirmation()
  const revert = () => {
    if (unsaved === undefined) {
      onRevert()
      return
    }
    confirm({
      title: t(($) => $.draft.revertUnsavedTitle),
      description: unsaved,
      confirmLabel: t(($) => $.draft.revert),
      cancelLabel: t(($) => $.leave.stay),
      onConfirm: onRevert,
    })
  }
  return (
    <Group gap="xs" wrap="nowrap">
      {pending && (
        <Badge color="yellow" variant="light">
          {t(($) => $.draft.pending)}
        </Badge>
      )}
      {pending && (
        <Button variant="default" loading={reverting} disabled={disabled} onClick={revert}>
          {t(($) => $.draft.revert)}
        </Button>
      )}
      <Button disabled={disabled || !publishable} loading={publishing} onClick={onPublish}>
        {t(($) => $.draft.publish)}
      </Button>
    </Group>
  )
}
