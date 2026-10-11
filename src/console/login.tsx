import {
  Anchor,
  Button,
  Code,
  Group,
  Image,
  PasswordInput,
  Stack,
  Text,
  TextInput,
  Title,
} from '@mantine/core'
import { useForm } from '@mantine/form'
import { notifications } from '@mantine/notifications'
import { useMutation } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  operatorAuthLoginMutation,
  operatorAuthSecondFactorMutation,
} from '../generated/operator/@tanstack/react-query.gen.ts'
import type { OperatorEnrolment, OperatorLoginResult } from '../generated/operator/types.gen.ts'
import { useApiErrorMessage } from '../hooks/useApiErrorMessage.ts'
import { consoleHomeRoute } from '../router.tsx'
import { getRateLimitWait } from '../utils/apiErrors.ts'

// What the password step granted: a code to enter, plus the secret to add to an
// authenticator app when the Operator has no TOTP yet (first login).
type CodeStep = { challenge: string; enrolment: OperatorEnrolment | undefined }

export function ConsoleLoginPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const apiErrorMessage = useApiErrorMessage()

  const form = useForm({ initialValues: { email: '', password: '' } })
  const codeForm = useForm({ initialValues: { code: '' } })
  const [step, setStep] = useState<CodeStep | null>(null)

  const showError = (error: unknown) => {
    const wait = getRateLimitWait(error)
    const rateLimited = wait
      ? wait.unit === 'minutes'
        ? t(($) => $.console.login.rateLimitedMinutes, { count: wait.count })
        : t(($) => $.console.login.rateLimitedSeconds, { count: wait.count })
      : undefined
    notifications.show({
      color: 'red',
      title: t(($) => $.console.login.errorTitle),
      message:
        rateLimited ??
        apiErrorMessage(
          error,
          t(($) => $.console.login.errorMessage),
        ),
    })
  }

  const loginMutation = useMutation({
    ...operatorAuthLoginMutation(),
    onSuccess: (result: OperatorLoginResult) => {
      setStep({ challenge: result.challenge, enrolment: result.enrolment })
    },
    onError: (error) => {
      form.setFieldValue('password', '')
      showError(error)
    },
  })

  const secondStepMutation = useMutation({
    ...operatorAuthSecondFactorMutation(),
    onSuccess: () => navigate({ to: consoleHomeRoute.to }),
    onError: (error) => {
      codeForm.setFieldValue('code', '')
      showError(error)
    },
  })

  const startOver = () => {
    setStep(null)
    codeForm.reset()
    form.setFieldValue('password', '')
  }

  if (step) {
    const { challenge, enrolment } = step
    return (
      <Stack maw={400} mx="auto" mt="xl" p="md">
        <Title order={3}>
          {enrolment
            ? t(($) => $.console.login.enrolmentTitle)
            : t(($) => $.console.login.secondStepTitle)}
        </Title>
        <Text size="sm" c="dimmed">
          {enrolment
            ? t(($) => $.console.login.enrolmentDescription)
            : t(($) => $.console.login.secondStepDescription)}
        </Text>

        {enrolment ? (
          <Stack align="center" gap="xs">
            <Image
              src={enrolment.qrCode}
              alt={t(($) => $.console.login.qrAlt)}
              w={200}
              h={200}
              fit="contain"
            />
            <Text size="sm" c="dimmed">
              {t(($) => $.console.login.secretLabel)}
            </Text>
            <Code>{enrolment.secret}</Code>
          </Stack>
        ) : null}

        <form
          onSubmit={codeForm.onSubmit((values) =>
            secondStepMutation.mutate({ body: { challenge, code: values.code.trim() } }),
          )}
        >
          <Stack>
            <TextInput
              label={t(($) => $.console.login.codeLabel)}
              autoComplete="one-time-code"
              inputMode="numeric"
              required
              data-autofocus
              {...codeForm.getInputProps('code')}
            />
            <Group justify="space-between" align="center">
              <Anchor component="button" type="button" size="sm" onClick={startOver}>
                {t(($) => $.console.login.startOver)}
              </Anchor>
              <Button type="submit" loading={secondStepMutation.isPending}>
                {enrolment
                  ? t(($) => $.console.login.confirmButton)
                  : t(($) => $.console.login.verifyButton)}
              </Button>
            </Group>
          </Stack>
        </form>
      </Stack>
    )
  }

  return (
    <Stack maw={400} mx="auto" mt="xl" p="md">
      <Title order={3}>{t(($) => $.console.login.title)}</Title>

      <form
        onSubmit={form.onSubmit((values) =>
          loginMutation.mutate({
            body: { email: values.email.trim(), password: values.password },
          }),
        )}
      >
        <Stack>
          <TextInput
            label={t(($) => $.console.login.emailLabel)}
            type="email"
            autoComplete="username"
            required
            {...form.getInputProps('email')}
          />
          <PasswordInput
            label={t(($) => $.console.login.passwordLabel)}
            autoComplete="current-password"
            required
            {...form.getInputProps('password')}
          />
          <Group justify="flex-end">
            <Button type="submit" loading={loginMutation.isPending}>
              {t(($) => $.console.login.submitButton)}
            </Button>
          </Group>
        </Stack>
      </form>
    </Stack>
  )
}
