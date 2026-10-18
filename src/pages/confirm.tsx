import { Alert, Button, Card, Stack, Text, Title } from '@mantine/core'
import { useMutation } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { sitePublicConfirmationsPerformMutation } from '../generated/site/@tanstack/react-query.gen.ts'
import { Route as confirmRoute } from '../routes/confirm.tsx'
import { isRateLimitedError } from '../utils/apiErrors.ts'

// Public double opt-in confirmation page (ADR 0013). The GET /e/confirm/{token}
// endpoint redirects here and records nothing — the confirmation happens only when
// the user presses Confirm, which performs it through the site API. This keeps GET
// safe so email link scanners can't confirm anyone (a deliberate human act is
// required for legal validity). An expired/invalid token arrives here without a
// `token` (with `expired=1`), so the page offers "sign up again" instead of a dead
// button.
export function ConfirmSubscriptionPage() {
  const { token, expired } = confirmRoute.useSearch()
  return <ConfirmSubscription token={token} expired={expired === '1'} />
}

export function ConfirmSubscription({
  token,
  expired,
}: {
  token?: string | undefined
  expired?: boolean
}) {
  const { t } = useTranslation()
  const mutation = useMutation(sitePublicConfirmationsPerformMutation())

  const confirm = () => {
    if (token) mutation.mutate({ path: { token } })
  }

  // Expired before the page was opened (no token / expired=1) or while it sat open (410).
  const isExpired = expired || !token || mutation.error?.status === 410

  return (
    <Stack maw={460} mx="auto" mt="xl" align="center">
      <Card withBorder w="100%" padding="xl">
        {isExpired ? (
          <Stack align="center" gap="sm">
            <Title order={3}>{t(($) => $.confirmSubscription.expiredTitle)}</Title>
            <Text c="dimmed" ta="center">
              {t(($) => $.confirmSubscription.expiredBody)}
            </Text>
          </Stack>
        ) : mutation.isSuccess ? (
          <Stack align="center" gap="sm">
            <Title order={3}>{t(($) => $.confirmSubscription.doneTitle)}</Title>
            <Text c="dimmed" ta="center">
              {t(($) => $.confirmSubscription.doneBody)}
            </Text>
          </Stack>
        ) : (
          <Stack align="center" gap="md">
            <Title order={3}>{t(($) => $.confirmSubscription.title)}</Title>
            <Text c="dimmed" ta="center">
              {t(($) => $.confirmSubscription.body)}
            </Text>
            {mutation.isError ? (
              <Alert color="red" title={t(($) => $.confirmSubscription.errorTitle)} w="100%">
                {isRateLimitedError(mutation.error)
                  ? t(($) => $.notifications.rateLimited)
                  : t(($) => $.confirmSubscription.errorBody)}
              </Alert>
            ) : null}
            <Button onClick={confirm} loading={mutation.isPending}>
              {t(($) => $.confirmSubscription.confirm)}
            </Button>
          </Stack>
        )}
      </Card>
    </Stack>
  )
}
