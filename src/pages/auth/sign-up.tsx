import { Button, Card, Group, PasswordInput, Stack, Text, TextInput, Title } from '@mantine/core'
import { useForm } from '@mantine/form'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useCountDown } from 'ahooks'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { AuthLayout } from '../../components/AuthLayout.tsx'
import { AnchorLink } from '../../components/RouterLink.tsx'
import { SigninProviders } from '../../components/SigninProviders.tsx'
import { SignupItemFields } from '../../components/SignupItemFields.tsx'
import {
  siteAuthSignUpMutation,
  siteAuthResendVerificationMutation,
  siteLegalDocumentsSignupItemsOptions,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import type { SiteSignUpInput } from '../../generated/site/types.gen.ts'
import { useApiErrorMessage } from '../../hooks/useApiErrorMessage.ts'
import { useNotify } from '../../hooks/useNotify.ts'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'
import { requiresEmailVerification } from '../../instance.ts'
import { Route as loginRoute } from '../../routes/login.tsx'
import { signupItemAnswers } from '../../utils/signupItems.ts'

// Seconds the resend button waits after a send; the server's per-IP limit stays the real gate.
const RESEND_COOLDOWN_SECONDS = 60

export function SignUpPage() {
  const { t } = useTranslation()
  const notify = useNotify()
  const apiErrorMessage = useApiErrorMessage()
  // With the gate off (ADR 0030) the account is usable at once; with it on, the link
  // to the address proves it, and signs in when followed in this browser.
  const usableAtOnce = !requiresEmailVerification()
  // The address the verification link went to; set once the Sign-up is accepted.
  const [sentTo, setSentTo] = useState<string | null>(null)
  const [created, setCreated] = useState(false)
  // The Sign-up items in effect (ADR 0062) and the boxes the person ticked: none at first.
  const itemsQuery = useQuery(siteLegalDocumentsSignupItemsOptions())
  const items = itemsQuery.data?.items ?? []
  const [ticked, setTicked] = useState<Record<string, boolean>>({})

  // The moment the resend button is offered again; none until a link is resent.
  const [resendAt, setResendAt] = useState<number>()
  const [cooldownLeft] = useCountDown({ targetDate: resendAt })
  const cooldown = Math.ceil(cooldownLeft / 1000)
  const resendMutation = useResourceMutation({
    mutation: siteAuthResendVerificationMutation(),
    successMessage: t(($) => $.login.resendSuccess),
    errorTitle: t(($) => $.login.resendErrorTitle),
    onDone: () => setResendAt(Date.now() + RESEND_COOLDOWN_SECONDS * 1000),
  })

  const form = useForm<Required<Omit<SiteSignUpInput, 'signupItems'>> & { confirm: string }>({
    initialValues: {
      name: '',
      email: '',
      password: '',
      confirm: '',
    },
    validate: {
      confirm: (value, values) => (value === values.password ? null : t(($) => $.signUp.mismatch)),
    },
  })

  const signUpMutation = useMutation({
    ...siteAuthSignUpMutation(),
    onSuccess: (_data, variables) => {
      if (usableAtOnce) {
        setCreated(true)
      } else {
        setSentTo(variables.body.email)
      }
    },
    onError: (error) => {
      notify({
        color: 'red',
        title: t(($) => $.signUp.errorTitle),
        message: apiErrorMessage(
          error,
          t(($) => $.signUp.errorMessage),
        ),
      })
    },
  })

  const handleSubmit = (values: Required<Omit<SiteSignUpInput, 'signupItems'>>) => {
    // With no items the form sends no answers at all, as it did before items existed.
    const signupItems = signupItemAnswers(items, ticked)
    signUpMutation.mutate({
      body: {
        name: values.name.trim(),
        email: values.email.trim(),
        password: values.password,
        ...(signupItems && { signupItems }),
      },
    })
  }

  if (created) {
    return (
      <AuthLayout maw={460}>
        <Card withBorder padding="xl">
          <Stack align="center" gap="sm">
            <Title order={3}>{t(($) => $.signUp.createdTitle)}</Title>
            <Text c="dimmed" ta="center">
              {t(($) => $.signUp.createdBody)}
            </Text>
            <AnchorLink to={loginRoute.to} size="sm" mt="md">
              {t(($) => $.signUp.signInLink)}
            </AnchorLink>
          </Stack>
        </Card>
      </AuthLayout>
    )
  }

  if (sentTo) {
    return (
      <AuthLayout maw={460}>
        <Card withBorder padding="xl">
          <Stack align="center" gap="sm">
            <Title order={3}>{t(($) => $.signUp.checkEmailTitle)}</Title>
            <Text c="dimmed" ta="center">
              {t(($) => $.signUp.checkEmailBody, { email: sentTo })}
            </Text>
            <Text size="sm" mt="md">
              {t(($) => $.signUp.didntReceive)}
            </Text>
            <Button
              variant="default"
              loading={resendMutation.isPending}
              disabled={cooldown > 0}
              onClick={() => resendMutation.mutate({ body: { email: sentTo } })}
            >
              {cooldown > 0
                ? t(($) => $.signUp.resendIn, { seconds: cooldown })
                : t(($) => $.login.resendVerification)}
            </Button>
            <AnchorLink to={loginRoute.to} size="sm" mt="md">
              {t(($) => $.signUp.loginLink)}
            </AnchorLink>
          </Stack>
        </Card>
      </AuthLayout>
    )
  }

  return (
    <AuthLayout>
      <Title order={3}>{t(($) => $.signUp.title)}</Title>

      <form onSubmit={form.onSubmit(handleSubmit)}>
        <Stack>
          <TextInput
            label={t(($) => $.signUp.nameLabel)}
            required
            {...form.getInputProps('name')}
          />
          <TextInput
            label={t(($) => $.signUp.emailLabel)}
            type="email"
            required
            {...form.getInputProps('email')}
          />
          <PasswordInput
            label={t(($) => $.signUp.passwordLabel)}
            autoComplete="new-password"
            required
            {...form.getInputProps('password')}
          />
          <PasswordInput
            label={t(($) => $.signUp.confirmPasswordLabel)}
            autoComplete="new-password"
            required
            {...form.getInputProps('confirm')}
          />

          <SignupItemFields
            items={items}
            ticked={ticked}
            onTick={(key, checked) => setTicked((current) => ({ ...current, [key]: checked }))}
            error={signUpMutation.error}
          />

          <Group justify="space-between" align="center">
            <AnchorLink to={loginRoute.to} size="sm">
              {t(($) => $.signUp.loginLink)}
            </AnchorLink>
            <Button type="submit" loading={signUpMutation.isPending}>
              {t(($) => $.signUp.submitButton)}
            </Button>
          </Group>
        </Stack>
      </form>

      <SigninProviders separator="above" />
    </AuthLayout>
  )
}
