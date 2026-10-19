import { Button, Card, Group, PasswordInput, Stack, Text, Title } from '@mantine/core'
import { useForm } from '@mantine/form'
import { notifications } from '@mantine/notifications'
import { useMutation } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { AnchorLink } from '../../components/RouterLink.tsx'
import { siteAuthVerifyEmailMutation } from '../../generated/site/@tanstack/react-query.gen.ts'
import { Route as loginRoute } from '../../routes/login.tsx'
import { Route as verifyEmailRoute } from '../../routes/verify-email.tsx'
import { getApiErrorMessage } from '../../utils/apiErrors.ts'

// Where the link in the verification email lands (ADR 0030): the token proves the
// address and the password chosen here is the first one the account ever has.
export function VerifyEmailPage() {
  const { t } = useTranslation()
  const { token } = verifyEmailRoute.useSearch()

  const form = useForm({
    initialValues: { password: '', confirm: '' },
    validate: {
      confirm: (value, values) =>
        value === values.password ? null : t(($) => $.verifyEmail.mismatch),
    },
  })

  const mutation = useMutation({
    ...siteAuthVerifyEmailMutation(),
    onError: (error) => {
      form.setValues({ password: '', confirm: '' })
      notifications.show({
        color: 'red',
        title: t(($) => $.verifyEmail.errorTitle),
        message: getApiErrorMessage(
          error,
          t(($) => $.verifyEmail.errorMessage),
        ),
      })
    },
  })

  if (mutation.isSuccess) {
    return (
      <Stack maw={460} mx="auto" mt="xl">
        <Card withBorder padding="xl">
          <Stack align="center" gap="sm">
            <Title order={3}>{t(($) => $.verifyEmail.successTitle)}</Title>
            <Text c="dimmed" ta="center">
              {t(($) => $.verifyEmail.successBody)}
            </Text>
            <AnchorLink to={loginRoute.to} size="sm" mt="md">
              {t(($) => $.verifyEmail.signIn)}
            </AnchorLink>
          </Stack>
        </Card>
      </Stack>
    )
  }

  return (
    <Stack maw={420} mx="auto" mt="xl">
      <Title order={3}>{t(($) => $.verifyEmail.title)}</Title>

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
    </Stack>
  )
}
