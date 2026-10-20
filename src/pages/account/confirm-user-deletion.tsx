import { Alert, Button, Divider, Group, Stack, Text, TextInput, Title } from '@mantine/core'
import { useForm } from '@mantine/form'
import { modals } from '@mantine/modals'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import {
  siteSecondFactorGetStatusOptions,
  siteUserDeleteUserMutation,
  siteWorkspacesListDeletedOptions,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import type { SiteDeleteUserInput } from '../../generated/site/types.gen.ts'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'
import { Route as deleteRoute } from '../../routes/account/delete.tsx'
import { Route as loginRoute } from '../../routes/login.tsx'

// ConfirmUserDeletionPage is where the mailed link of a User with no password lands (User
// deletion, ADR 0050): the token of the link, the User's own email typed in and, with a Second
// factor, a code finish the deletion.
export function ConfirmUserDeletionPage() {
  const { t } = useTranslation()
  const { token } = deleteRoute.useSearch()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const statusQuery = useQuery(siteSecondFactorGetStatusOptions())
  const deletedQuery = useQuery(siteWorkspacesListDeletedOptions())
  const form = useForm({ initialValues: { email: '', code: '' } })

  const deleteUser = useResourceMutation({
    mutation: siteUserDeleteUserMutation(),
    errorTitle: t(($) => $.profile.deleteUserErrorTitle),
    onDone: async () => {
      queryClient.clear()
      await navigate({ to: loginRoute.to })
    },
  })

  const status = statusQuery.data
  const waiting = deletedQuery.data ?? []

  const handleSubmit = (values: typeof form.values) => {
    const body: SiteDeleteUserInput = { email: values.email.trim(), token }
    if (status?.enabled) {
      body.code = values.code.trim()
    }
    modals.openConfirmModal({
      title: t(($) => $.profile.deleteUserTitle),
      children: <Text>{t(($) => $.profile.deleteUserConfirm)}</Text>,
      labels: {
        cancel: t(($) => $.actions.cancel),
        confirm: t(($) => $.profile.deleteUserButton),
      },
      confirmProps: { color: 'red' },
      onConfirm: () => deleteUser.mutate({ body }),
    })
  }

  return (
    <form onSubmit={form.onSubmit(handleSubmit)}>
      <Stack maw={420}>
        <Title order={2}>{t(($) => $.profile.deleteUserSectionTitle)}</Title>
        <Divider />
        <Text size="sm" c="dimmed">
          {t(($) => $.profile.deleteUserDescription)}
        </Text>
        {waiting.length > 0 ? (
          <Alert color="yellow">
            {t(($) => $.profile.deleteUserWindowWarning, {
              workspaces: waiting.map((workspace) => workspace.name).join(', '),
            })}
          </Alert>
        ) : null}
        <TextInput
          label={t(($) => $.profile.deleteUserEmailLabel)}
          type="email"
          required
          {...form.getInputProps('email')}
        />
        {status?.enabled ? (
          <TextInput
            label={t(($) => $.profile.deleteUserCodeLabel)}
            required
            {...form.getInputProps('code')}
          />
        ) : null}
        <Group justify="flex-end">
          <Button type="submit" color="red" loading={deleteUser.isPending}>
            {t(($) => $.profile.deleteUserButton)}
          </Button>
        </Group>
      </Stack>
    </form>
  )
}
