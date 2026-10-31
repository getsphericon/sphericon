import { Card, Loader, Stack, Text, Title } from '@mantine/core'
import { useSuspenseQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { CenteredLayout } from '../../components/CenteredLayout.tsx'
import { QueryBoundary } from '../../components/QueryBoundary.tsx'
import { AnchorLink } from '../../components/RouterLink.tsx'
import { siteAuthConfirmEmailChange } from '../../generated/site/sdk.gen.ts'
import { Route as confirmEmailChangeRoute } from '../../routes/confirm-email-change.tsx'
import { Route as loginRoute } from '../../routes/login.tsx'
import { hasErrorStatus } from '../../utils/apiErrors.ts'

// A refused link (400) is spent, garbage or expired, an answer rather than a fault; a failure of
// the request itself ends on the same words.
function Refused() {
  const { t } = useTranslation()
  return (
    <>
      <Title order={3}>{t(($) => $.confirmEmailChange.errorTitle)}</Title>
      <Text c="dimmed" ta="center">
        {t(($) => $.confirmEmailChange.errorBody)}
      </Text>
    </>
  )
}

// Opening the link is the confirmation: the page reads what its token yields, once. The answer
// is kept for good (the token is spent by it), so a remount or a refocus never sends it again; the
// request takes no abort signal, so one that was sent is not cut off and sent a second time.
function Confirmed({ token }: { token: string }) {
  const { t } = useTranslation()
  const { data: accepted } = useSuspenseQuery({
    queryKey: ['siteAuthConfirmEmailChange', token],
    queryFn: async () => {
      try {
        await siteAuthConfirmEmailChange({ body: { token }, throwOnError: true })
        return true
      } catch (error) {
        if (hasErrorStatus(error, [400])) {
          return false
        }
        throw error
      }
    },
    staleTime: 'static',
    retry: false,
  })

  if (!accepted) {
    return <Refused />
  }

  return (
    <>
      <Title order={3}>{t(($) => $.confirmEmailChange.successTitle)}</Title>
      <Text c="dimmed" ta="center">
        {t(($) => $.confirmEmailChange.successBody)}
      </Text>
      <AnchorLink to={loginRoute.to} size="sm" mt="md">
        {t(($) => $.confirmEmailChange.goToLogin)}
      </AnchorLink>
    </>
  )
}

export function ConfirmEmailChangePage() {
  const { t } = useTranslation()
  const { token } = confirmEmailChangeRoute.useSearch()

  return (
    <CenteredLayout maw={460} align="center">
      <Card withBorder w="100%" padding="xl">
        <Stack align="center" gap="sm">
          <QueryBoundary
            fallback={
              <>
                <Loader />
                <Text c="dimmed">{t(($) => $.confirmEmailChange.verifying)}</Text>
              </>
            }
            errorTitle={t(($) => $.confirmEmailChange.errorTitle)}
            renderError={() => <Refused />}
          >
            <Confirmed token={token} />
          </QueryBoundary>
        </Stack>
      </Card>
    </CenteredLayout>
  )
}
