import { useTranslation } from 'react-i18next'

import { useNotify } from '../hooks/useNotify.ts'

// The console's actions answer 200 with whether anything changed, so a no-op says so
// instead of looking like a success.
export function useChangeNotice(changedMessage: string, unchangedMessage: string) {
  const { t } = useTranslation()
  const notify = useNotify()
  return (change: { changed: boolean }) => {
    notify(
      change.changed
        ? {
            color: 'teal',
            title: t(($) => $.notifications.successTitle),
            message: changedMessage,
          }
        : {
            color: 'blue',
            title: t(($) => $.console.unchangedTitle),
            message: unchangedMessage,
          },
    )
  }
}
