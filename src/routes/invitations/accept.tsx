import { Alert, Button, Loader, PasswordInput, Stack, Text, TextInput, Title } from '@mantine/core'
import { useForm } from '@mantine/form'
import { notifications } from '@mantine/notifications'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useLocation, useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import {
  sitePublicInvitationsAcceptMutation,
  sitePublicInvitationsLookupOptions,
  siteWorkspacesListOptions,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import { useApiErrorMessage } from '../../hooks/useApiErrorMessage.ts'
import { acceptInvitationRoute, indexRoute, loginRoute } from '../../router.tsx'

export function AcceptInvitationPage() {
  const { t } = useTranslation()
  const apiErrorMessage = useApiErrorMessage()
  const navigate = useNavigate()
  const { token } = acceptInvitationRoute.useParams()
  const location = useLocation()

  const lookup = useQuery({
    ...sitePublicInvitationsLookupOptions({ path: { token } }),
    retry: false,
  })

  // A Verified User accepts with their own session: probe it once the lookup says so.
  const session = useQuery({
    ...siteWorkspacesListOptions(),
    enabled: lookup.data?.hasAccount === true,
    retry: false,
  })

  const form = useForm({ initialValues: { name: '', password: '' } })

  const mutation = useMutation({
    ...sitePublicInvitationsAcceptMutation(),
    onSuccess: () => {
      const signedIn = lookup.data?.hasAccount === true
      notifications.show({
        color: 'teal',
        title: t(($) => $.acceptInvitation.successTitle),
        message: signedIn
          ? t(($) => $.acceptInvitation.joinedMessage)
          : t(($) => $.acceptInvitation.successMessage),
      })
      void navigate({ to: signedIn ? indexRoute.to : loginRoute.to })
    },
    onError: (error) => {
      notifications.show({
        color: 'red',
        title: t(($) => $.acceptInvitation.errorTitle),
        message: apiErrorMessage(
          error,
          t(($) => $.acceptInvitation.errorMessage),
        ),
      })
    },
  })

  if (lookup.isLoading || session.isLoading) {
    return (
      <Stack maw={420} mx="auto" mt="xl" align="center">
        <Loader />
        <Text c="dimmed">{t(($) => $.acceptInvitation.loading)}</Text>
      </Stack>
    )
  }

  if (lookup.isError || !lookup.data) {
    return (
      <Stack maw={420} mx="auto" mt="xl">
        <Alert color="red" title={t(($) => $.acceptInvitation.errorTitle)}>
          {t(($) => $.acceptInvitation.invalid)}
        </Alert>
      </Stack>
    )
  }

  const invite = lookup.data
  const needsAccount = !invite.hasAccount
  const needsSignIn = invite.hasAccount && session.isError

  const handleSubmit = (values: { name: string; password: string }) => {
    mutation.mutate({
      path: { token },
      body: needsAccount ? { name: values.name.trim(), password: values.password } : {},
    })
  }

  return (
    <Stack maw={420} mx="auto" mt="xl">
      <Title order={3}>{t(($) => $.acceptInvitation.title)}</Title>
      <Text>{t(($) => $.acceptInvitation.intro, { workspace: invite.workspaceName })}</Text>
      <Text c="dimmed" size="sm">
        {invite.email}
      </Text>

      {needsSignIn ? (
        <Stack>
          <Alert color="blue">{t(($) => $.acceptInvitation.signInRequired)}</Alert>
          <Button
            onClick={() =>
              void navigate({ to: loginRoute.to, search: { redirect: location.href } })
            }
          >
            {t(($) => $.acceptInvitation.signInButton)}
          </Button>
        </Stack>
      ) : (
        <form onSubmit={form.onSubmit(handleSubmit)}>
          <Stack>
            {needsAccount ? (
              <>
                <TextInput
                  label={t(($) => $.acceptInvitation.nameLabel)}
                  required
                  {...form.getInputProps('name')}
                />
                <PasswordInput
                  label={t(($) => $.acceptInvitation.passwordLabel)}
                  required
                  {...form.getInputProps('password')}
                />
              </>
            ) : null}
            <Button type="submit" loading={mutation.isPending}>
              {needsAccount
                ? t(($) => $.acceptInvitation.acceptButton)
                : t(($) => $.acceptInvitation.acceptExistingButton)}
            </Button>
          </Stack>
        </form>
      )}
    </Stack>
  )
}
