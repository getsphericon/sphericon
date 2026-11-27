import { Alert, Button, Group, Stack, Text, Title } from '@mantine/core'
import { useForm } from '@mantine/form'
import { useSuspenseQueries } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import {
  siteAuthSigninProvidersOptions,
  siteIdentitiesLinkMutation,
  siteIdentitiesListOptions,
  siteIdentitiesUnlinkMutation,
} from '../generated/site/@tanstack/react-query.gen.ts'
import type {
  SiteLinkedIdentity,
  SiteSigninError,
  SiteSigninProvider,
} from '../generated/site/types.gen.ts'
import { useActionModal } from '../hooks/useActionModal.tsx'
import { useResourceMutation } from '../hooks/useResourceMutation.ts'
import { IdentityProof } from './IdentityProof.tsx'
import { PasswordField } from './PasswordField.tsx'
import { QueryBoundary } from './QueryBoundary.tsx'
import { BlockSkeleton } from './Skeletons.tsx'

// The account's Linked identities (ADR 0048): one row per Sign-in provider the Instance enabled,
// with Connect while the User has no account of it linked and Disconnect once they have, and a
// Disconnect row for an identity whose provider the Instance no longer offers. Connecting asks for
// the same proof as adding a Passkey (ADR 0176): the password, or a fresh provider sign-in or
// Passkey check for a User with none, which lands back on `returnTo`. Nothing is shown on an
// Instance with no provider and an account with no identity, so the title is part of what loads.
interface LinkedIdentitiesProps {
  returnTo: string
  // Why a link just made did not succeed, which the provider's callback put on `returnTo`.
  linkError: SiteSigninError | undefined
}

export function LinkedIdentities({ returnTo, linkError }: LinkedIdentitiesProps) {
  const { t } = useTranslation()
  return (
    <QueryBoundary
      fallback={
        <Stack>
          <Title order={4}>{t(($) => $.signin.linked.title)}</Title>
          <BlockSkeleton />
        </Stack>
      }
      errorTitle={t(($) => $.security.loadErrorTitle)}
    >
      <IdentityList returnTo={returnTo} linkError={linkError} />
    </QueryBoundary>
  )
}

function IdentityList({ returnTo, linkError }: LinkedIdentitiesProps) {
  const { t } = useTranslation()
  // Both lists are fetched at once rather than one after the other.
  const [identities, providers] = useSuspenseQueries({
    queries: [siteIdentitiesListOptions(), siteAuthSigninProvidersOptions()],
  })
  const unlink = useResourceMutation({
    mutation: siteIdentitiesUnlinkMutation(),
    successMessage: t(($) => $.signin.linked.disconnected),
    errorTitle: t(($) => $.signin.linked.disconnectErrorTitle),
  })

  const { items, passwordSet, identityProofFresh } = identities.data
  const enabled = providers.data.items
  if (items.length === 0 && enabled.length === 0) {
    return null
  }
  const proof: LinkProof = {
    passwordless: !passwordSet,
    needsProof: !passwordSet && !identityProofFresh,
    returnTo,
  }

  const disconnect = (identity: SiteLinkedIdentity) => (
    <Group key={identity.id} justify="space-between">
      <Text>{identity.displayName}</Text>
      <Button
        variant="default"
        loading={unlink.isPending && unlink.variables.path.id === identity.id}
        onClick={() => unlink.mutate({ path: { id: identity.id } })}
      >
        {t(($) => $.signin.linked.disconnect)}
      </Button>
    </Group>
  )

  return (
    <Stack>
      <Title order={4}>{t(($) => $.signin.linked.title)}</Title>
      {linkError ? (
        <Alert color="red" title={t(($) => $.signin.linked.errorTitle)}>
          {t(($) => $.signin.errors[linkError])}
        </Alert>
      ) : null}
      {enabled.map((provider) => {
        const linked = items.filter((identity) => identity.provider === provider.name)
        return linked.length > 0 ? (
          linked.map((identity) => disconnect(identity))
        ) : (
          <ConnectRow key={provider.name} provider={provider} proof={proof} />
        )
      })}
      {items
        .filter((identity) => !enabled.some((provider) => provider.name === identity.provider))
        .map((identity) => disconnect(identity))}
    </Stack>
  )
}

// What a link asks of the User: the password (`passwordless` false), nothing more for a User with
// no password whose session holds a fresh proof, or a fresh proof first (`needsProof`), after which
// the browser lands back on `returnTo`.
interface LinkProof {
  passwordless: boolean
  needsProof: boolean
  returnTo: string
}

// One provider the User has no account of linked: Connect opens the proof in a modal.
function ConnectRow({ provider, proof }: { provider: SiteSigninProvider; proof: LinkProof }) {
  const { t } = useTranslation()
  const connect = useActionModal(
    t(($) => $.signin.linked.connectTitle, { provider: provider.displayName }),
    (close) =>
      proof.needsProof ? (
        <IdentityProof
          hint={t(($) => $.signin.linked.providerLoginHint)}
          returnTo={proof.returnTo}
        />
      ) : (
        <ConnectForm provider={provider} passwordless={proof.passwordless} onClose={close} />
      ),
  )
  return (
    <>
      <Group justify="space-between">
        <Text>{provider.displayName}</Text>
        <Button variant="default" onClick={connect.open}>
          {t(($) => $.signin.linked.connect)}
        </Button>
      </Group>
      {connect.modal}
    </>
  )
}

// ConnectForm gives the proof (the password, or nothing for a User with no password whose proof
// is fresh) and starts the link: the server answers the provider's URL, which the browser follows;
// the provider sends it back to the security page.
function ConnectForm({
  provider,
  passwordless,
  onClose,
}: {
  provider: SiteSigninProvider
  passwordless: boolean
  onClose: () => void
}) {
  const { t } = useTranslation()
  const form = useForm({ initialValues: { currentPassword: '' } })
  const start = useResourceMutation({
    mutation: siteIdentitiesLinkMutation(),
    // A link only begins here: nothing changes until the provider sends the browser back.
    invalidate: false,
    errorTitle: t(($) => $.signin.linked.errorTitle),
    onDone: (data) => window.location.assign(data.redirectUrl),
  })

  return (
    <form
      onSubmit={form.onSubmit((v) =>
        start.mutate({
          body: passwordless
            ? { provider: provider.name }
            : { provider: provider.name, currentPassword: v.currentPassword },
        }),
      )}
    >
      <Stack>
        <Text size="sm">
          {t(($) => $.signin.linked.connectDescription, { provider: provider.displayName })}
        </Text>
        {passwordless ? null : (
          <PasswordField
            label={t(($) => $.security.passwordLabel)}
            required
            data-autofocus
            {...form.getInputProps('currentPassword')}
          />
        )}
        <Group justify="flex-end">
          <Button variant="default" onClick={onClose}>
            {t(($) => $.actions.cancel)}
          </Button>
          <Button type="submit" loading={start.isPending}>
            {t(($) => $.actions.continue)}
          </Button>
        </Group>
      </Stack>
    </form>
  )
}
