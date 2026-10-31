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
import { useMutation, useQuery, useSuspenseQuery } from '@tanstack/react-query'
import { useLocation, useNavigate, useSearch } from '@tanstack/react-router'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { CenteredLayout } from '../../components/CenteredLayout.tsx'
import { QueryBoundary } from '../../components/QueryBoundary.tsx'
import { ButtonLink } from '../../components/RouterLink.tsx'
import { SigninProviders } from '../../components/SigninProviders.tsx'
import { SignupItemFields } from '../../components/SignupItemFields.tsx'
import {
  siteLegalDocumentsSignupItemsOptions,
  sitePublicInvitationsAcceptMutation,
  sitePublicInvitationsLookupOptions,
  siteWorkspacesListOptions,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import { zSiteSigninError } from '../../generated/site/zod.gen.ts'
import { useAcceptanceWatch } from '../../hooks/useAcceptanceWatch.ts'
import { useApiErrorMessage } from '../../hooks/useApiErrorMessage.ts'
import { useNotify } from '../../hooks/useNotify.ts'
import { Route as indexRoute } from '../../routes/index.tsx'
import { Route as acceptInvitationRoute } from '../../routes/invitations/$token.tsx'
import { Route as loginRoute } from '../../routes/login.tsx'
import { signupItemAnswers } from '../../utils/signupItems.ts'

// InvitationLoading shows while the invitation is looked up and the session probed.
function InvitationLoading() {
  const { t } = useTranslation()
  return (
    <CenteredLayout maw={420} align="center">
      <Loader />
      <Text c="dimmed">{t(($) => $.acceptInvitation.loading)}</Text>
    </CenteredLayout>
  )
}

export function AcceptInvitationPage() {
  const { t } = useTranslation()
  // A signed-in User with a required acceptance pending (ADR 0062) is refused the session probe
  // and the accept alike: they go to the accept-or-sign-out screen first.
  useAcceptanceWatch()

  return (
    <QueryBoundary
      fallback={<InvitationLoading />}
      errorTitle={t(($) => $.acceptInvitation.errorTitle)}
      // An invitation that cannot be looked up is unknown, revoked or expired, all alike.
      renderError={() => (
        <CenteredLayout maw={420}>
          <Alert color="red" title={t(($) => $.acceptInvitation.errorTitle)}>
            {t(($) => $.acceptInvitation.invalid)}
          </Alert>
        </CenteredLayout>
      )}
    >
      <AcceptInvitation />
    </QueryBoundary>
  )
}

function AcceptInvitation() {
  const { t } = useTranslation()
  const notify = useNotify()
  const apiErrorMessage = useApiErrorMessage()
  const navigate = useNavigate()
  const { token } = acceptInvitationRoute.useParams()
  const location = useLocation()
  // Set when the server's provider callback sent the person back here without joining.
  const signinError = zSiteSigninError.safeParse(useSearch({ strict: false }).error).data

  const invite = useSuspenseQuery({
    ...sitePublicInvitationsLookupOptions({ path: { token } }),
    retry: false,
  }).data

  // The lookup says nothing about whether the address has an account (ADR 0031), so the
  // page offers both ways in: set up an account, or join with the session of the account
  // that already holds the address. Probe the session to know which of the two to show. A
  // refused probe means signed out, not a failure, so it stays out of the boundary.
  const session = useQuery({ ...siteWorkspacesListOptions(), retry: false })

  const form = useForm({ initialValues: { name: '', password: '' } })
  // The Sign-up items in effect (ADR 0062), asked only of an invitee who sets up an account:
  // consent follows an existing User, so joining with a session answers none.
  const itemsQuery = useQuery(siteLegalDocumentsSignupItemsOptions())
  const items = itemsQuery.data?.items ?? []
  const [ticked, setTicked] = useState<Record<string, boolean>>({})

  const mutation = useMutation({
    ...sitePublicInvitationsAcceptMutation(),
    onSuccess: (_data, variables) => {
      const joined = Object.keys(variables.body).length === 0
      notify({
        color: 'teal',
        title: t(($) => $.acceptInvitation.successTitle),
        message: joined
          ? t(($) => $.acceptInvitation.joinedMessage)
          : t(($) => $.acceptInvitation.successMessage),
      })
      void navigate({ to: joined ? indexRoute.to : loginRoute.to })
    },
    onError: (error) => {
      notify({
        color: 'red',
        title: t(($) => $.acceptInvitation.errorTitle),
        message: apiErrorMessage(
          error,
          t(($) => $.acceptInvitation.errorMessage),
        ),
      })
    },
  })

  if (session.isLoading) {
    return <InvitationLoading />
  }

  const signedIn = session.isSuccess

  const handleSubmit = (values: { name: string; password: string }) => {
    // With no items the form sends no answers at all, as it did before items existed.
    const signupItems = signupItemAnswers(items, ticked)
    mutation.mutate({
      path: { token },
      body: {
        name: values.name.trim(),
        password: values.password,
        ...(signupItems && { signupItems }),
      },
    })
  }

  return (
    <CenteredLayout maw={420}>
      <Title order={3}>{t(($) => $.acceptInvitation.title)}</Title>
      <Text>{t(($) => $.acceptInvitation.intro, { workspace: invite.workspaceName })}</Text>
      <Text c="dimmed" size="sm">
        {invite.email}
      </Text>

      {signinError ? (
        <Alert color="red" title={t(($) => $.signin.errorTitle)}>
          {t(($) => $.signin.errors[signinError])}
        </Alert>
      ) : null}

      <SigninProviders invitation={token} />

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
          <SignupItemFields
            items={items}
            ticked={ticked}
            onTick={(key, checked) => setTicked((current) => ({ ...current, [key]: checked }))}
            error={mutation.error}
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
        <ButtonLink variant="default" to={loginRoute.to} search={{ redirect: location.href }}>
          {t(($) => $.acceptInvitation.signInButton)}
        </ButtonLink>
      )}
    </CenteredLayout>
  )
}
