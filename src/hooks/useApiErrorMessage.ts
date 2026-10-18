import { useTranslation } from 'react-i18next'

import { getApiErrorMessage, getRateLimitWait, isRateLimitedError } from '../utils/apiErrors.ts'

// The message to show for a failed API call: the localized "too many requests" text for a
// 429 (ADR 0025), with how long to wait when the throttle says so; the API detail otherwise,
// then the fallback.
export function useApiErrorMessage() {
  const { t } = useTranslation()
  return (error: unknown, fallback: string) => {
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
