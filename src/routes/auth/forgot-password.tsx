import { Alert, Anchor, Button, Card, Group, Stack, Text, TextInput, Title } from '@mantine/core'
import { useForm } from '@mantine/form'
import { useMutation } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { siteAuthForgotPasswordMutation } from '../../generated/site/@tanstack/react-query.gen.ts'
import { useApiErrorMessage } from '../../hooks/useApiErrorMessage.ts'
import { loginRoute } from '../../router.tsx'

export function ForgotPasswordPage() {
  const { t } = useTranslation()
  const apiErrorMessage = useApiErrorMessage()

  const form = useForm({ initialValues: { email: '' } })

  // Succeeds (202) whether or not the address exists, or has had its mails for the
  // hour, so a single confirmation state covers all of them — no account
  // enumeration. The one failure a visitor sees is the per-IP 429.
  const mutation = useMutation(siteAuthForgotPasswordMutation())

  const handleSubmit = (values: { email: string }) => {
    mutation.mutate({ body: { email: values.email.trim() } })
  }

  if (mutation.isSuccess) {
    return (
      <Stack maw={420} mx="auto" mt="xl">
        <Card withBorder padding="xl">
          <Stack gap="sm">
            <Title order={3}>{t(($) => $.forgotPassword.successTitle)}</Title>
            <Text c="dimmed">{t(($) => $.forgotPassword.successBody)}</Text>
            <Anchor component="a" href={loginRoute.to} size="sm">
              {t(($) => $.forgotPassword.backToLogin)}
            </Anchor>
          </Stack>
        </Card>
      </Stack>
    )
  }

  return (
    <Stack maw={420} mx="auto" mt="xl">
      <Title order={3}>{t(($) => $.forgotPassword.title)}</Title>
      <Text c="dimmed" size="sm">
        {t(($) => $.forgotPassword.description)}
      </Text>

      <form onSubmit={form.onSubmit(handleSubmit)}>
        <Stack>
          {mutation.isError && (
            <Alert color="red">
              {apiErrorMessage(
                mutation.error,
                t(($) => $.notifications.errorMessage),
              )}
            </Alert>
          )}
          <TextInput
            label={t(($) => $.forgotPassword.emailLabel)}
            type="email"
            required
            {...form.getInputProps('email')}
          />
          <Group justify="space-between" align="center">
            <Anchor component="a" href={loginRoute.to} size="sm">
              {t(($) => $.forgotPassword.backToLogin)}
            </Anchor>
            <Button type="submit" loading={mutation.isPending}>
              {t(($) => $.forgotPassword.submitButton)}
            </Button>
          </Group>
        </Stack>
      </form>
    </Stack>
  )
}
