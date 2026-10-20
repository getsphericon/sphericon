import { Alert, Button, Group, PasswordInput, Stack, Text, TextInput } from '@mantine/core'
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
import { Route as loginRoute } from '../../routes/login.tsx'

// SoleOwnedWindowAlert warns that a Workspace in its deletion window, whose only Owner the
// User is, can no longer be restored once the User is gone (User deletion, ADR 0050). Nothing
// is shown when there is no such Workspace.
export function SoleOwnedWindowAlert() {
  const { t } = useTranslation()
  const deletedQuery = useQuery(siteWorkspacesListDeletedOptions())
  const doomed = (deletedQuery.data ?? []).filter((workspace) => workspace.onlyOwner)
  if (doomed.length === 0) {
    return null
  }
  return (
    <Alert color="yellow">
      {t(($) => $.profile.deleteUserWindowWarning, {
        workspaces: doomed.map((workspace) => workspace.name).join(', '),
      })}
    </Alert>
  )
}

// DeleteUserForm finishes the deletion of the signed-in User: the typed email, the proof (the
// password, or the `token` of the mailed link of a User with no password) and, with a Second
// factor, a code, then a confirmation. It lands on the login screen once the User is gone.
export function DeleteUserForm({ token }: { token?: string }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const statusQuery = useQuery(siteSecondFactorGetStatusOptions())
  const form = useForm({ initialValues: { email: '', currentPassword: '', code: '' } })

  const deleteUser = useResourceMutation({
    mutation: siteUserDeleteUserMutation(),
    errorTitle: t(($) => $.profile.deleteUserErrorTitle),
    onDone: async () => {
      queryClient.clear()
      await navigate({ to: loginRoute.to })
    },
  })

  const status = statusQuery.data
  if (!status) {
    return null
  }

  const handleSubmit = (values: typeof form.values) => {
    const body: SiteDeleteUserInput = { email: values.email.trim() }
    if (token === undefined) {
      body.currentPassword = values.currentPassword
    } else {
      body.token = token
    }
    if (status.enabled) {
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
      <Stack>
        <Text size="sm" c="dimmed">
          {t(($) => $.profile.deleteUserDescription)}
        </Text>
        <SoleOwnedWindowAlert />
        <TextInput
          label={t(($) => $.profile.deleteUserEmailLabel)}
          type="email"
          required
          {...form.getInputProps('email')}
        />
        {token === undefined ? (
          <PasswordInput
            label={t(($) => $.profile.deleteUserPasswordLabel)}
            required
            {...form.getInputProps('currentPassword')}
          />
        ) : null}
        {status.enabled ? (
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
