import { Button, Card, Group, Loader, Stack, Text, Title } from '@mantine/core'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { AuthLayout } from '../../components/AuthLayout.tsx'
import { AnchorLink } from '../../components/RouterLink.tsx'
import { SignupItemFields } from '../../components/SignupItemFields.tsx'
import {
  siteAuthSigninPendingDeclineMutation,
  siteAuthSigninPendingOptions,
  siteAuthSigninPendingSubmitMutation,
  siteLegalDocumentsSignupItemsOptions,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import { useApiErrorMessage } from '../../hooks/useApiErrorMessage.ts'
import { useNotify } from '../../hooks/useNotify.ts'
import { Route as indexRoute } from '../../routes/index.tsx'
import { Route as signUpRoute } from '../../routes/sign-up.tsx'
import { Route as signinPendingRoute } from '../../routes/signin-pending.tsx'
import { hasErrorStatus } from '../../utils/apiErrors.ts'
import { signupItemAnswers } from '../../utils/signupItems.ts'

// A pending sign-up answers 400 once it expired, was completed or declined.
function isGone(error: unknown) {
  return hasErrorStatus(error, [400])
}

// The screen a Sign-in provider's sign-up lands on while the Instance asks Sign-up items (ADR
// 0062): no account exists until the person answers them; declining creates nothing.
export function SigninPendingPage() {
  const { t } = useTranslation()
  const notify = useNotify()
  const apiErrorMessage = useApiErrorMessage()
  const navigate = useNavigate()
  const { token, redirect } = signinPendingRoute.useSearch()
  const pendingQuery = useQuery({
    ...siteAuthSigninPendingOptions({ query: { token } }),
    retry: false,
  })
  const itemsQuery = useQuery(siteLegalDocumentsSignupItemsOptions())
  const items = itemsQuery.data?.items ?? []
  const [ticked, setTicked] = useState<Record<string, boolean>>({})

  const submitMutation = useMutation({
    ...siteAuthSigninPendingSubmitMutation(),
    onSuccess: async () => navigate(redirect ? { href: redirect } : { to: indexRoute.to }),
    onError: (error) => {
      if (isGone(error)) {
        return
      }
      notify({
        color: 'red',
        title: t(($) => $.signinPending.errorTitle),
        message: apiErrorMessage(
          error,
          t(($) => $.signinPending.errorMessage),
        ),
      })
    },
  })
  const declineMutation = useMutation({
    ...siteAuthSigninPendingDeclineMutation(),
    onSuccess: async () => navigate({ to: signUpRoute.to }),
  })

  if (isGone(pendingQuery.error) || isGone(submitMutation.error)) {
    return (
      <AuthLayout maw={460}>
        <Card withBorder padding="xl">
          <Stack align="center" gap="sm">
            <Title order={3}>{t(($) => $.signinPending.goneTitle)}</Title>
            <Text c="dimmed" ta="center">
              {t(($) => $.signinPending.goneBody)}
            </Text>
            <AnchorLink to={signUpRoute.to} size="sm" mt="md">
              {t(($) => $.signinPending.startAgain)}
            </AnchorLink>
          </Stack>
        </Card>
      </AuthLayout>
    )
  }

  const pending = pendingQuery.data
  if (!pending) {
    return (
      <AuthLayout maw={460}>
        <Loader />
      </AuthLayout>
    )
  }

  const handleSubmit = () => {
    const signupItems = signupItemAnswers(items, ticked)
    submitMutation.mutate({ body: { token, ...(signupItems && { signupItems }) } })
  }

  return (
    <AuthLayout>
      <Title order={3}>{t(($) => $.signinPending.title)}</Title>
      <Text c="dimmed">
        {t(($) => $.signinPending.body, { email: pending.email, provider: pending.provider })}
      </Text>
      <Stack mt="md">
        <SignupItemFields
          items={items}
          ticked={ticked}
          onTick={(key, checked) => setTicked((current) => ({ ...current, [key]: checked }))}
          error={submitMutation.error}
        />
        <Group justify="space-between" align="center">
          <Button
            variant="default"
            loading={declineMutation.isPending}
            onClick={() => declineMutation.mutate({ body: { token } })}
          >
            {t(($) => $.signinPending.declineButton)}
          </Button>
          <Button loading={submitMutation.isPending} onClick={handleSubmit}>
            {t(($) => $.signinPending.submitButton)}
          </Button>
        </Group>
      </Stack>
    </AuthLayout>
  )
}
