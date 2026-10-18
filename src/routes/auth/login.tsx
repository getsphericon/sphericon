import {
  Alert,
  Anchor,
  Button,
  Group,
  PasswordInput,
  Stack,
  Text,
  TextInput,
  Title,
} from '@mantine/core'
import { useForm } from '@mantine/form'
import { notifications } from '@mantine/notifications'
import { useMutation } from '@tanstack/react-query'
import { useNavigate, useRouter, useSearch } from '@tanstack/react-router'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  siteAuthLoginMutation,
  siteAuthResendVerificationMutation,
  siteAuthSecondFactorMutation,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import type { SiteLoginInput, SiteLoginResult } from '../../generated/site/types.gen.ts'
import { useApiErrorMessage } from '../../hooks/useApiErrorMessage.ts'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'
import { forgotPasswordRoute, indexRoute, registerRoute } from '../../router.tsx'
import { isEmailVerificationRequiredError } from '../../utils/apiErrors.ts'

export function LoginPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const router = useRouter()
  // Set when a guard (e.g. the OAuth consent screen) sent the user here to sign in.
  const { redirect } = useSearch({ strict: false })

  const apiErrorMessage = useApiErrorMessage()

  const form = useForm<SiteLoginInput>({
    initialValues: {
      email: '',
      password: '',
    },
  })

  // Set when the password was right but the User has a Second factor (ADR 0020):
  // the session starts only after the code step.
  const [challenge, setChallenge] = useState<string | null>(null)
  const codeForm = useForm({ initialValues: { code: '' } })

  // Set when the login was refused because the address is not proven yet (ADR 0030):
  // the refusal offers to send the verification link again to this address.
  const [unverifiedEmail, setUnverifiedEmail] = useState<string | null>(null)

  const resendMutation = useResourceMutation({
    mutation: siteAuthResendVerificationMutation(),
    successMessage: t(($) => $.login.resendSuccess),
    errorTitle: t(($) => $.login.resendErrorTitle),
  })

  const showError = (error: unknown) =>
    notifications.show({
      color: 'red',
      title: t(($) => $.login.errorTitle),
      message: apiErrorMessage(
        error,
        t(($) => $.login.errorMessage),
      ),
    })

  const signedIn = () =>
    redirect ? router.history.push(redirect) : navigate({ to: indexRoute.to })

  const loginMutation = useMutation({
    ...siteAuthLoginMutation(),
    onSuccess: (result: SiteLoginResult) => {
      if (result.outcome === 'challenge' && result.challenge) {
        setChallenge(result.challenge)
        return
      }
      return signedIn()
    },
    onError: (error, variables) => {
      // Drop the rejected password so the user retypes a fresh one.
      form.setFieldValue('password', '')
      if (isEmailVerificationRequiredError(error)) {
        setUnverifiedEmail(variables.body.email)
        return
      }
      setUnverifiedEmail(null)
      showError(error)
    },
  })

  const secondStepMutation = useMutation({
    ...siteAuthSecondFactorMutation(),
    onSuccess: signedIn,
    onError: (error) => {
      codeForm.setFieldValue('code', '')
      showError(error)
    },
  })

  const startOver = () => {
    setChallenge(null)
    codeForm.reset()
    form.setFieldValue('password', '')
  }

  if (challenge) {
    return (
      <Stack maw={400} mx="auto" mt="xl">
        <Title order={3}>{t(($) => $.login.secondStepTitle)}</Title>
        <Text size="sm" c="dimmed">
          {t(($) => $.login.secondStepDescription)}
        </Text>

        <form
          onSubmit={codeForm.onSubmit((values) =>
            secondStepMutation.mutate({ body: { challenge, code: values.code.trim() } }),
          )}
        >
          <Stack>
            <TextInput
              label={t(($) => $.login.codeLabel)}
              autoComplete="one-time-code"
              required
              data-autofocus
              {...codeForm.getInputProps('code')}
            />
            <Group justify="space-between" align="center">
              <Anchor component="button" type="button" size="sm" onClick={startOver}>
                {t(($) => $.login.startOver)}
              </Anchor>
              <Button type="submit" loading={secondStepMutation.isPending}>
                {t(($) => $.login.verifyButton)}
              </Button>
            </Group>
          </Stack>
        </form>
      </Stack>
    )
  }

  const handleSubmit = (values: SiteLoginInput) => {
    setUnverifiedEmail(null)
    loginMutation.mutate({
      body: {
        email: values.email.trim(),
        password: values.password,
      },
    })
  }

  return (
    <Stack maw={400} mx="auto" mt="xl">
      <Title order={3}>{t(($) => $.login.title)}</Title>

      {unverifiedEmail ? (
        <Alert color="yellow" title={t(($) => $.login.verifyRequiredTitle)}>
          <Stack gap="xs" align="flex-start">
            <Text size="sm">{t(($) => $.login.verifyRequiredMessage)}</Text>
            <Button
              variant="light"
              size="xs"
              loading={resendMutation.isPending}
              onClick={() => resendMutation.mutate({ body: { email: unverifiedEmail } })}
            >
              {t(($) => $.login.resendVerification)}
            </Button>
          </Stack>
        </Alert>
      ) : null}

      <form onSubmit={form.onSubmit(handleSubmit)}>
        <Stack>
          <TextInput
            label={t(($) => $.login.emailLabel)}
            type="email"
            required
            {...form.getInputProps('email')}
          />
          <PasswordInput
            label={t(($) => $.login.passwordLabel)}
            required
            {...form.getInputProps('password')}
          />

          <Group justify="space-between" align="center">
            <Stack gap={2}>
              <Anchor component="a" href={registerRoute.to} size="sm">
                {t(($) => $.login.registerLink)}
              </Anchor>
              <Anchor component="a" href={forgotPasswordRoute.to} size="sm">
                {t(($) => $.login.forgotPasswordLink)}
              </Anchor>
            </Stack>
            <Button type="submit" loading={loginMutation.isPending}>
              {t(($) => $.login.submitButton)}
            </Button>
          </Group>
        </Stack>
      </form>
    </Stack>
  )
}
