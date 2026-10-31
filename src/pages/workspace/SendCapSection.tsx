import { Badge, Box, Group, Progress, Stack, Text, Title } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import type { SiteSendCap } from '../../generated/site/types.gen.ts'

// The Workspace's Send cap (ADR 0067): the platform's limit on its sends per rolling 24 hours across
// every provider, which the Workspace only reads, and how much of it the last 24 hours used. It is
// its own section, apart from each provider's Provider limit, the limit the Workspace sets. Nothing
// renders when no Send cap applies.
export function SendCapSection({ sendCap }: { sendCap: SiteSendCap | null | undefined }) {
  const { t } = useTranslation()
  if (!sendCap) {
    return null
  }
  return (
    <Box>
      <Title order={4} mb="sm">
        {t(($) => $.settings.sendCap.title)}
      </Title>
      <Stack gap={4}>
        <Group gap="xs" wrap="nowrap">
          <Text size="sm">{t(($) => $.settings.sendCap.perDay, { perDay: sendCap.perDay })}</Text>
          <Badge size="xs" variant="light" color="gray">
            {t(($) => $.settings.sendCap.setByPlatform)}
          </Badge>
        </Group>
        <Text size="xs" c="dimmed">
          {t(($) => $.settings.sendCap.sentOf, {
            sent: sendCap.sentLast24h,
            perDay: sendCap.perDay,
          })}
        </Text>
        <Progress
          size="xs"
          value={Math.min(100, (sendCap.sentLast24h / sendCap.perDay) * 100)}
          aria-label={t(($) => $.settings.sendCap.sentLast24h)}
        />
        <Text size="sm" c="dimmed">
          {t(($) => $.settings.sendCap.description)}
        </Text>
      </Stack>
    </Box>
  )
}
