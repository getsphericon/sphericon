import { Button, Group, PasswordInput, Stack, Title } from '@mantine/core'
import { useForm } from '@mantine/form'
import { useMutation } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { CenteredLayout } from '../../components/CenteredLayout.tsx'
import { siteAuthResetPasswordMutation } from '../../generated/site/@tanstack/react-query.gen.ts'
import { useNotify } from '../../hooks/useNotify.ts'
import { Route as loginRoute } from '../../routes/login.tsx'
import { Route as resetPasswordRoute } from '../../routes/reset-password.tsx'
import { getApiErrorMessage } from '../../utils/apiErrors.ts'

export function ResetPasswordPage() {
  const { t } = useTranslation()
  const notify = useNotify()
  const navigate = useNavigate()
  const { token } = resetPasswordRoute.useSearch()

  const form = useForm({
    initialValues: { password: '', confirm: '' },
    validate: {
      confirm: (value, values) =>
        value === values.password ? null : t(($) => $.resetPassword.mismatch),
    },
  })

  const mutation = useMutation({
    ...siteAuthResetPasswordMutation(),
    onSuccess: () => {
      notify({
        color: 'teal',
        title: t(($) => $.notifications.successTitle),
        message: t(($) => $.resetPassword.successMessage),
      })
      void navigate({ to: loginRoute.to })
    },
    onError: (error) => {
      form.setValues({ password: '', confirm: '' })
      notify({
        color: 'red',
        title: t(($) => $.resetPassword.errorTitle),
        message: getApiErrorMessage(
          error,
          t(($) => $.resetPassword.errorMessage),
        ),
      })
    },
  })

  const handleSubmit = (values: { password: string }) => {
    mutation.mutate({ body: { token, password: values.password } })
  }

  return (
    <CenteredLayout maw={420}>
      <Title order={3}>{t(($) => $.resetPassword.title)}</Title>

      <form onSubmit={form.onSubmit(handleSubmit)}>
        <Stack>
          <PasswordInput
            label={t(($) => $.resetPassword.newPasswordLabel)}
            required
            {...form.getInputProps('password')}
          />
          <PasswordInput
            label={t(($) => $.resetPassword.confirmPasswordLabel)}
            required
            {...form.getInputProps('confirm')}
          />
          <Group justify="flex-end">
            <Button type="submit" loading={mutation.isPending}>
              {t(($) => $.resetPassword.submitButton)}
            </Button>
          </Group>
        </Stack>
      </form>
    </CenteredLayout>
  )
}
