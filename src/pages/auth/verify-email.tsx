import { Button, Card, Group, Loader, PasswordInput, Stack, Text, Title } from '@mantine/core'
import { useForm } from '@mantine/form'
import { useMutation, useSuspenseQuery } from '@tanstack/react-query'
import { Navigate, useNavigate } from '@tanstack/react-router'
import type { ReactNode } from 'react'
import { useErrorBoundary } from 'react-error-boundary'
import { useTranslation } from 'react-i18next'

import { CenteredLayout } from '../../components/CenteredLayout.tsx'
import { QueryBoundary } from '../../components/QueryBoundary.tsx'
import { AnchorLink } from '../../components/RouterLink.tsx'
import { siteAuthVerifyEmailMutation } from '../../generated/site/@tanstack/react-query.gen.ts'
import { siteAuthVerifyEmail } from '../../generated/site/sdk.gen.ts'
import { useNotify } from '../../hooks/useNotify.ts'
import { Route as loginRoute } from '../../routes/login.tsx'
import { Route as verifyEmailRoute } from '../../routes/verify-email.tsx'
import { Route as overviewRoute } from '../../routes/workspaces/$workspaceId/index.tsx'
import { getApiErrorMessage, hasErrorStatus } from '../../utils/apiErrors.ts'

function StatusCard({ children }: { children: ReactNode }) {
  return (
    <CenteredLayout maw={460} align="center">
      <Card withBorder w="100%" padding="xl">
        <Stack align="center" gap="sm">
          {children}
        </Stack>
      </Card>
    </CenteredLayout>
  )
}

// A refused link is spent, garbage or expired: the page says so instead of the form.
function SpentLink() {
  const { t } = useTranslation()
  return (
    <StatusCard>
      <Title order={3}>{t(($) => $.verifyEmail.errorTitle)}</Title>
      <Text c="dimmed" ta="center">
        {t(($) => $.verifyEmail.errorBody)}
      </Text>
      <AnchorLink to={loginRoute.to} size="sm" mt="md">
        {t(($) => $.verifyEmail.signIn)}
      </AnchorLink>
    </StatusCard>
  )
}

// A failure that says nothing about the link: the token is sent again on a retry.
function VerificationFailed() {
  const { t } = useTranslation()
  const { resetBoundary } = useErrorBoundary()
  return (
    <StatusCard>
      <Title order={3}>{t(($) => $.verifyEmail.errorTitle)}</Title>
      <Text c="dimmed" ta="center">
        {t(($) => $.verifyEmail.errorMessage)}
      </Text>
      <Button onClick={resetBoundary}>{t(($) => $.verifyEmail.retry)}</Button>
      <AnchorLink to={loginRoute.to} size="sm" mt="md">
        {t(($) => $.verifyEmail.signIn)}
      </AnchorLink>
    </StatusCard>
  )
}

// The link was not opened in the Sign-up browser: the form sets the password that goes live.
function PasswordForm({ token }: { token: string }) {
  const { t } = useTranslation()
  const notify = useNotify()
  const navigate = useNavigate()

  const form = useForm({
    initialValues: { password: '', confirm: '' },
    validate: {
      confirm: (value, values) =>
        value === values.password ? null : t(($) => $.verifyEmail.mismatch),
    },
  })

  const mutation = useMutation({
    ...siteAuthVerifyEmailMutation(),
    // The session cookie is set by the answer, which also names the Workspace to open.
    onSuccess: async (result) =>
      navigate({
        to: overviewRoute.to,
        params: { workspaceId: result.landingWorkspaceId },
        replace: true,
      }),
    onError: (error) => {
      form.setValues({ password: '', confirm: '' })
      if (hasErrorStatus(error, [400])) {
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

  if (mutation.isError && hasErrorStatus(mutation.error, [400])) {
    return <SpentLink />
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

// What the token alone yields: in the Sign-up browser the password from the Sign-up form stays
// and the answer starts the session; anywhere else the answer is "password needed" (428) and
// nothing has changed; a refused link (400) is spent. The page reads it once: the answer is kept
// for good, so a remount or a refocus never sends the token again, and the request takes no abort
// signal, so one that was sent is not cut off and sent a second time.
function Verification({ token }: { token: string }) {
  const { data } = useSuspenseQuery({
    queryKey: ['siteAuthVerifyEmail', token],
    queryFn: async () => {
      try {
        const { data: verified } = await siteAuthVerifyEmail({
          body: { token },
          throwOnError: true,
        })
        return { kind: 'verified', landingWorkspaceId: verified.landingWorkspaceId } as const
      } catch (error) {
        if (hasErrorStatus(error, [428])) {
          return { kind: 'needsPassword' } as const
        }
        if (hasErrorStatus(error, [400])) {
          return { kind: 'spent' } as const
        }
        throw error
      }
    },
    staleTime: 'static',
    retry: false,
  })

  if (data.kind === 'needsPassword') {
    return <PasswordForm token={token} />
  }
  if (data.kind === 'spent') {
    return <SpentLink />
  }
  return (
    <Navigate to={overviewRoute.to} params={{ workspaceId: data.landingWorkspaceId }} replace />
  )
}

// Where the link in the verification email lands (ADR 0030).
export function VerifyEmailPage() {
  const { t } = useTranslation()
  const { token } = verifyEmailRoute.useSearch()

  if (token === '') {
    return <SpentLink />
  }

  return (
    <QueryBoundary
      fallback={
        <StatusCard>
          <Loader />
          <Text c="dimmed">{t(($) => $.verifyEmail.verifying)}</Text>
        </StatusCard>
      }
      errorTitle={t(($) => $.verifyEmail.errorTitle)}
      renderError={() => <VerificationFailed />}
    >
      <Verification token={token} />
    </QueryBoundary>
  )
}
