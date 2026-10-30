import { Alert, Button, Group, Stack, Text, Title } from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { useSearch } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import {
  siteAuthSigninProvidersOptions,
  siteIdentitiesListOptions,
  siteIdentitiesListQueryKey,
  siteIdentitiesUnlinkMutation,
} from '../generated/site/@tanstack/react-query.gen.ts'
import { client } from '../generated/site/client.gen.ts'
import type { SiteIdentitiesLinkData, SiteLinkedIdentity } from '../generated/site/types.gen.ts'
import { zSiteSigninError } from '../generated/site/zod.gen.ts'
import { useResourceMutation } from '../hooks/useResourceMutation.ts'
import { BlockSkeleton } from './Skeletons.tsx'

// The URL that starts linking a provider account to the signed-in User. The browser navigates
// there and the server redirects on to the provider, so the URL is built from the contract.
function linkUrl(provider: string) {
  return client.buildUrl<SiteIdentitiesLinkData>({
    url: '/me/identities/{provider}/link',
    path: { provider },
  })
}

// The account's Linked identities (ADR 0048): one row per Sign-in provider the Instance enabled,
// with Connect while the User has no account of it linked and Disconnect once they have, and a
// Disconnect row for an identity whose provider the Instance no longer offers. Nothing is shown
// on an Instance with no provider and an account with no identity.
export function LinkedIdentities() {
  const { t } = useTranslation()
  const search = useSearch({ strict: false })
  const linkError = zSiteSigninError.safeParse(search.error).data
  const identities = useQuery(siteIdentitiesListOptions())
  const providers = useQuery(siteAuthSigninProvidersOptions())
  const unlink = useResourceMutation({
    mutation: siteIdentitiesUnlinkMutation(),
    invalidate: [siteIdentitiesListQueryKey()],
    successMessage: t(($) => $.signin.linked.disconnected),
    errorTitle: t(($) => $.signin.linked.disconnectErrorTitle),
  })

  const items = identities.data?.items ?? []
  const enabled = providers.data?.items ?? []
  if (identities.isSuccess && providers.isSuccess && items.length === 0 && enabled.length === 0) {
    return null
  }

  const disconnect = (identity: SiteLinkedIdentity) => (
    <Group key={identity.id} justify="space-between">
      <Text>{identity.displayName}</Text>
      <Button
        color="red"
        variant="light"
        loading={unlink.isPending && unlink.variables.path.id === identity.id}
        onClick={() => unlink.mutate({ path: { id: identity.id } })}
      >
        {t(($) => $.signin.linked.disconnect)}
      </Button>
    </Group>
  )

  return (
    <Stack>
      <Title order={4}>{t(($) => $.signin.linked.title)}</Title>
      {linkError ? (
        <Alert color="red" title={t(($) => $.signin.linked.errorTitle)}>
          {t(($) => $.signin.errors[linkError])}
        </Alert>
      ) : null}
      {identities.isError ? (
        <Alert color="red" title={t(($) => $.security.loadErrorTitle)} />
      ) : null}
      {identities.isLoading ? <BlockSkeleton /> : null}
      {identities.isSuccess
        ? enabled.map((provider) => {
            const linked = items.filter((identity) => identity.provider === provider.name)
            return linked.length > 0 ? (
              linked.map((identity) => disconnect(identity))
            ) : (
              <Group key={provider.name} justify="space-between">
                <Text>{provider.displayName}</Text>
                <Button component="a" href={linkUrl(provider.name)} variant="default">
                  {t(($) => $.signin.linked.connect)}
                </Button>
              </Group>
            )
          })
        : null}
      {items
        .filter((identity) => !enabled.some((provider) => provider.name === identity.provider))
        .map((identity) => disconnect(identity))}
    </Stack>
  )
}
