import { useTranslation } from 'react-i18next'

import { siteBroadcastsCancelMutation } from '../../generated/site/@tanstack/react-query.gen.ts'
import type { SiteBroadcastResource } from '../../generated/site/types.gen.ts'
import { useConfirmation } from '../../hooks/useConfirmation.tsx'
import { useNotify } from '../../hooks/useNotify.ts'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'

// Cancel of a sending Broadcast (ADR 0150), from its list row or its report. It cannot be undone,
// so the confirmation names how many recipients have not been sent it yet and that what went out
// cannot be recalled; the toast after it says how many the cancel left unsent.
export function useCancelBroadcast(workspaceId: string) {
  const { t } = useTranslation()
  const confirm = useConfirmation()
  const notify = useNotify()
  const cancelBroadcast = useResourceMutation({
    mutation: siteBroadcastsCancelMutation(),
    errorTitle: t(($) => $.alerts.broadcastCancelErrorTitle),
    onDone: (cancelled) => {
      notify({
        color: 'teal',
        title: t(($) => $.notifications.successTitle),
        message: t(($) => $.notifications.broadcastCancelled, { count: cancelled.unsent ?? 0 }),
      })
    },
  })

  return (broadcast: SiteBroadcastResource) =>
    confirm({
      title: t(($) => $.broadcasts.cancel.title, { name: broadcast.name }),
      description: t(($) => $.broadcasts.cancel.description, {
        count: broadcast.stats.pendingCount,
      }),
      confirmLabel: t(($) => $.broadcasts.cancel.confirm),
      cancelLabel: t(($) => $.broadcasts.cancel.keep),
      onConfirm: () => cancelBroadcast.mutate({ path: { workspaceId, id: broadcast.id } }),
    })
}
