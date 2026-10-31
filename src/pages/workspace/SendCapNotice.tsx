import { Alert, NumberFormatter, Progress, Stack, Text } from '@mantine/core'
import { IconGauge } from '@tabler/icons-react'
import { useTranslation } from 'react-i18next'

import type { SiteSendCap } from '../../generated/site/types.gen.ts'

// The Send cap the platform set on the Workspace (ADR 0067), read-only, with the last 24 hours
// of sends against it. It is the Workspace's ceiling across every provider, set apart from each
// Integration's own Provider limit.
export function SendCapNotice({ sendCap }: { sendCap: SiteSendCap }) {
  const { t } = useTranslation()
  return (
    <Alert
      variant="light"
      color="gray"
      icon={<IconGauge size={18} />}
      title={
        <>
          {t(($) => $.settings.sendCap.title)}:{' '}
          <NumberFormatter value={sendCap.perDay} thousandSeparator=" " />{' '}
          {t(($) => $.settings.sendCap.perDay)}
        </>
      }
    >
      <Stack gap={6}>
        <Text size="sm">{t(($) => $.settings.sendCap.description)}</Text>
        <Text size="xs" c="dimmed">
          {t(($) => $.settings.sendCap.sentLast24h)}:{' '}
          <NumberFormatter value={sendCap.sentLast24h} thousandSeparator=" " />
          {' / '}
          <NumberFormatter value={sendCap.perDay} thousandSeparator=" " />
        </Text>
        <Progress
          size="xs"
          value={Math.min(100, (sendCap.sentLast24h / sendCap.perDay) * 100)}
          aria-label={t(($) => $.settings.sendCap.sentLast24h)}
        />
      </Stack>
    </Alert>
  )
}
