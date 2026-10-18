import {
  Alert,
  Button,
  Divider,
  Loader,
  PasswordInput,
  Stack,
  Text,
  TextInput,
  Title,
} from '@mantine/core'
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
import { Route as indexRoute } from '../../routes/index.tsx'
import { Route as acceptInvitationRoute } from '../../routes/invitations/$token.tsx'
import { Route as loginRoute } from '../../routes/login.tsx'

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

  // The lookup says nothing about whether the address has an account (ADR 0031), so the
  // page offers both ways in: set up an account, or join with the session of the account
  // that already holds the address. Probe the session to know which of the two to show.
  const session = useQuery({
    ...siteWorkspacesListOptions(),
    enabled: lookup.isSuccess,
    retry: false,
  })

  const form = useForm({ initialValues: { name: '', password: '' } })

  const mutation = useMutation({
    ...sitePublicInvitationsAcceptMutation(),
    onSuccess: (_data, variables) => {
      const joined = Object.keys(variables.body).length === 0
      notifications.show({
        color: 'teal',
        title: t(($) => $.acceptInvitation.successTitle),
        message: joined
          ? t(($) => $.acceptInvitation.joinedMessage)
          : t(($) => $.acceptInvitation.successMessage),
      })
      void navigate({ to: joined ? indexRoute.to : loginRoute.to })
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
  const signedIn = session.isSuccess

  const handleSubmit = (values: { name: string; password: string }) => {
    mutation.mutate({
      path: { token },
      body: { name: values.name.trim(), password: values.password },
    })
  }

  return (
    <Stack maw={420} mx="auto" mt="xl">
      <Title order={3}>{t(($) => $.acceptInvitation.title)}</Title>
      <Text>{t(($) => $.acceptInvitation.intro, { workspace: invite.workspaceName })}</Text>
      <Text c="dimmed" size="sm">
        {invite.email}
      </Text>

      <form onSubmit={form.onSubmit(handleSubmit)}>
        <Stack>
          <TextInput
            label={t(($) => $.acceptInvitation.nameLabel)}
            required
            {...form.getInputProps('name')}
          />
          <PasswordInput
            label={t(($) => $.acceptInvitation.passwordLabel)}
            autoComplete="new-password"
            required
            {...form.getInputProps('password')}
          />
          <Button type="submit" loading={mutation.isPending}>
            {t(($) => $.acceptInvitation.acceptButton)}
          </Button>
        </Stack>
      </form>

      <Divider label={t(($) => $.acceptInvitation.haveAccount)} labelPosition="center" />
      {signedIn ? (
        <Button
          variant="default"
          loading={mutation.isPending}
          onClick={() => mutation.mutate({ path: { token }, body: {} })}
        >
          {t(($) => $.acceptInvitation.acceptExistingButton)}
        </Button>
      ) : (
        <Button
          variant="default"
          onClick={() => void navigate({ to: loginRoute.to, search: { redirect: location.href } })}
        >
          {t(($) => $.acceptInvitation.signInButton)}
        </Button>
      )}
    </Stack>
  )
}
