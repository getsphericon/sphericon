import { Badge, Group, NumberFormatter, Progress, Stack, Text } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import type {
  SiteProviderLimitStatus,
  SiteProviderLimitValue,
} from '../../generated/site/types.gen.ts'
import { useProviderLimitWarnings } from './providerLimit.ts'

function LimitLine({ label, value }: { label: string; value: SiteProviderLimitValue }) {
  const { t } = useTranslation()
  return (
    <Group gap="xs" wrap="nowrap">
      <Text size="sm">
        {label}:{' '}
        {value.limit === null ? (
          t(($) => $.settings.integrations.providerLimit.unlimited)
        ) : (
          <NumberFormatter value={value.limit} thousandSeparator=" " />
        )}
      </Text>
      {value.source ? (
        <Badge size="xs" variant="light" color={value.source === 'manual' ? 'blue' : 'grape'}>
          {value.source === 'manual'
            ? t(($) => $.settings.integrations.providerLimit.sourceManual)
            : t(($) => $.settings.integrations.providerLimit.sourceProvider)}
        </Badge>
      ) : null}
    </Group>
  )
}

// The effective Provider limit of an Integration, where each ceiling comes from and
// the last 24 hours of usage (a bar against the daily ceiling when there is one). withWarnings
// labels the row with its warnings, which the alerts above the list explain.
export function ProviderLimitStatus({
  status,
  withWarnings = false,
}: {
  status: SiteProviderLimitStatus
  withWarnings?: boolean
}) {
  const { t } = useTranslation()
  const flagged = useProviderLimitWarnings().filter(
    (copy) => withWarnings && status.warnings.includes(copy.warning),
  )
  const daily = status.perDay.limit
  return (
    <Stack gap={4}>
      {flagged.length === 0 ? null : (
        <Group gap={4}>
          {flagged.map(({ warning, color, badge }) => (
            <Badge key={warning} size="xs" variant="light" color={color}>
              {badge}
            </Badge>
          ))}
        </Group>
      )}
      <LimitLine
        label={t(($) => $.settings.integrations.providerLimit.perSecondShort)}
        value={status.perSecond}
      />
      <LimitLine
        label={t(($) => $.settings.integrations.providerLimit.perDayShort)}
        value={status.perDay}
      />
      <Text size="xs" c="dimmed">
        {t(($) => $.settings.integrations.providerLimit.sentLast24h)}:{' '}
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
          aria-label={t(($) => $.settings.integrations.providerLimit.sentLast24h)}
        />
      )}
    </Stack>
  )
}
