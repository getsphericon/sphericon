import { Badge, Group, NumberFormatter, Progress, Stack, Text } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import type { SiteSendLimitStatus, SiteSendLimitValue } from '../../generated/site/types.gen.ts'

function LimitLine({ label, value }: { label: string; value: SiteSendLimitValue }) {
  const { t } = useTranslation()
  return (
    <Group gap="xs" wrap="nowrap">
      <Text size="sm">
        {label}:{' '}
        {value.limit === null ? (
          t(($) => $.settings.integrations.limits.unlimited)
        ) : (
          <NumberFormatter value={value.limit} thousandSeparator=" " />
        )}
      </Text>
      {value.source ? (
        <Badge size="xs" variant="light" color={value.source === 'manual' ? 'blue' : 'grape'}>
          {value.source === 'manual'
            ? t(($) => $.settings.integrations.limits.sourceManual)
            : t(($) => $.settings.integrations.limits.sourceProvider)}
        </Badge>
      ) : null}
    </Group>
  )
}

// The effective Send rate limit of an Integration, where each ceiling comes from and
// the last 24 hours of usage (a bar against the daily ceiling when there is one).
export function SendLimitStatus({ status }: { status: SiteSendLimitStatus }) {
  const { t } = useTranslation()
  const daily = status.perDay.limit
  return (
    <Stack gap={4}>
      <LimitLine
        label={t(($) => $.settings.integrations.limits.perSecondShort)}
        value={status.perSecond}
      />
      <LimitLine
        label={t(($) => $.settings.integrations.limits.perDayShort)}
        value={status.perDay}
      />
      <Text size="xs" c="dimmed">
        {t(($) => $.settings.integrations.limits.sentLast24h)}:{' '}
        <NumberFormatter value={status.sentLast24h} thousandSeparator=" " />
        {daily === null ? null : (
          <>
            {' / '}
            <NumberFormatter value={daily} thousandSeparator=" " />
          </>
        )}
      </Text>
      {daily === null ? null : (
        <Progress
          size="xs"
          value={Math.min(100, (status.sentLast24h / daily) * 100)}
          aria-label={t(($) => $.settings.integrations.limits.sentLast24h)}
        />
      )}
    </Stack>
  )
}
