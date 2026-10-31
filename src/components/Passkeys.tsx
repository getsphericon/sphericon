import { Alert, Button, Group, PasswordInput, Stack, Text, TextInput, Title } from '@mantine/core'
import { useForm } from '@mantine/form'
import { startRegistration } from '@simplewebauthn/browser'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import {
  sitePasskeysListOptions,
  sitePasskeysListQueryKey,
} from '../generated/site/@tanstack/react-query.gen.ts'
import {
  sitePasskeysBeginRegistration,
  sitePasskeysFinishRegistration,
  sitePasskeysRemove,
  sitePasskeysRename,
} from '../generated/site/sdk.gen.ts'
import type { SitePasskey } from '../generated/site/types.gen.ts'
import { zPasskeyName } from '../generated/site/zod.gen.ts'
import { useActionModal } from '../hooks/useActionModal.tsx'
import { useDateFormat } from '../hooks/useDateFormat.ts'
import { useResourceMutation } from '../hooks/useResourceMutation.ts'
import { IdentityProof } from './IdentityProof.tsx'
import { BlockSkeleton } from './Skeletons.tsx'

interface AddPasskey {
  name: string
  currentPassword: string
}

// The browser's side of a registration (ADR 0052): the server's creation options go to the
// authenticator and its answer goes back, with the name and the proof of identity.
async function addPasskey({ name, currentPassword }: AddPasskey, passwordless: boolean) {
  const begun = await sitePasskeysBeginRegistration({ throwOnError: true })
  const credential = await startRegistration({ optionsJSON: begun.data.options })
  return sitePasskeysFinishRegistration({
    body: { name, credential: { ...credential }, ...(passwordless ? {} : { currentPassword }) },
    throwOnError: true,
  })
}

// AddPasskeyForm names the new Passkey and gives the proof (the password, or nothing for a User
// with no password whose provider proof is fresh) before the browser's authenticator answers.
function AddPasskeyForm({ passwordless, onClose }: { passwordless: boolean; onClose: () => void }) {
  const { t } = useTranslation()
  const form = useForm({ initialValues: { name: '', currentPassword: '' } })
  const add = useResourceMutation({
    mutation: {
      mutationFn: async (input: AddPasskey) => addPasskey(input, passwordless),
    },
    invalidate: [sitePasskeysListQueryKey()],
    successMessage: t(($) => $.security.passkeys.added),
    errorTitle: t(($) => $.security.passkeys.addErrorTitle),
    onDone: onClose,
  })

  return (
    <form onSubmit={form.onSubmit((v) => add.mutate(v))}>
      <Stack>
        <TextInput
          label={t(($) => $.security.passkeys.nameLabel)}
          required
          data-autofocus
          maxLength={zPasskeyName.maxLength ?? undefined}
          {...form.getInputProps('name')}
        />
        {passwordless ? null : (
          <PasswordInput
            label={t(($) => $.security.passwordLabel)}
            required
            {...form.getInputProps('currentPassword')}
          />
        )}
        <Group justify="flex-end">
          <Button variant="default" onClick={onClose}>
            {t(($) => $.security.passkeys.cancel)}
          </Button>
          <Button type="submit" loading={add.isPending}>
            {t(($) => $.security.passkeys.continue)}
          </Button>
        </Group>
      </Stack>
    </form>
  )
}

// The User's Passkeys (ADR 0052): the list with its names and last use, and the button that adds
// one in a modal. Adding asks for the password, or for a fresh sign-in with a provider when the
// User has none; a successful add ends the User's other sessions.
export function Passkeys({ returnTo }: { returnTo: string }) {
  const { t } = useTranslation()
  const passkeys = useQuery(sitePasskeysListOptions())

  const data = passkeys.data
  const passwordless = data?.passwordSet === false
  const needsProvider = passwordless && !data.identityProofFresh
  const add = useActionModal(
    t(($) => $.security.passkeys.addButton),
    (close) =>
      needsProvider ? (
        <IdentityProof hint={t(($) => $.security.passkeys.providerLoginHint)} returnTo={returnTo} />
      ) : (
        <AddPasskeyForm passwordless={passwordless} onClose={close} />
      ),
  )

  // An Instance served on an IP address has no host name to bind a Passkey to.
  if (data && !data.available) {
    return null
  }

  return (
    <Stack>
      <Title order={4}>{t(($) => $.security.passkeys.title)}</Title>
      {passkeys.isError ? <Alert color="red" title={t(($) => $.security.loadErrorTitle)} /> : null}
      {passkeys.isLoading ? <BlockSkeleton /> : null}
      {data && data.items.length === 0 ? (
        <Text size="sm" c="dimmed">
          {t(($) => $.security.passkeys.empty)}
        </Text>
      ) : null}
      {data?.items.map((passkey) => (
        <PasskeyRow
          key={passkey.id}
          passkey={passkey}
          passwordless={passwordless}
          needsProvider={needsProvider}
          returnTo={returnTo}
        />
      ))}
      {data ? (
        <Group justify="flex-end">
          <Button variant="default" onClick={add.open}>
            {t(($) => $.security.passkeys.addButton)}
          </Button>
        </Group>
      ) : null}
      {add.modal}
    </Stack>
  )
}

// RenamePasskeyForm gives a Passkey a new name.
function RenamePasskeyForm({ passkey, onClose }: { passkey: SitePasskey; onClose: () => void }) {
  const { t } = useTranslation()
  const form = useForm({ initialValues: { name: passkey.name } })
  const rename = useResourceMutation({
    mutation: {
      mutationFn: async (name: string) =>
        sitePasskeysRename({ path: { id: passkey.id }, body: { name }, throwOnError: true }),
    },
    invalidate: [sitePasskeysListQueryKey()],
    successMessage: t(($) => $.security.passkeys.renamed),
    errorTitle: t(($) => $.security.passkeys.renameErrorTitle),
    onDone: onClose,
  })

  return (
    <form onSubmit={form.onSubmit((v) => rename.mutate(v.name))}>
      <Stack>
        <TextInput
          label={t(($) => $.security.passkeys.renameLabel)}
          required
          data-autofocus
          maxLength={zPasskeyName.maxLength ?? undefined}
          {...form.getInputProps('name')}
        />
        <Group justify="flex-end">
          <Button variant="default" onClick={onClose}>
            {t(($) => $.security.passkeys.cancel)}
          </Button>
          <Button type="submit" loading={rename.isPending}>
            {t(($) => $.security.passkeys.save)}
          </Button>
        </Group>
      </Stack>
    </form>
  )
}

// RemovePasskeyForm removes a Passkey once the proof is given (the password, or nothing for a
// User with no password whose provider proof is fresh).
function RemovePasskeyForm({
  passkey,
  passwordless,
  onClose,
}: {
  passkey: SitePasskey
  passwordless: boolean
  onClose: () => void
}) {
  const { t } = useTranslation()
  const form = useForm({ initialValues: { currentPassword: '' } })
  const remove = useResourceMutation({
    mutation: {
      mutationFn: async (currentPassword: string) =>
        sitePasskeysRemove({
          path: { id: passkey.id },
          body: passwordless ? {} : { currentPassword },
          throwOnError: true,
        }),
    },
    invalidate: [sitePasskeysListQueryKey()],
    successMessage: t(($) => $.security.passkeys.removed),
    errorTitle: t(($) => $.security.passkeys.removeErrorTitle),
    onDone: onClose,
  })

  return (
    <form onSubmit={form.onSubmit((v) => remove.mutate(v.currentPassword))}>
      <Stack>
        {passwordless ? null : (
          <PasswordInput
            label={t(($) => $.security.passwordLabel)}
            required
            data-autofocus
            {...form.getInputProps('currentPassword')}
          />
        )}
        <Group justify="flex-end">
          <Button variant="default" onClick={onClose}>
            {t(($) => $.security.passkeys.cancel)}
          </Button>
          <Button type="submit" color="red" loading={remove.isPending}>
            {t(($) => $.security.passkeys.removeButton)}
          </Button>
        </Group>
      </Stack>
    </form>
  )
}

interface PasskeyRowProps {
  passkey: SitePasskey
  passwordless: boolean
  needsProvider: boolean
  returnTo: string
}

// One Passkey of the list: its name and last use, with a rename and a remove, each in a modal.
// Removing asks for the same proof as adding (the password, or a fresh provider login) and ends
// the User's other sessions (ADR 0052).
function PasskeyRow({ passkey, passwordless, needsProvider, returnTo }: PasskeyRowProps) {
  const { t } = useTranslation()
  const { formatDate } = useDateFormat()
  const rename = useActionModal(
    t(($) => $.security.passkeys.renameButton),
    (close) => <RenamePasskeyForm passkey={passkey} onClose={close} />,
  )
  const remove = useActionModal(
    t(($) => $.security.passkeys.removeButton),
    (close) => (
      <Stack>
        <Text>{t(($) => $.security.passkeys.removeConfirm, { name: passkey.name })}</Text>
        {needsProvider ? (
          <IdentityProof
            hint={t(($) => $.security.passkeys.removeProviderLoginHint)}
            returnTo={returnTo}
          />
        ) : (
          <RemovePasskeyForm passkey={passkey} passwordless={passwordless} onClose={close} />
        )}
      </Stack>
    ),
  )

  return (
    <>
      <Group justify="space-between">
        <Stack gap={0}>
          <Text>{passkey.name}</Text>
          <Text size="sm" c="dimmed">
            {passkey.lastUsedAt
              ? t(($) => $.security.passkeys.lastUsed, {
                  date: formatDate(passkey.lastUsedAt),
                })
              : t(($) => $.security.passkeys.neverUsed)}
          </Text>
        </Stack>
        <Group>
          <Button variant="subtle" onClick={rename.open}>
            {t(($) => $.security.passkeys.renameButton)}
          </Button>
          <Button variant="subtle" color="red" onClick={remove.open}>
            {t(($) => $.security.passkeys.removeButton)}
          </Button>
        </Group>
      </Group>
      {rename.modal}
      {remove.modal}
    </>
  )
}
