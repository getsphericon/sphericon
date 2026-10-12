import { Anchor, Button, Card, Group, Stack, Text, TextInput, Title } from '@mantine/core'
import { useForm } from '@mantine/form'
import { notifications } from '@mantine/notifications'
import { useMutation } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { siteAuthRegisterMutation } from '../../generated/site/@tanstack/react-query.gen.ts'
import type { SiteRegisterInput } from '../../generated/site/types.gen.ts'
import { useApiErrorMessage } from '../../hooks/useApiErrorMessage.ts'
import { loginRoute } from '../../router.tsx'

export function RegisterPage() {
  const { t } = useTranslation()
  const apiErrorMessage = useApiErrorMessage()
  // The address the verification link went to; set once registration is accepted.
  const [sentTo, setSentTo] = useState<string | null>(null)

  const form = useForm<SiteRegisterInput>({
    initialValues: {
      name: '',
      email: '',
    },
  })

  const registerMutation = useMutation({
    ...siteAuthRegisterMutation(),
    onSuccess: (_data, variables) => setSentTo(variables.body.email),
    onError: (error) => {
      notifications.show({
        color: 'red',
        title: t(($) => $.registration.errorTitle),
        message: apiErrorMessage(
          error,
          t(($) => $.registration.errorMessage),
        ),
      })
    },
  })

  const handleSubmit = (values: SiteRegisterInput) => {
    registerMutation.mutate({
      body: {
        name: values.name.trim(),
        email: values.email.trim(),
      },
    })
  }

  if (sentTo) {
    return (
      <Stack maw={460} mx="auto" mt="xl">
        <Card withBorder padding="xl">
          <Stack align="center" gap="sm">
            <Title order={3}>{t(($) => $.registration.checkEmailTitle)}</Title>
            <Text c="dimmed" ta="center">
              {t(($) => $.registration.checkEmailBody, { email: sentTo })}
            </Text>
            <Anchor component="a" href={loginRoute.to} size="sm" mt="md">
              {t(($) => $.registration.loginLink)}
            </Anchor>
          </Stack>
        </Card>
      </Stack>
    )
  }

  return (
    <Stack maw={400} mx="auto" mt="xl">
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

          <Group justify="space-between" align="center">
            <Anchor component="a" href={loginRoute.to} size="sm">
              {t(($) => $.registration.loginLink)}
            </Anchor>
            <Button type="submit" loading={registerMutation.isPending}>
              {t(($) => $.registration.submitButton)}
            </Button>
          </Group>
        </Stack>
      </form>
    </Stack>
  )
}
