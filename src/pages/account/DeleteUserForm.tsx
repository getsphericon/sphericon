import { Alert, Button, Divider, Group, PasswordInput, Stack, Text, TextInput } from '@mantine/core'
import { useForm } from '@mantine/form'
import { modals } from '@mantine/modals'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import {
  siteSecondFactorGetStatusOptions,
  siteUserDeleteUserLinkMutation,
  siteUserDeleteUserMutation,
  siteWorkspacesListDeletedOptions,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import type { SiteDeleteUserInput, SiteSoleOwnedWorkspace } from '../../generated/site/types.gen.ts'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'
import { Route as loginRoute } from '../../routes/login.tsx'
import { getSoleOwnedWorkspaces } from '../../utils/apiErrors.ts'

// What a page does with the Workspaces a refused deletion names, the ones the User is the only
// Owner of (ADR 0050): the acceptance screen offers to delete each.
type OnSoleOwned = (workspaces: SiteSoleOwnedWorkspace[]) => void

// soleOwnedHandler hands the Workspaces a refused deletion names to onSoleOwned and lets the
// refusal toast as any other.
function soleOwnedHandler(onSoleOwned?: OnSoleOwned) {
  return (error: unknown) => {
    const workspaces = getSoleOwnedWorkspaces(error)
    if (workspaces) {
      onSoleOwned?.(workspaces)
    }
    return false
  }
}

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
export function DeleteUserForm({
  token,
  onSoleOwned,
}: {
  token?: string
  onSoleOwned?: OnSoleOwned | undefined
}) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const statusQuery = useQuery(siteSecondFactorGetStatusOptions())
  const form = useForm({ initialValues: { email: '', currentPassword: '', code: '' } })

  const deleteUser = useResourceMutation({
    mutation: siteUserDeleteUserMutation(),
    errorTitle: t(($) => $.profile.deleteUserErrorTitle),
    handleError: soleOwnedHandler(onSoleOwned),
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

// DeleteUserSection erases the User at once (User deletion, ADR 0050): the typed email, the
// password and, with a Second factor, a code. A Workspace waiting for erasure that the User
// solely owns is named, since it can no longer be restored afterwards. A User with no password
// asks for a one-time link to their Verified email instead of typing a password; the link
// lands on the confirm page. onSoleOwned gets the Workspaces a refused deletion names.
export function DeleteUserSection({ onSoleOwned }: { onSoleOwned?: OnSoleOwned }) {
  const { t } = useTranslation()
  const statusQuery = useQuery(siteSecondFactorGetStatusOptions())
  const linkForm = useForm({ initialValues: { email: '' } })

  const requestLink = useResourceMutation({
    mutation: siteUserDeleteUserLinkMutation(),
    errorTitle: t(($) => $.profile.deleteUserLinkErrorTitle),
    handleError: soleOwnedHandler(onSoleOwned),
    successMessage: t(($) => $.profile.deleteUserLinkSent),
  })

  const status = statusQuery.data
  if (!status) {
    return null
  }

  return (
    <Stack maw={420}>
      <Divider label={t(($) => $.profile.deleteUserSectionTitle)} />
      {status.passwordSet ? (
        <DeleteUserForm onSoleOwned={onSoleOwned} />
      ) : (
        <form
          onSubmit={linkForm.onSubmit((values) =>
            requestLink.mutate({ body: { email: values.email.trim() } }),
          )}
        >
          <Stack>
            <Text size="sm" c="dimmed">
              {t(($) => $.profile.deleteUserLinkDescription)}
            </Text>
            <SoleOwnedWindowAlert />
            <TextInput
              label={t(($) => $.profile.deleteUserEmailLabel)}
              type="email"
              required
              {...linkForm.getInputProps('email')}
            />
            <Group justify="flex-end">
              <Button type="submit" color="red" variant="light" loading={requestLink.isPending}>
                {t(($) => $.profile.deleteUserLinkButton)}
              </Button>
            </Group>
          </Stack>
        </form>
      )}
    </Stack>
  )
}
