import { Button, Card, Group, Loader, PasswordInput, Stack, Text, Title } from '@mantine/core'
import { useForm } from '@mantine/form'
import { useMutation } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { CenteredLayout } from '../../components/CenteredLayout.tsx'
import { AnchorLink } from '../../components/RouterLink.tsx'
import { siteAuthVerifyEmailMutation } from '../../generated/site/@tanstack/react-query.gen.ts'
import { useNotify } from '../../hooks/useNotify.ts'
import { Route as loginRoute } from '../../routes/login.tsx'
import { Route as verifyEmailRoute } from '../../routes/verify-email.tsx'
import { Route as overviewRoute } from '../../routes/workspaces/$workspaceId/index.tsx'
import { getApiErrorMessage, hasErrorStatus } from '../../utils/apiErrors.ts'

// Where the link in the verification email lands (ADR 0030). The page sends the token
// alone first: in the Sign-up browser, the password from the Sign-up form
// stays and the answer starts the session. Anywhere else the answer is "password
// needed", nothing has changed, and the form here sets the password that goes live.
export function VerifyEmailPage() {
  const { t } = useTranslation()
  const notify = useNotify()
  const { token } = verifyEmailRoute.useSearch()
  const navigate = useNavigate()

  const form = useForm({
    initialValues: { password: '', confirm: '' },
    validate: {
      confirm: (value, values) =>
        value === values.password ? null : t(($) => $.verifyEmail.mismatch),
    },
  })

  // The link was not opened in the Sign-up browser: the form replaces the password.
  const [needsPassword, setNeedsPassword] = useState(false)

  const mutation = useMutation({
    ...siteAuthVerifyEmailMutation(),
    // The session cookie is set by the answer, which also names the Workspace to open: the
    // one the User joined by its domain, else their first.
    onSuccess: async (result) =>
      navigate({
        to: overviewRoute.to,
        params: { workspaceId: result.landingWorkspaceId },
        replace: true,
      }),
    onError: (error) => {
      form.setValues({ password: '', confirm: '' })
      if (hasErrorStatus(error, [428])) {
        setNeedsPassword(true)
        return
      }
      // A refused link is spent, garbage or expired: the page says so instead of the form.
      if (hasErrorStatus(error, [400]) || !needsPassword) {
        return
      }
      notify({
        color: 'red',
        title: t(($) => $.verifyEmail.errorTitle),
        message: getApiErrorMessage(
          error,
          t(($) => $.verifyEmail.errorMessage),
        ),
      })
    },
  })

  const { mutate } = mutation

  // Fire once on mount, with no password; the ref guards against StrictMode's double-invoke.
  const fired = useRef(false)
  useEffect(() => {
    if (fired.current || token === '') {
      return
    }
    fired.current = true
    mutate({ body: { token } })
  }, [mutate, token])

  if (token === '' || (mutation.isError && hasErrorStatus(mutation.error, [400]))) {
    return (
      <CenteredLayout maw={460}>
        <Card withBorder padding="xl">
          <Stack align="center" gap="sm">
            <Title order={3}>{t(($) => $.verifyEmail.errorTitle)}</Title>
            <Text c="dimmed" ta="center">
              {t(($) => $.verifyEmail.errorBody)}
            </Text>
            <AnchorLink to={loginRoute.to} size="sm" mt="md">
              {t(($) => $.verifyEmail.signIn)}
            </AnchorLink>
          </Stack>
        </Card>
      </CenteredLayout>
    )
  }

  if (!needsPassword) {
    return (
      <CenteredLayout maw={460} align="center">
        <Card withBorder w="100%" padding="xl">
          <Stack align="center" gap="sm">
            {mutation.isError ? (
              <>
                <Title order={3}>{t(($) => $.verifyEmail.errorTitle)}</Title>
                <Text c="dimmed" ta="center">
                  {t(($) => $.verifyEmail.errorMessage)}
                </Text>
                <Button onClick={() => mutate({ body: { token } })}>
                  {t(($) => $.verifyEmail.retry)}
                </Button>
                <AnchorLink to={loginRoute.to} size="sm" mt="md">
                  {t(($) => $.verifyEmail.signIn)}
                </AnchorLink>
              </>
            ) : (
              <>
                <Loader />
                <Text c="dimmed">{t(($) => $.verifyEmail.verifying)}</Text>
              </>
            )}
          </Stack>
        </Card>
      </CenteredLayout>
    )
  }

  return (
    <CenteredLayout maw={420}>
      <Title order={3}>{t(($) => $.verifyEmail.title)}</Title>
      <Text c="dimmed">{t(($) => $.verifyEmail.otherBrowserBody)}</Text>

      <form
        onSubmit={form.onSubmit((values) =>
          mutation.mutate({ body: { token, password: values.password } }),
        )}
      >
        <Stack>
          <PasswordInput
            label={t(($) => $.verifyEmail.passwordLabel)}
            autoComplete="new-password"
            required
            {...form.getInputProps('password')}
          />
          <PasswordInput
            label={t(($) => $.verifyEmail.confirmPasswordLabel)}
            autoComplete="new-password"
            required
            {...form.getInputProps('confirm')}
          />
          <Group justify="flex-end">
            <Button type="submit" loading={mutation.isPending}>
              {t(($) => $.verifyEmail.submitButton)}
            </Button>
          </Group>
        </Stack>
      </form>
    </CenteredLayout>
  )
}
