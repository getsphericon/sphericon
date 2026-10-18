import { Group, Progress, Stack, Text } from '@mantine/core'
import type { TFunction } from 'i18next'
import { useTranslation } from 'react-i18next'

import type { SiteBroadcastProgress } from '../../generated/site/types.gen.ts'

const MS_PER_MINUTE = 60_000
const MINUTES_PER_HOUR = 60

// The estimate is rendered coarsely on purpose ("about 3 hours"): it is derived from a
// rate, so a precise countdown would claim more than the server knows.
function remainingText(t: TFunction, completionAt: string): string {
  const minutes = Math.round((Date.parse(completionAt) - Date.now()) / MS_PER_MINUTE)
  if (minutes < 1) return t(($) => $.broadcasts.progress.almostDone)
  if (minutes < MINUTES_PER_HOUR) {
    return t(($) => $.broadcasts.progress.remainingMinutes, { count: minutes })
  }
  return t(($) => $.broadcasts.progress.remainingHours, {
    count: Math.round(minutes / MINUTES_PER_HOUR),
  })
}

interface BroadcastProgressProps {
  progress: SiteBroadcastProgress
}

/**
 * "Sent X of N, about H remaining" for a sending broadcast. A busy Integration (its Send
 * rate limit is spent) is not a blocked state: the bar just advances at the limit's pace.
 */
export function BroadcastProgress({ progress }: BroadcastProgressProps) {
  const { t } = useTranslation()
  const total = progress.processedCount + progress.remainingCount
  const percent = total === 0 ? 0 : (progress.processedCount / total) * 100

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
    </Stack>
  )
}
