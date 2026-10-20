import { Button, Divider, Stack } from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { siteAuthSigninProvidersOptions } from '../generated/site/@tanstack/react-query.gen.ts'
import { client } from '../generated/site/client.gen.ts'
import type { SiteAuthSigninStartData } from '../generated/site/types.gen.ts'

// The URL that starts a sign-in with a provider: the browser navigates to it (the server
// redirects to the provider), so it is built from the contract, not fetched.
function signinStartUrl(
  provider: string,
  returnTo: string | undefined,
  invitation: string | undefined,
) {
  return client.buildUrl<SiteAuthSigninStartData>({
    url: '/auth/signin/{provider}/start',
    path: { provider },
    query: {
      ...(returnTo ? { return_to: returnTo } : {}),
      ...(invitation ? { invitation } : {}),
    },
  })
}

// The "Continue with <provider>" buttons of the login and register pages (ADR 0048): one per
// Sign-in provider the Instance enabled, and nothing at all when it enabled none, so the
// pages stay as they were. `returnTo` is the in-app path to land on afterwards; `invitation`
// is the token of an Invitation link, which the sign-in then accepts; `separated` adds the "or"
// rule before a form that follows the buttons.
export function SigninProviders({
  returnTo,
  invitation,
  separated = true,
}: {
  returnTo?: string | undefined
  invitation?: string | undefined
  separated?: boolean
}) {
  const { t } = useTranslation()
  const { data } = useQuery(siteAuthSigninProvidersOptions())

  if (!data || data.items.length === 0) {
    return null
  }

  return (
    <Stack>
      {data.items.map((provider) => (
        <Button
          key={provider.name}
          component="a"
          href={signinStartUrl(provider.name, returnTo, invitation)}
          variant="default"
        >
          {t(($) => $.signin.continueWith, { provider: provider.displayName })}
        </Button>
      ))}
      {separated ? <Divider label={t(($) => $.signin.or)} labelPosition="center" /> : null}
    </Stack>
  )
}
