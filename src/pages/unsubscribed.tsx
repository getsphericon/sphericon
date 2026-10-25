import { Anchor, Card, Stack, Text, Title } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import { CenteredLayout } from '../components/CenteredLayout.tsx'
import { Route as unsubscribedRoute } from '../routes/unsubscribed.tsx'

// Public confirmation page the unsubscribe endpoint (/e/u/{token}) redirects to
// after recording the opt-out. The backend passes the deliberate "unsubscribe
// from everything" escalation as the `all` search param (a full URL it built), so
// nothing here constructs a tracking path.
export function UnsubscribedPage() {
  const { t } = useTranslation()
  const { all } = unsubscribedRoute.useSearch()

  return (
    <CenteredLayout maw={460} align="center">
      <Card withBorder w="100%" padding="xl">
        <Stack align="center" gap="sm">
          <Title order={3}>{t(($) => $.unsubscribed.title)}</Title>
          <Text c="dimmed" ta="center">
            {t(($) => $.unsubscribed.body)}
          </Text>
          {all ? (
            <Anchor href={all} size="sm" mt="md">
              {t(($) => $.unsubscribed.unsubscribeAll)}
            </Anchor>
          ) : null}
        </Stack>
      </Card>
    </CenteredLayout>
  )
}
