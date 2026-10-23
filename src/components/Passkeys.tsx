import { Alert, Button, Group, PasswordInput, Stack, Text, TextInput, Title } from '@mantine/core'
import { useForm } from '@mantine/form'
import { startRegistration } from '@simplewebauthn/browser'
import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
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

// The User's Passkeys (ADR 0052): the list with its names and last use, and the form that adds
// one. Adding asks for the password, or for a fresh sign-in with a provider when the User has
// none; a successful add ends the User's other sessions.
export function Passkeys({ returnTo }: { returnTo: string }) {
  const { t } = useTranslation()
  const passkeys = useQuery(sitePasskeysListOptions())
  const [adding, setAdding] = useState(false)
  const form = useForm({ initialValues: { name: '', currentPassword: '' } })
  const add = useResourceMutation({
    mutation: {
      mutationFn: async (input: AddPasskey) =>
        addPasskey(input, passkeys.data?.passwordSet === false),
    },
    invalidate: [sitePasskeysListQueryKey()],
    successMessage: t(($) => $.security.passkeys.added),
    errorTitle: t(($) => $.security.passkeys.addErrorTitle),
    onDone: () => {
      form.reset()
      setAdding(false)
    },
  })

  const data = passkeys.data
  const passwordless = data?.passwordSet === false
  const needsProvider = passwordless && !data.identityProofFresh

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
      {data && !adding ? (
        <Group justify="flex-end">
          <Button variant="default" onClick={() => setAdding(true)}>
            {t(($) => $.security.passkeys.addButton)}
          </Button>
        </Group>
      ) : null}
      {adding && needsProvider ? (
        <IdentityProof hint={t(($) => $.security.passkeys.providerLoginHint)} returnTo={returnTo} />
      ) : null}
      {adding && !needsProvider ? (
        <form onSubmit={form.onSubmit((v) => add.mutate(v))}>
          <Stack>
            <TextInput
              label={t(($) => $.security.passkeys.nameLabel)}
              required
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
              <Button variant="subtle" onClick={() => setAdding(false)}>
                {t(($) => $.security.passkeys.cancel)}
              </Button>
              <Button type="submit" loading={add.isPending}>
                {t(($) => $.security.passkeys.continue)}
              </Button>
            </Group>
          </Stack>
        </form>
      ) : null}
    </Stack>
  )
}

interface PasskeyRowProps {
  passkey: SitePasskey
  passwordless: boolean
  needsProvider: boolean
  returnTo: string
}

// One Passkey of the list: its name and last use, with a rename and a remove. Removing asks for
// the same proof as adding (the password, or a fresh provider login) and ends the User's other
// sessions (ADR 0052).
function PasskeyRow({ passkey, passwordless, needsProvider, returnTo }: PasskeyRowProps) {
  const { t } = useTranslation()
  const [mode, setMode] = useState<'view' | 'rename' | 'remove'>('view')
  const renameForm = useForm({ initialValues: { name: passkey.name } })
  const removeForm = useForm({ initialValues: { currentPassword: '' } })
  const rename = useResourceMutation({
    mutation: {
      mutationFn: async (name: string) =>
        sitePasskeysRename({ path: { id: passkey.id }, body: { name }, throwOnError: true }),
    },
    invalidate: [sitePasskeysListQueryKey()],
    successMessage: t(($) => $.security.passkeys.renamed),
    errorTitle: t(($) => $.security.passkeys.renameErrorTitle),
    onDone: () => setMode('view'),
  })
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
    onDone: () => setMode('view'),
  })

  if (mode === 'rename') {
    return (
      <form onSubmit={renameForm.onSubmit((v) => rename.mutate(v.name))}>
        <Group align="flex-end">
          <TextInput
            label={t(($) => $.security.passkeys.renameLabel)}
            required
            maxLength={zPasskeyName.maxLength ?? undefined}
            {...renameForm.getInputProps('name')}
          />
          <Button variant="subtle" onClick={() => setMode('view')}>
            {t(($) => $.security.passkeys.cancel)}
          </Button>
          <Button type="submit" loading={rename.isPending}>
            {t(($) => $.security.passkeys.save)}
          </Button>
        </Group>
      </form>
    )
  }

  if (mode === 'remove') {
    return (
      <Stack>
        <Text>{t(($) => $.security.passkeys.removeConfirm, { name: passkey.name })}</Text>
        {needsProvider ? (
          <IdentityProof
            hint={t(($) => $.security.passkeys.removeProviderLoginHint)}
            returnTo={returnTo}
          />
        ) : (
          <form onSubmit={removeForm.onSubmit((v) => remove.mutate(v.currentPassword))}>
            <Stack>
              {passwordless ? null : (
                <PasswordInput
                  label={t(($) => $.security.passwordLabel)}
                  required
                  {...removeForm.getInputProps('currentPassword')}
                />
              )}
              <Group justify="flex-end">
                <Button variant="subtle" onClick={() => setMode('view')}>
                  {t(($) => $.security.passkeys.cancel)}
                </Button>
                <Button type="submit" color="red" loading={remove.isPending}>
                  {t(($) => $.security.passkeys.removeButton)}
                </Button>
              </Group>
            </Stack>
          </form>
        )}
      </Stack>
    )
  }

  return (
    <Group justify="space-between">
      <Stack gap={0}>
        <Text>{passkey.name}</Text>
        <Text size="sm" c="dimmed">
          {passkey.lastUsedAt
            ? t(($) => $.security.passkeys.lastUsed, {
                date: new Date(passkey.lastUsedAt).toLocaleDateString(),
              })
            : t(($) => $.security.passkeys.neverUsed)}
        </Text>
      </Stack>
      <Group>
        <Button variant="subtle" onClick={() => setMode('rename')}>
          {t(($) => $.security.passkeys.renameButton)}
        </Button>
        <Button variant="subtle" color="red" onClick={() => setMode('remove')}>
          {t(($) => $.security.passkeys.removeButton)}
        </Button>
      </Group>
    </Group>
  )
}
