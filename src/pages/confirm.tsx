import { Alert, Button, Card, Image, Loader, Stack, Text, Title } from '@mantine/core'
import { useMutation, useSuspenseQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { CenteredLayout } from '../components/CenteredLayout.tsx'
import { QueryBoundary } from '../components/QueryBoundary.tsx'
import {
  sitePublicConfirmationsPerformMutation,
  sitePublicConfirmationsReadOptions,
  sitePublicConfirmationsRenewMutation,
} from '../generated/site/@tanstack/react-query.gen.ts'
import type { SitePublicBrand } from '../generated/site/types.gen.ts'
import { Route as confirmRoute } from '../routes/confirm.tsx'
import { hasErrorStatus, isRateLimitedError } from '../utils/apiErrors.ts'

// Public double opt-in confirmation page (ADR 0013). The GET /e/confirm/{token} endpoint redirects
// here and records nothing — the confirmation happens only when the person presses Confirm, which
// performs it through the site API. This keeps GET safe so email link scanners can't confirm anyone
// (a deliberate human act is required for legal validity). The page reads what its link shows: the
// Workspace's name and its Brand kit as it stands, and whether the link has expired; an expired
// link offers a new one, a request the person makes.
export function ConfirmSubscriptionPage() {
  const { token } = confirmRoute.useSearch()
  return <ConfirmSubscription token={token} />
}

export function ConfirmSubscription({ token }: { token?: string | undefined }) {
  const { t } = useTranslation()
  return (
    <CenteredLayout maw={460} align="center">
      <Card withBorder w="100%" padding="xl">
        {token ? (
          <QueryBoundary
            fallback={
              <Stack align="center">
                <Loader />
              </Stack>
            }
            errorTitle={t(($) => $.confirmSubscription.errorTitle)}
            // A token the server refuses is no link of ours; any other failure keeps the alert.
            renderError={(error) =>
              hasErrorStatus(error, [400, 404]) ? <InvalidLink /> : undefined
            }
          >
            <ConfirmLink token={token} />
          </QueryBoundary>
        ) : (
          <InvalidLink />
        )}
      </Card>
    </CenteredLayout>
  )
}

function InvalidLink() {
  const { t } = useTranslation()
  return (
    <Stack align="center" gap="sm">
      <Title order={3}>{t(($) => $.confirmSubscription.invalidTitle)}</Title>
      <Text c="dimmed" ta="center">
        {t(($) => $.confirmSubscription.invalidBody)}
      </Text>
    </Stack>
  )
}

function ConfirmLink({ token }: { token: string }) {
  const { t } = useTranslation()
  const page = useSuspenseQuery(sitePublicConfirmationsReadOptions({ path: { token } }))
  const perform = useMutation(sitePublicConfirmationsPerformMutation())

  const { brand, workspaceName } = page.data
  // Expired before the page was opened, or while it sat open (410).
  if (page.data.expired || hasErrorStatus(perform.error, [410])) {
    return <ExpiredLink token={token} brand={brand} workspaceName={workspaceName} />
  }
  if (perform.isSuccess) {
    return (
      <Branded brand={brand}>
        <Title order={3}>{t(($) => $.confirmSubscription.doneTitle)}</Title>
        <Text ta="center">
          {t(($) => $.confirmSubscription.doneBody, { workspace: workspaceName })}
        </Text>
      </Branded>
    )
  }
  return (
    <Branded brand={brand}>
      <Title order={3}>{t(($) => $.confirmSubscription.title)}</Title>
      <Text ta="center">{t(($) => $.confirmSubscription.body, { workspace: workspaceName })}</Text>
      {perform.isError ? (
        <Alert color="red" title={t(($) => $.confirmSubscription.errorTitle)} w="100%">
          {isRateLimitedError(perform.error)
            ? t(($) => $.notifications.rateLimited)
            : t(($) => $.confirmSubscription.errorBody)}
        </Alert>
      ) : null}
      <BrandButton
        brand={brand}
        loading={perform.isPending}
        onClick={() => perform.mutate({ path: { token } })}
      >
        {t(($) => $.confirmSubscription.confirm)}
      </BrandButton>
    </Branded>
  )
}

function ExpiredLink({
  token,
  brand,
  workspaceName,
}: {
  token: string
  brand: SitePublicBrand
  workspaceName: string
}) {
  const { t } = useTranslation()
  const renew = useMutation(sitePublicConfirmationsRenewMutation())

  if (renew.isSuccess) {
    return (
      <Branded brand={brand}>
        <Title order={3}>
          {renew.data.status === 'confirmed'
            ? t(($) => $.confirmSubscription.alreadyConfirmedTitle)
            : t(($) => $.confirmSubscription.renewedTitle)}
        </Title>
        <Text ta="center">
          {renew.data.status === 'confirmed'
            ? t(($) => $.confirmSubscription.alreadyConfirmedBody, { workspace: workspaceName })
            : t(($) => $.confirmSubscription.renewedBody)}
        </Text>
      </Branded>
    )
  }
  return (
    <Branded brand={brand}>
      <Title order={3}>{t(($) => $.confirmSubscription.expiredTitle)}</Title>
      <Text ta="center">
        {t(($) => $.confirmSubscription.expiredBody, { workspace: workspaceName })}
      </Text>
      {renew.isError ? (
        <Alert color="red" title={t(($) => $.confirmSubscription.errorTitle)} w="100%">
          {isRateLimitedError(renew.error)
            ? t(($) => $.notifications.rateLimited)
            : t(($) => $.confirmSubscription.errorBody)}
        </Alert>
      ) : null}
      <BrandButton
        brand={brand}
        loading={renew.isPending}
        onClick={() => renew.mutate({ path: { token } })}
      >
        {t(($) => $.confirmSubscription.renew)}
      </BrandButton>
    </Branded>
  )
}

// Branded is the page's content in the Workspace's Brand kit: its logo, its text color and its
// font.
function Branded({ brand, children }: { brand: SitePublicBrand; children: React.ReactNode }) {
  return (
    <Stack align="center" gap="md" c={brand.textColor} ff={brand.bodyFont}>
      {brand.logoUrl ? <Image src={brand.logoUrl} alt="" h={40} w="auto" fit="contain" /> : null}
      {children}
    </Stack>
  )
}

function BrandButton({
  brand,
  loading,
  onClick,
  children,
}: {
  brand: SitePublicBrand
  loading: boolean
  onClick: () => void
  children: React.ReactNode
}) {
  return (
    <Button
      color={brand.accentColor}
      c={brand.buttonTextColor}
      radius={brand.buttonRadius}
      loading={loading}
      onClick={onClick}
    >
      {children}
    </Button>
  )
}
