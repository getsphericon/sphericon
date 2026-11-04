import { Alert, Anchor, Button, Card, Stack, Text, Title } from '@mantine/core'
import { useMutation } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { CenteredLayout } from '../components/CenteredLayout.tsx'
import { sitePublicUnsubscribesPerformMutation } from '../generated/site/@tanstack/react-query.gen.ts'
import type { SiteIntegrationChannel } from '../generated/site/types.gen.ts'
import { useUnsubscribeCopy } from '../hooks/useUnsubscribeCopy.ts'
import { Route as unsubscribeRoute } from '../routes/unsubscribe.tsx'

// Public unsubscribe confirmation page (ADR 0012 / RFC 8058). The GET
// /e/u/{token} endpoint redirects here and records nothing — the opt-out happens
// only when the user presses Confirm, which performs it through the site API. This
// keeps GET safe so email link scanners can't unsubscribe anyone. The mailbox
// provider's one-click POST keeps using POST /e/u/{token}.
export function UnsubscribePage() {
  const { token, all, channel } = unsubscribeRoute.useSearch()
  return <Unsubscribe token={token} all={all} channel={channel} />
}

export function Unsubscribe({
  token,
  all,
  channel,
}: {
  token: string
  all?: string | undefined
  channel?: SiteIntegrationChannel | undefined
}) {
  const { t } = useTranslation()
  const copy = useUnsubscribeCopy(channel)
  const mutation = useMutation(sitePublicUnsubscribesPerformMutation())

  const confirm = () => mutation.mutate({ path: { token } })

  return (
    <CenteredLayout maw={460} align="center">
      <Card withBorder w="100%" padding="xl">
        {mutation.isSuccess ? (
          <Stack align="center" gap="sm">
            <Title order={3}>{t(($) => $.unsubscribed.title)}</Title>
            <Text c="dimmed" ta="center">
              {copy.done}
            </Text>
            {all ? (
              <Anchor href={all} size="sm" mt="md">
                {copy.all}
              </Anchor>
            ) : null}
          </Stack>
        ) : (
          <Stack align="center" gap="md">
            <Title order={3}>{t(($) => $.unsubscribe.title)}</Title>
            <Text c="dimmed" ta="center">
              {copy.ask}
            </Text>
            {mutation.isError ? (
              <Alert color="red" title={t(($) => $.unsubscribe.errorTitle)} w="100%">
                {t(($) => $.unsubscribe.errorBody)}
              </Alert>
            ) : null}
            <Button onClick={confirm} loading={mutation.isPending} color="red">
              {t(($) => $.unsubscribe.confirm)}
            </Button>
          </Stack>
        )}
      </Card>
    </CenteredLayout>
  )
}
