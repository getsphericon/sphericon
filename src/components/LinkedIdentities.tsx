import { Alert, Button, Group, Loader, Stack, Text, Title } from '@mantine/core'
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
import type { SiteIdentitiesLinkData } from '../generated/site/types.gen.ts'
import { zSiteSigninError } from '../generated/site/zod.gen.ts'
import { useResourceMutation } from '../hooks/useResourceMutation.ts'

// The URL that starts linking a provider account to the signed-in User. The browser navigates
// there and the server redirects on to the provider, so the URL is built from the contract.
function linkUrl(provider: string) {
  return client.buildUrl<SiteIdentitiesLinkData>({
    url: '/me/identities/{provider}/link',
    path: { provider },
  })
}

// The account's Linked identities (ADR 0048): the Sign-in providers the User can sign in with,
// a button to unlink each, and one to link another account of each provider the Instance
// enabled (a different email is fine). Nothing is shown on an Instance with no provider and
// an account with no identity.
export function LinkedIdentities() {
  const { t } = useTranslation()
  const search = useSearch({ strict: false })
  const linkError = zSiteSigninError.safeParse(search.error).data
  const identities = useQuery(siteIdentitiesListOptions())
  const providers = useQuery(siteAuthSigninProvidersOptions())
  const unlink = useResourceMutation({
    mutation: siteIdentitiesUnlinkMutation(),
    invalidate: [siteIdentitiesListQueryKey()],
    successMessage: t(($) => $.signin.linked.unlinked),
    errorTitle: t(($) => $.signin.linked.unlinkErrorTitle),
  })

  const items = identities.data?.items ?? []
  const enabled = providers.data?.items ?? []
  if (identities.isSuccess && providers.isSuccess && items.length === 0 && enabled.length === 0) {
    return null
  }

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
      {identities.isLoading ? <Loader /> : null}
      {identities.isSuccess && items.length === 0 ? (
        <Text size="sm" c="dimmed">
          {t(($) => $.signin.linked.empty)}
        </Text>
      ) : null}
      {items.map((identity) => (
        <Group key={identity.id} justify="space-between">
          <Text>{identity.displayName}</Text>
          <Button
            color="red"
            variant="light"
            loading={unlink.isPending && unlink.variables.path.id === identity.id}
            onClick={() => unlink.mutate({ path: { id: identity.id } })}
          >
            {t(($) => $.signin.linked.unlink)}
          </Button>
        </Group>
      ))}
      {enabled.map((provider) => (
        <Button key={provider.name} component="a" href={linkUrl(provider.name)} variant="default">
          {t(($) => $.signin.linked.link, { provider: provider.displayName })}
        </Button>
      ))}
    </Stack>
  )
}
