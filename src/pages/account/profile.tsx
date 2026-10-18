import {
  Alert,
  Badge,
  Button,
  Divider,
  Group,
  Loader,
  PasswordInput,
  Stack,
  Text,
  TextInput,
  Title,
} from '@mantine/core'
import { useForm } from '@mantine/form'
import { modals } from '@mantine/modals'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import {
  siteUserEmailChangeMutation,
  siteUserGetMeOptions,
  siteUserGetMeQueryKey,
  siteUserSignOutEverywhereMutation,
  siteUserUpdateMeMutation,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import type {
  SiteEmailChangeInput,
  SiteUpdateMeInput,
  SiteUserResource,
} from '../../generated/site/types.gen.ts'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'
import { Route as loginRoute } from '../../routes/login.tsx'

type ProfileFormValues = {
  name: string
  currentPassword: string
  newPassword: string
}

// ProfileForm is split out so the Mantine form seeds its initial values from the
// loaded user without an effect — the parent only renders it once data is ready.
function ProfileForm({ user }: { user: SiteUserResource }) {
  const { t } = useTranslation()

  const form = useForm<ProfileFormValues>({
    initialValues: { name: user.name, currentPassword: '', newPassword: '' },
  })

  const updateMutation = useResourceMutation({
    mutation: siteUserUpdateMeMutation(),
    invalidate: [siteUserGetMeQueryKey()],
    successMessage: t(($) => $.profile.updated),
    errorTitle: t(($) => $.profile.errorTitle),
    onDone: () => {
      form.setFieldValue('currentPassword', '')
      form.setFieldValue('newPassword', '')
    },
  })

  const handleSubmit = (values: ProfileFormValues) => {
    const body: SiteUpdateMeInput = { name: values.name.trim() }
    if (values.newPassword) {
      body.currentPassword = values.currentPassword
      body.newPassword = values.newPassword
    }
    updateMutation.mutate({ body })
  }

  return (
    <form onSubmit={form.onSubmit(handleSubmit)}>
      <Stack maw={420}>
        <TextInput label={t(($) => $.profile.nameLabel)} required {...form.getInputProps('name')} />
        <TextInput
          label={t(($) => $.profile.emailLabel)}
          value={user.email}
          disabled
          rightSectionWidth={110}
          rightSection={
            <Badge color={user.emailVerified ? 'teal' : 'gray'} variant="light">
              {user.emailVerified
                ? t(($) => $.profile.verifiedBadge)
                : t(($) => $.profile.unverifiedBadge)}
            </Badge>
          }
        />

        <Divider label={t(($) => $.profile.passwordSectionTitle)} />
        <PasswordInput
          label={t(($) => $.profile.currentPasswordLabel)}
          {...form.getInputProps('currentPassword')}
        />
        <PasswordInput
          label={t(($) => $.profile.newPasswordLabel)}
          {...form.getInputProps('newPassword')}
        />

        <Group justify="flex-end">
          <Button type="submit" loading={updateMutation.isPending}>
            {t(($) => $.actions.save)}
          </Button>
        </Group>
      </Stack>
    </form>
  )
}

// EmailSection requests an email change; the new address must confirm via link.
function EmailSection() {
  const { t } = useTranslation()
  const form = useForm({ initialValues: { newEmail: '', currentPassword: '' } })

  const changeMutation = useResourceMutation({
    mutation: siteUserEmailChangeMutation(),
    successMessage: t(($) => $.profile.changeEmailSuccess),
    errorTitle: t(($) => $.profile.changeEmailErrorTitle),
    onDone: () => form.reset(),
  })

  const handleSubmit = (values: SiteEmailChangeInput) => {
    changeMutation.mutate({
      body: { newEmail: values.newEmail.trim(), currentPassword: values.currentPassword },
    })
  }

  return (
    <form onSubmit={form.onSubmit(handleSubmit)}>
      <Stack maw={420}>
        <Divider label={t(($) => $.profile.emailSectionTitle)} />
        <TextInput
          label={t(($) => $.profile.newEmailLabel)}
          type="email"
          required
          {...form.getInputProps('newEmail')}
        />
        <PasswordInput
          label={t(($) => $.profile.currentPasswordForEmailLabel)}
          required
          {...form.getInputProps('currentPassword')}
        />
        <Group justify="flex-end">
          <Button type="submit" loading={changeMutation.isPending}>
            {t(($) => $.profile.changeEmailButton)}
          </Button>
        </Group>
      </Stack>
    </form>
  )
}

// SessionsSection ends every session of the user, this browser's included, so
// it asks first and lands on the login screen.
function SessionsSection() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()

  const signOut = useResourceMutation({
    mutation: siteUserSignOutEverywhereMutation(),
    errorTitle: t(($) => $.profile.signOutEverywhereErrorTitle),
    onDone: async () => {
      queryClient.clear()
      await navigate({ to: loginRoute.to })
    },
  })

  const confirm = () => {
    modals.openConfirmModal({
      title: t(($) => $.profile.signOutEverywhereTitle),
      children: <Text>{t(($) => $.profile.signOutEverywhereConfirm)}</Text>,
      labels: {
        cancel: t(($) => $.actions.cancel),
        confirm: t(($) => $.profile.signOutEverywhereButton),
      },
      confirmProps: { color: 'red' },
      onConfirm: () => signOut.mutate({}),
    })
  }

  return (
    <Stack maw={420}>
      <Divider label={t(($) => $.profile.sessionsSectionTitle)} />
      <Text size="sm" c="dimmed">
        {t(($) => $.profile.signOutEverywhereDescription)}
      </Text>
      <Group justify="flex-end">
        <Button color="red" variant="light" loading={signOut.isPending} onClick={confirm}>
          {t(($) => $.profile.signOutEverywhereButton)}
        </Button>
      </Group>
    </Stack>
  )
}

export function ProfilePage() {
  const { t } = useTranslation()
  const meQuery = useQuery(siteUserGetMeOptions())

  return (
    <Stack>
      <Title order={2}>{t(($) => $.profile.title)}</Title>

      {meQuery.isError ? (
        <Alert color="red" title={t(($) => $.profile.loadErrorTitle)}>
          {t(($) => $.profile.loadErrorTitle)}
        </Alert>
      ) : null}

      {meQuery.isLoading ? <Loader /> : null}

      {meQuery.data ? (
        <Stack>
          <ProfileForm user={meQuery.data} />
          <EmailSection />
          <SessionsSection />
        </Stack>
      ) : null}
    </Stack>
  )
}
