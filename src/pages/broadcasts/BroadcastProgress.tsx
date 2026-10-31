import { Group, Progress, Stack, Text } from '@mantine/core'
import type { TFunction } from 'i18next'
import { useTranslation } from 'react-i18next'

import type { SiteBroadcastProgress } from '../../generated/site/types.gen.ts'

const MS_PER_MINUTE = 60_000
const MINUTES_PER_HOUR = 60
const HOURS_PER_DAY = 24
// Past two days an hour count reads poorly; a Send cap can stretch a send over days.
const DAYS_FROM_HOURS = 48

// The estimate is rendered coarsely on purpose ("about 3 hours"): it is derived from a
// rate, so a precise countdown would claim more than the server knows.
function remainingText(t: TFunction, completionAt: string): string {
  const minutes = Math.round((Date.parse(completionAt) - Date.now()) / MS_PER_MINUTE)
  if (minutes < 1) {
    return t(($) => $.broadcasts.progress.almostDone)
  }
  if (minutes < MINUTES_PER_HOUR) {
    return t(($) => $.broadcasts.progress.remainingMinutes, { count: minutes })
  }
  const hours = Math.round(minutes / MINUTES_PER_HOUR)
  if (hours < DAYS_FROM_HOURS) {
    return t(($) => $.broadcasts.progress.remainingHours, { count: hours })
  }
  return t(($) => $.broadcasts.progress.remainingDays, {
    count: Math.round(hours / HOURS_PER_DAY),
  })
}

interface BroadcastProgressProps {
  progress: SiteBroadcastProgress
}

/**
 * "Sent X of N, about H remaining" for a sending broadcast, and what slows it: the Send cap the
 * platform set or the Integration's Provider limit, so a slow send is not blamed on the wrong one.
 * A busy limit is not a blocked state: the bar just advances at its pace.
 */
export function BroadcastProgress({ progress }: BroadcastProgressProps) {
  const { t } = useTranslation()
  const total = progress.processedCount + progress.remainingCount
  const percent = total === 0 ? 0 : (progress.processedCount / total) * 100
  const limit = progress.limitedBy

  return (
    <Stack gap="xs">
      <Group gap="xs">
        <Text size="sm">
          {t(($) => $.broadcasts.progress.summary, {
            processed: progress.processedCount,
            total,
          })}
        </Text>
        {progress.estimatedCompletionAt ? (
          <Text size="sm" c="dimmed">
            {remainingText(t, progress.estimatedCompletionAt)}
          </Text>
        ) : null}
      </Group>
      <Progress value={percent} animated={progress.remainingCount > 0} />
      {limit ? (
        <Text size="xs" c="dimmed">
          {t(($) => $.broadcasts.progress.limitedBy[limit])}
        </Text>
      ) : null}
    </Stack>
  )
}
