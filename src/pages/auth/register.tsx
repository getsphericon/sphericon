import { Button, Card, Group, PasswordInput, Stack, Text, TextInput, Title } from '@mantine/core'
import { useForm } from '@mantine/form'
import { useInterval } from '@mantine/hooks'
import { useMutation } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { CenteredLayout } from '../../components/CenteredLayout.tsx'
import { AnchorLink } from '../../components/RouterLink.tsx'
import { SigninProviders } from '../../components/SigninProviders.tsx'
import {
  siteAuthRegisterMutation,
  siteAuthResendVerificationMutation,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import type { SiteRegisterInput } from '../../generated/site/types.gen.ts'
import { useApiErrorMessage } from '../../hooks/useApiErrorMessage.ts'
import { useNotify } from '../../hooks/useNotify.ts'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'
import { requiresEmailVerification } from '../../instance.ts'
import { Route as loginRoute } from '../../routes/login.tsx'

// Seconds the resend button waits after a send; the server's per-IP limit stays the real gate.
const RESEND_COOLDOWN_SECONDS = 60

export function RegisterPage() {
  const { t } = useTranslation()
  const notify = useNotify()
  const apiErrorMessage = useApiErrorMessage()
  // With the gate off (ADR 0030) the account is usable at once; with it on, the link
  // to the address proves it, and signs in when followed in this browser.
  const usableAtOnce = !requiresEmailVerification()
  // The address the verification link went to; set once registration is accepted.
  const [sentTo, setSentTo] = useState<string | null>(null)
  const [created, setCreated] = useState(false)

  const [cooldown, setCooldown] = useState(0)
  const ticker = useInterval(() => setCooldown((seconds) => Math.max(0, seconds - 1)), 1000)
  const resendMutation = useResourceMutation({
    mutation: siteAuthResendVerificationMutation(),
    successMessage: t(($) => $.login.resendSuccess),
    errorTitle: t(($) => $.login.resendErrorTitle),
    onDone: () => {
      setCooldown(RESEND_COOLDOWN_SECONDS)
      ticker.start()
    },
  })
  useEffect(() => {
    if (cooldown === 0) {
      ticker.stop()
    }
  }, [cooldown, ticker])

  const form = useForm<Required<SiteRegisterInput> & { confirm: string }>({
    initialValues: {
      name: '',
      email: '',
      password: '',
      confirm: '',
    },
    validate: {
      confirm: (value, values) =>
        value === values.password ? null : t(($) => $.registration.mismatch),
    },
  })

  const registerMutation = useMutation({
    ...siteAuthRegisterMutation(),
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
        title: t(($) => $.registration.errorTitle),
        message: apiErrorMessage(
          error,
          t(($) => $.registration.errorMessage),
        ),
      })
    },
  })

  const handleSubmit = (values: Required<SiteRegisterInput>) => {
    registerMutation.mutate({
      body: {
        name: values.name.trim(),
        email: values.email.trim(),
        password: values.password,
      },
    })
  }

  if (created) {
    return (
      <CenteredLayout maw={460}>
        <Card withBorder padding="xl">
          <Stack align="center" gap="sm">
            <Title order={3}>{t(($) => $.registration.createdTitle)}</Title>
            <Text c="dimmed" ta="center">
              {t(($) => $.registration.createdBody)}
            </Text>
            <AnchorLink to={loginRoute.to} size="sm" mt="md">
              {t(($) => $.registration.signInLink)}
            </AnchorLink>
          </Stack>
        </Card>
      </CenteredLayout>
    )
  }

  if (sentTo) {
    return (
      <CenteredLayout maw={460}>
        <Card withBorder padding="xl">
          <Stack align="center" gap="sm">
            <Title order={3}>{t(($) => $.registration.checkEmailTitle)}</Title>
            <Text c="dimmed" ta="center">
              {t(($) => $.registration.checkEmailBody, { email: sentTo })}
            </Text>
            <Text size="sm" mt="md">
              {t(($) => $.registration.didntReceive)}
            </Text>
            <Button
              variant="default"
              loading={resendMutation.isPending}
              disabled={cooldown > 0}
              onClick={() => resendMutation.mutate({ body: { email: sentTo } })}
            >
              {cooldown > 0
                ? t(($) => $.registration.resendIn, { seconds: cooldown })
                : t(($) => $.login.resendVerification)}
            </Button>
            <AnchorLink to={loginRoute.to} size="sm" mt="md">
              {t(($) => $.registration.loginLink)}
            </AnchorLink>
          </Stack>
        </Card>
      </CenteredLayout>
    )
  }

  return (
    <CenteredLayout>
      <Title order={3}>{t(($) => $.registration.title)}</Title>

      <form onSubmit={form.onSubmit(handleSubmit)}>
        <Stack>
          <TextInput
            label={t(($) => $.registration.nameLabel)}
            required
            {...form.getInputProps('name')}
          />
          <TextInput
            label={t(($) => $.registration.emailLabel)}
            type="email"
            required
            {...form.getInputProps('email')}
          />
          <PasswordInput
            label={t(($) => $.registration.passwordLabel)}
            autoComplete="new-password"
            required
            {...form.getInputProps('password')}
          />
          <PasswordInput
            label={t(($) => $.registration.confirmPasswordLabel)}
            autoComplete="new-password"
            required
            {...form.getInputProps('confirm')}
          />

          <Group justify="space-between" align="center">
            <AnchorLink to={loginRoute.to} size="sm">
              {t(($) => $.registration.loginLink)}
            </AnchorLink>
            <Button type="submit" loading={registerMutation.isPending}>
              {t(($) => $.registration.submitButton)}
            </Button>
          </Group>
        </Stack>
      </form>

      <SigninProviders separator="above" />
    </CenteredLayout>
  )
}
