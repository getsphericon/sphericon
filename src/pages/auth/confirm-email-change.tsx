import { Anchor, Card, Loader, Stack, Text, Title } from '@mantine/core'
import { useMutation } from '@tanstack/react-query'
import { useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'

import { siteAuthConfirmEmailChangeMutation } from '../../generated/site/@tanstack/react-query.gen.ts'
import { Route as confirmEmailChangeRoute } from '../../routes/confirm-email-change.tsx'
import { Route as loginRoute } from '../../routes/login.tsx'

export function ConfirmEmailChangePage() {
  const { t } = useTranslation()
  const { token } = confirmEmailChangeRoute.useSearch()
  const mutation = useMutation(siteAuthConfirmEmailChangeMutation())

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
              <Title order={3}>{t(($) => $.confirmEmailChange.successTitle)}</Title>
              <Text c="dimmed" ta="center">
                {t(($) => $.confirmEmailChange.successBody)}
              </Text>
              <Anchor component="a" href={loginRoute.to} size="sm" mt="md">
                {t(($) => $.confirmEmailChange.goToLogin)}
              </Anchor>
            </>
          ) : mutation.isError ? (
            <>
              <Title order={3}>{t(($) => $.confirmEmailChange.errorTitle)}</Title>
              <Text c="dimmed" ta="center">
                {t(($) => $.confirmEmailChange.errorBody)}
              </Text>
            </>
          ) : (
            <>
              <Loader />
              <Text c="dimmed">{t(($) => $.confirmEmailChange.verifying)}</Text>
            </>
          )}
        </Stack>
      </Card>
    </Stack>
  )
}
