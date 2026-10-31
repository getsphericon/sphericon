import {
  Badge,
  Button,
  Divider,
  Group,
  PasswordInput,
  Select,
  Stack,
  Text,
  TextInput,
  Title,
} from '@mantine/core'
import { useForm } from '@mantine/form'
import { modals } from '@mantine/modals'
import { useQueryClient, useSuspenseQuery } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { QueryBoundary } from '../../components/QueryBoundary.tsx'
import { BlockSkeleton } from '../../components/Skeletons.tsx'
import {
  siteUserEmailChangeMutation,
  siteUserGetMeOptions,
  siteUserGetMeQueryKey,
  siteUserSignOutEverywhereMutation,
  siteUserUpdateMeMutation,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import type { SiteEmailChangeInput, SiteUserResource } from '../../generated/site/types.gen.ts'
import { useActionModal } from '../../hooks/useActionModal.tsx'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'
import { Route as loginRoute } from '../../routes/login.tsx'
import { browserTimeZone, TIME_ZONES } from '../../utils/datetime.ts'
import { DeleteUserSection } from './DeleteUserForm.tsx'
import { OptionalConsentsSection } from './OptionalConsentsSection.tsx'

// ProfileForm is split out so the Mantine form seeds its initial values from the
// loaded user without an effect — the parent only renders it once data is ready. An empty time
// zone follows the browser (ADR 0068).
function ProfileForm({ user }: { user: SiteUserResource }) {
  const { t } = useTranslation()

  const form = useForm<{ name: string; timezone: string | null }>({
    initialValues: { name: user.name, timezone: user.timezone },
  })

  const updateMutation = useResourceMutation({
    mutation: siteUserUpdateMeMutation(),
    invalidate: [siteUserGetMeQueryKey()],
    successMessage: t(($) => $.profile.updated),
    errorTitle: t(($) => $.profile.errorTitle),
  })

  return (
    <form
      onSubmit={form.onSubmit((values) =>
        updateMutation.mutate({ body: { name: values.name.trim(), timezone: values.timezone } }),
      )}
    >
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
        <Select
          label={t(($) => $.profile.timezoneLabel)}
          description={t(($) => $.profile.timezoneDescription)}
          placeholder={t(($) => $.profile.timezoneBrowser, { zone: browserTimeZone() })}
          data={TIME_ZONES}
          searchable
          clearable
          {...form.getInputProps('timezone')}
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

// PasswordForm changes the password: the current one proves it is the User.
function PasswordForm({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation()
  const form = useForm({ initialValues: { currentPassword: '', newPassword: '' } })

  const updateMutation = useResourceMutation({
    mutation: siteUserUpdateMeMutation(),
    successMessage: t(($) => $.profile.passwordChanged),
    errorTitle: t(($) => $.profile.passwordErrorTitle),
    onDone: onClose,
  })

  return (
    <form onSubmit={form.onSubmit((values) => updateMutation.mutate({ body: values }))}>
      <Stack>
        <PasswordInput
          label={t(($) => $.profile.currentPasswordLabel)}
          required
          data-autofocus
          {...form.getInputProps('currentPassword')}
        />
        <PasswordInput
          label={t(($) => $.profile.newPasswordLabel)}
          required
          {...form.getInputProps('newPassword')}
        />
        <Group justify="flex-end">
          <Button variant="default" onClick={onClose}>
            {t(($) => $.actions.cancel)}
          </Button>
          <Button type="submit" loading={updateMutation.isPending}>
            {t(($) => $.profile.passwordSectionTitle)}
          </Button>
        </Group>
      </Stack>
    </form>
  )
}

// EmailForm requests an email change; the new address must confirm via link.
function EmailForm({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation()
  const form = useForm({ initialValues: { newEmail: '', currentPassword: '' } })

  const changeMutation = useResourceMutation({
    mutation: siteUserEmailChangeMutation(),
    successMessage: t(($) => $.profile.changeEmailSuccess),
    errorTitle: t(($) => $.profile.changeEmailErrorTitle),
    onDone: onClose,
  })

  const handleSubmit = (values: SiteEmailChangeInput) => {
    changeMutation.mutate({
      body: { newEmail: values.newEmail.trim(), currentPassword: values.currentPassword },
    })
  }

  return (
    <form onSubmit={form.onSubmit(handleSubmit)}>
      <Stack>
        <TextInput
          label={t(($) => $.profile.newEmailLabel)}
          type="email"
          required
          data-autofocus
          {...form.getInputProps('newEmail')}
        />
        <PasswordInput
          label={t(($) => $.profile.currentPasswordForEmailLabel)}
          required
          {...form.getInputProps('currentPassword')}
        />
        <Group justify="flex-end">
          <Button variant="default" onClick={onClose}>
            {t(($) => $.actions.cancel)}
          </Button>
          <Button type="submit" loading={changeMutation.isPending}>
            {t(($) => $.profile.changeEmailButton)}
          </Button>
        </Group>
      </Stack>
    </form>
  )
}

// SignInSection opens the change of the email and of the password, each in a modal.
function SignInSection() {
  const { t } = useTranslation()
  const email = useActionModal(
    t(($) => $.profile.emailSectionTitle),
    (close) => <EmailForm onClose={close} />,
  )
  const password = useActionModal(
    t(($) => $.profile.passwordSectionTitle),
    (close) => <PasswordForm onClose={close} />,
  )
  return (
    <Stack maw={420}>
      <Divider label={t(($) => $.profile.signInSectionTitle)} />
      <Group justify="flex-end">
        <Button variant="default" onClick={email.open}>
          {t(($) => $.profile.emailSectionTitle)}
        </Button>
        <Button variant="default" onClick={password.open}>
          {t(($) => $.profile.passwordSectionTitle)}
        </Button>
      </Group>
      {email.modal}
      {password.modal}
    </Stack>
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

  return (
    <Stack>
      <Title order={2}>{t(($) => $.profile.title)}</Title>

      <QueryBoundary fallback={<BlockSkeleton />} errorTitle={t(($) => $.profile.loadErrorTitle)}>
        <Profile />
      </QueryBoundary>
    </Stack>
  )
}

// Profile is every section of the page, shown once the User loads.
function Profile() {
  const user = useSuspenseQuery(siteUserGetMeOptions()).data

  return (
    <Stack>
      <ProfileForm user={user} />
      <SignInSection />
      <OptionalConsentsSection />
      <SessionsSection />
      <DeleteUserSection />
    </Stack>
  )
}
