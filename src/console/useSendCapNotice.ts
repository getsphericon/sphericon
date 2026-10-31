import { useTranslation } from 'react-i18next'

import { useNotify } from '../hooks/useNotify.ts'

// Both Send cap changes answer 200 with whether anything changed, so a no-op says so instead of
// looking like a success.
export function useSendCapNotice() {
  const { t } = useTranslation()
  const notify = useNotify()
  return (change: { changed: boolean }) => {
    notify(
      change.changed
        ? {
            color: 'teal',
            title: t(($) => $.notifications.successTitle),
            message: t(($) => $.console.workspace.sendCap.done),
          }
        : {
            color: 'blue',
            title: t(($) => $.console.workspace.unchangedTitle),
            message: t(($) => $.console.workspace.sendCap.unchanged),
          },
    )
  }
}
