import { useTranslation } from 'react-i18next'

import {
  getApiErrorMessage,
  getRateLimitWait,
  getSoleOwnedWorkspaces,
  getUsedBy,
  isRateLimitedError,
} from '../utils/apiErrors.ts'

// The message to show for a failed API call: the users a refused removal names (ADR 0049); the
// Workspaces a refused User deletion names (ADR 0050); the
// localized "too many requests" text for a 429 (ADR 0025), with how long to wait when the
// throttle says so; the API detail otherwise, then the fallback.
export function useApiErrorMessage() {
  const { t } = useTranslation()
  return (error: unknown, fallback: string) => {
    const usedBy = getUsedBy(error)
    if (usedBy) {
      const kinds = {
        broadcast: t(($) => $.usedByKind.broadcast),
        rule: t(($) => $.usedByKind.rule),
        automation: t(($) => $.usedByKind.automation),
        segment: t(($) => $.usedByKind.segment),
        enrollment: t(($) => $.usedByKind.enrollment),
        firing: t(($) => $.usedByKind.firing),
      }
      const users = usedBy
        .map((user) => {
          if (user.kind === 'enrollment') {
            return t(($) => $.notifications.usedByEnrollments, {
              count: user.count ?? 1,
              name: user.name,
            })
          }
          if (user.kind === 'firing') {
            return t(($) => $.notifications.usedByFirings, {
              count: user.count ?? 1,
              name: user.name,
            })
          }
          return `${kinds[user.kind]} "${user.name}"`
        })
        .join(', ')
      return t(($) => $.notifications.inUse, { users })
    }
    const soleOwned = getSoleOwnedWorkspaces(error)
    if (soleOwned) {
      return t(($) => $.notifications.soleOwner, {
        workspaces: soleOwned.map((workspace) => workspace.name).join(', '),
      })
    }
    const wait = getRateLimitWait(error)
    if (wait) {
      return wait.unit === 'minutes'
        ? t(($) => $.notifications.rateLimitedMinutes, { count: wait.count })
        : t(($) => $.notifications.rateLimitedSeconds, { count: wait.count })
    }
    return isRateLimitedError(error)
      ? t(($) => $.notifications.rateLimited)
      : getApiErrorMessage(error, fallback)
  }
}
