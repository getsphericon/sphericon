import {
  Alert,
  Badge,
  Button,
  Code,
  CopyButton,
  Group,
  Image,
  PasswordInput,
  SimpleGrid,
  Stack,
  Text,
  Title,
} from '@mantine/core'
import type { PasswordInputProps } from '@mantine/core'
import { useForm } from '@mantine/form'
import { useSuspenseQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { IdentityProof } from '../../components/IdentityProof.tsx'
import { LinkedIdentities } from '../../components/LinkedIdentities.tsx'
import { OneTimeCodeInput } from '../../components/OneTimeCodeInput.tsx'
import { Passkeys } from '../../components/Passkeys.tsx'
import { QueryBoundary } from '../../components/QueryBoundary.tsx'
import { BlockSkeleton } from '../../components/Skeletons.tsx'
import {
  siteSecondFactorConfirmEnrollmentMutation,
  siteSecondFactorDisableMutation,
  siteSecondFactorGetStatusOptions,
  siteSecondFactorRegenerateRecoveryCodesMutation,
  siteSecondFactorStartEnrollmentMutation,
  siteWorkspacesListQueryKey,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import type {
  SiteSecondFactorEnrollment,
  SiteSecondFactorStartInput,
  SiteSecondFactorStatus,
} from '../../generated/site/types.gen.ts'
import { useActionModal } from '../../hooks/useActionModal.tsx'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'
import { Route as securityRoute } from '../../routes/account/security.tsx'

// The proof a step carries: the password, or nothing for a User who has none (the server then
// reads the fresh provider login from the session, ADR 0048).
type Proof = SiteSecondFactorStartInput

// The form field of a password, shown only to a User who has one.
function PasswordField({ passwordless, ...props }: { passwordless: boolean } & PasswordInputProps) {
  const { t } = useTranslation()
  return passwordless ? null : (
    <PasswordInput label={t(($) => $.security.passwordLabel)} required {...props} />
  )
}

const proofOf = (passwordless: boolean, currentPassword: string): Proof =>
  passwordless ? {} : { currentPassword }

// RecoveryCodes shows a fresh set once: the server never returns them again.
function RecoveryCodes({ codes, onDone }: { codes: string[]; onDone: () => void }) {
  const { t } = useTranslation()
  return (
    <Alert color="yellow" title={t(($) => $.security.recoveryCodesTitle)}>
      <Stack>
        <Text size="sm">{t(($) => $.security.recoveryCodesHint)}</Text>
        <SimpleGrid cols={{ base: 1, xs: 2 }} spacing="xs">
          {codes.map((code) => (
            <Code key={code}>{code}</Code>
          ))}
        </SimpleGrid>
        <Group justify="flex-end">
          <CopyButton value={codes.join('\n')}>
            {({ copied, copy }) => (
              <Button variant="light" onClick={copy}>
                {copied ? t(($) => $.security.copied) : t(($) => $.security.copyCodes)}
              </Button>
            )}
          </CopyButton>
          <Button onClick={onDone}>{t(($) => $.security.savedCodes)}</Button>
        </Group>
      </Stack>
    </Alert>
  )
}

// Enrollment shows the pending secret (QR code and key) and confirms it with a
// code from the authenticator app and the proof given when it started.
function Enrollment({
  enrollment,
  proof,
  onConfirmed,
}: {
  enrollment: SiteSecondFactorEnrollment
  proof: Proof
  onConfirmed: (codes: string[]) => void
}) {
  const { t } = useTranslation()
  const form = useForm({ initialValues: { code: '' } })
  const confirm = useResourceMutation({
    mutation: siteSecondFactorConfirmEnrollmentMutation(),
    // The Workspaces carry the grace deadline of a Two-factor requirement, which
    // a confirmed Second factor lifts.
    invalidate: [siteWorkspacesListQueryKey()],
    successMessage: t(($) => $.security.enabledMessage),
    errorTitle: t(($) => $.security.confirmErrorTitle),
    onDone: (data) => onConfirmed(data.codes),
  })

  return (
    <form
      onSubmit={form.onSubmit((v) => confirm.mutate({ body: { ...proof, code: v.code.trim() } }))}
    >
      <Stack>
        <Text size="sm">{t(($) => $.security.scanHint)}</Text>
        <Image src={enrollment.qrCode} alt={t(($) => $.security.qrAlt)} w={200} h={200} />
        <Text size="sm">{t(($) => $.security.keyHint)}</Text>
        <Group gap="xs">
          <Code>{enrollment.secret}</Code>
          <CopyButton value={enrollment.secret}>
            {({ copied, copy }) => (
              <Button size="xs" variant="subtle" onClick={copy}>
                {copied ? t(($) => $.security.copied) : t(($) => $.security.copyKey)}
              </Button>
            )}
          </CopyButton>
        </Group>
        <OneTimeCodeInput label={t(($) => $.security.codeLabel)} {...form.getInputProps('code')} />
        <Group justify="flex-end">
          <Button type="submit" loading={confirm.isPending}>
            {t(($) => $.security.confirmButton)}
          </Button>
        </Group>
      </Stack>
    </form>
  )
}

// SetUp enrolls the Second factor inside its modal: the proof first (the password, or nothing
// for a User with no password), then the pending secret to scan and confirm. Both enrollment
// steps require the proof, so a hijacked session cannot enroll.
function SetUp({
  passwordless,
  onCancel,
  onCodes,
}: {
  passwordless: boolean
  onCancel: () => void
  onCodes: (codes: string[]) => void
}) {
  const { t } = useTranslation()
  const form = useForm({ initialValues: { currentPassword: '' } })
  const [started, setStarted] = useState<{
    enrollment: SiteSecondFactorEnrollment
    proof: Proof
  } | null>(null)
  const start = useResourceMutation({
    mutation: siteSecondFactorStartEnrollmentMutation(),
    errorTitle: t(($) => $.security.startErrorTitle),
    onDone: (data, variables) => setStarted({ enrollment: data, proof: variables.body }),
  })

  if (started) {
    return (
      <Enrollment enrollment={started.enrollment} proof={started.proof} onConfirmed={onCodes} />
    )
  }
  return (
    <form
      onSubmit={form.onSubmit((v) =>
        start.mutate({ body: proofOf(passwordless, v.currentPassword) }),
      )}
    >
      <Stack>
        <PasswordField
          passwordless={passwordless}
          data-autofocus
          {...form.getInputProps('currentPassword')}
        />
        <Group justify="flex-end">
          <Button variant="default" onClick={onCancel}>
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

// Disabled is the state without a Second factor: the setup opens in a modal.
function Disabled({
  passwordless,
  onCodes,
}: {
  passwordless: boolean
  onCodes: (codes: string[]) => void
}) {
  const { t } = useTranslation()
  const setUp = useActionModal(
    t(($) => $.security.setUpButton),
    (close) => (
      <SetUp
        passwordless={passwordless}
        onCancel={close}
        onCodes={(codes) => {
          close()
          onCodes(codes)
        }}
      />
    ),
  )
  return (
    <Stack>
      <Text size="sm" c="dimmed">
        {t(($) => $.security.offDescription)}
      </Text>
      <Group justify="flex-end">
        <Button onClick={setUp.open}>{t(($) => $.security.setUpButton)}</Button>
      </Group>
      {setUp.modal}
    </Stack>
  )
}

// Regenerate replaces the Recovery codes once the proof is given; the fresh set goes to the page.
function Regenerate({
  passwordless,
  onCancel,
  onCodes,
}: {
  passwordless: boolean
  onCancel: () => void
  onCodes: (codes: string[]) => void
}) {
  const { t } = useTranslation()
  const form = useForm({ initialValues: { currentPassword: '' } })
  const regenerate = useResourceMutation({
    mutation: siteSecondFactorRegenerateRecoveryCodesMutation(),
    errorTitle: t(($) => $.security.regenerateErrorTitle),
    onDone: (data) => onCodes(data.codes),
  })

  return (
    <form
      onSubmit={form.onSubmit((v) =>
        regenerate.mutate({ body: proofOf(passwordless, v.currentPassword) }),
      )}
    >
      <Stack>
        <Text size="sm">{t(($) => $.security.regenerateDescription)}</Text>
        <PasswordField
          passwordless={passwordless}
          data-autofocus
          {...form.getInputProps('currentPassword')}
        />
        <Group justify="flex-end">
          <Button variant="default" onClick={onCancel}>
            {t(($) => $.actions.cancel)}
          </Button>
          <Button type="submit" loading={regenerate.isPending}>
            {t(($) => $.security.regenerateButton)}
          </Button>
        </Group>
      </Stack>
    </form>
  )
}

// Disable turns the Second factor off with the proof and a code.
function Disable({ passwordless, onClose }: { passwordless: boolean; onClose: () => void }) {
  const { t } = useTranslation()
  const form = useForm({ initialValues: { currentPassword: '', code: '' } })
  const disable = useResourceMutation({
    mutation: siteSecondFactorDisableMutation(),
    invalidate: [siteWorkspacesListQueryKey()],
    successMessage: t(($) => $.security.disabledMessage),
    errorTitle: t(($) => $.security.disableErrorTitle),
    onDone: onClose,
  })

  return (
    <form
      onSubmit={form.onSubmit((v) =>
        disable.mutate({
          body: { ...proofOf(passwordless, v.currentPassword), code: v.code.trim() },
        }),
      )}
    >
      <Stack>
        <PasswordField
          passwordless={passwordless}
          data-autofocus
          {...form.getInputProps('currentPassword')}
        />
        <OneTimeCodeInput
          label={t(($) => $.security.codeLabel)}
          recovery
          {...form.getInputProps('code')}
        />
        <Group justify="flex-end">
          <Button variant="default" onClick={onClose}>
            {t(($) => $.actions.cancel)}
          </Button>
          <Button type="submit" color="red" loading={disable.isPending}>
            {t(($) => $.security.disableButton)}
          </Button>
        </Group>
      </Stack>
    </form>
  )
}

// Enabled is the state with a Second factor: regenerate the Recovery codes or disable it, each
// in a modal.
function Enabled({
  status,
  passwordless,
  onCodes,
}: {
  status: SiteSecondFactorStatus
  passwordless: boolean
  onCodes: (codes: string[]) => void
}) {
  const { t } = useTranslation()
  const regenerate = useActionModal(
    t(($) => $.security.regenerateTitle),
    (close) => (
      <Regenerate
        passwordless={passwordless}
        onCancel={close}
        onCodes={(codes) => {
          close()
          onCodes(codes)
        }}
      />
    ),
  )
  const disable = useActionModal(
    t(($) => $.security.disableTitle),
    (close) => <Disable passwordless={passwordless} onClose={close} />,
  )

  return (
    <Stack>
      <Text size="sm">
        {t(($) => $.security.codesRemaining, { count: status.recoveryCodesRemaining })}
      </Text>
      <Group justify="flex-end">
        <Button variant="light" onClick={regenerate.open}>
          {t(($) => $.security.regenerateButton)}
        </Button>
        <Button color="red" variant="light" onClick={disable.open}>
          {t(($) => $.security.disableButton)}
        </Button>
      </Group>
      {regenerate.modal}
      {disable.modal}
    </Stack>
  )
}

// SecurityBody is the state of the Second factor, or, for a User with no password whose provider
// proof is too old to count, the way to a fresh one: a Passkey check or a provider login (ADR 0048, ADR 0052): the steps need that proof.
function SecurityBody({
  status,
  onCodes,
}: {
  status: SiteSecondFactorStatus
  onCodes: (codes: string[]) => void
}) {
  const { t } = useTranslation()
  const passwordless = !status.passwordSet
  if (passwordless && !status.identityProofFresh) {
    return (
      <IdentityProof hint={t(($) => $.security.providerLoginHint)} returnTo={securityRoute.to} />
    )
  }
  return status.enabled ? (
    <Enabled status={status} passwordless={passwordless} onCodes={onCodes} />
  ) : (
    <Disabled passwordless={passwordless} onCodes={onCodes} />
  )
}

// StatusBadge says whether the Second factor is on; it shows nothing until the status loads.
function StatusBadge() {
  const { t } = useTranslation()
  const status = useSuspenseQuery(siteSecondFactorGetStatusOptions()).data
  return (
    <Badge color={status.enabled ? 'teal' : 'gray'} variant="light">
      {status.enabled ? t(($) => $.security.on) : t(($) => $.security.off)}
    </Badge>
  )
}

// SecurityStatus reads the Second factor's status for the body; the page's boundary shows the
// load and its failure.
function SecurityStatus({ onCodes }: { onCodes: (codes: string[]) => void }) {
  const status = useSuspenseQuery(siteSecondFactorGetStatusOptions()).data
  return <SecurityBody status={status} onCodes={onCodes} />
}

// SecurityPage is the User's Second factor (ADR 0020): enroll an authenticator
// app, see the Recovery codes once, regenerate them, or disable the factor.
export function SecurityPage() {
  const { t } = useTranslation()
  const [codes, setCodes] = useState<string[] | null>(null)

  return (
    <Stack maw={420}>
      <Title order={2}>{t(($) => $.security.title)}</Title>
      <Group justify="space-between">
        <Title order={4}>{t(($) => $.security.twoFactorTitle)}</Title>
        {/* The body's boundary below reports the load and its failure once. */}
        <QueryBoundary
          fallback={null}
          errorTitle={t(($) => $.security.loadErrorTitle)}
          renderError={() => null}
        >
          <StatusBadge />
        </QueryBoundary>
      </Group>

      {codes ? <RecoveryCodes codes={codes} onDone={() => setCodes(null)} /> : null}

      {codes ? null : (
        <QueryBoundary
          fallback={<BlockSkeleton />}
          errorTitle={t(($) => $.security.loadErrorTitle)}
        >
          <SecurityStatus onCodes={setCodes} />
        </QueryBoundary>
      )}

      <Passkeys returnTo={securityRoute.to} />

      <LinkedIdentities />
    </Stack>
  )
}
