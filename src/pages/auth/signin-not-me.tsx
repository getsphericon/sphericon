import { Card, Loader, Stack, Text, Title } from '@mantine/core'
import { useMutation } from '@tanstack/react-query'
import { useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'

import { AnchorLink } from '../../components/RouterLink.tsx'
import { siteAuthSigninNotMeMutation } from '../../generated/site/@tanstack/react-query.gen.ts'
import { Route as loginRoute } from '../../routes/login.tsx'
import { Route as signinNotMeRoute } from '../../routes/signin-not-me.tsx'

export function SigninNotMePage() {
  const { t } = useTranslation()
  const { token } = signinNotMeRoute.useSearch()
  const mutation = useMutation(siteAuthSigninNotMeMutation())

  const { mutate } = mutation

  // Fire once on mount; the ref guards against StrictMode's double-invoke.
  const fired = useRef(false)
  useEffect(() => {
    if (fired.current) {
      return
    }
    fired.current = true
    mutate({ body: { token } })
  }, [mutate, token])

  return (
    <Stack maw={460} mx="auto" mt="xl" align="center">
      <Card withBorder w="100%" padding="xl">
        <Stack align="center" gap="sm">
          {mutation.isSuccess ? (
            <>
              <Title order={3}>{t(($) => $.signinNotMe.successTitle)}</Title>
              <Text c="dimmed" ta="center">
                {t(($) => $.signinNotMe.successBody)}
              </Text>
              <AnchorLink to={loginRoute.to} size="sm" mt="md">
                {t(($) => $.signinNotMe.goToLogin)}
              </AnchorLink>
            </>
          ) : mutation.isError ? (
            <>
              <Title order={3}>{t(($) => $.signinNotMe.errorTitle)}</Title>
              <Text c="dimmed" ta="center">
                {t(($) => $.signinNotMe.errorBody)}
              </Text>
            </>
          ) : (
            <>
              <Loader />
              <Text c="dimmed">{t(($) => $.signinNotMe.working)}</Text>
            </>
          )}
        </Stack>
      </Card>
    </Stack>
  )
}
