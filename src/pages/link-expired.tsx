import { Card, Stack, Text, Title } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import { CenteredLayout } from '../components/CenteredLayout.tsx'

// Public page a short tracked link (/c/{token}) redirects to when it leads nowhere: its message was
// deleted by message retention with the link (ADR 0143), or it never existed. It names neither the
// sender nor the message.
export function LinkExpiredPage() {
  const { t } = useTranslation()

  return (
    <CenteredLayout maw={460} align="center">
      <Card withBorder w="100%" padding="xl">
        <Stack align="center" gap="sm">
          <Title order={3}>{t(($) => $.linkExpired.title)}</Title>
          <Text c="dimmed" ta="center">
            {t(($) => $.linkExpired.body)}
          </Text>
        </Stack>
      </Card>
    </CenteredLayout>
  )
}
