import {
  Alert,
  Badge,
  Button,
  Code,
  CopyButton,
  Divider,
  Group,
  Image,
  Loader,
  PasswordInput,
  SimpleGrid,
  Stack,
  Text,
  TextInput,
  Title,
} from '@mantine/core'
import type { PasswordInputProps } from '@mantine/core'
import { useForm } from '@mantine/form'
import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { IdentityProof } from '../../components/IdentityProof.tsx'
import { LinkedIdentities } from '../../components/LinkedIdentities.tsx'
import { Passkeys } from '../../components/Passkeys.tsx'
import {
  siteSecondFactorConfirmEnrollmentMutation,
  siteSecondFactorDisableMutation,
  siteSecondFactorGetStatusOptions,
  siteSecondFactorGetStatusQueryKey,
  siteSecondFactorRegenerateRecoveryCodesMutation,
  siteSecondFactorStartEnrollmentMutation,
  siteWorkspacesListQueryKey,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import type {
  SiteSecondFactorEnrollment,
  SiteSecondFactorStatus,
} from '../../generated/site/types.gen.ts'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'
import { Route as securityRoute } from '../../routes/account/security.tsx'

// The proof a step carries: the password, or nothing for a User who has none (the server then
// reads the fresh provider login from the session, ADR 0048).
interface Proof {
  currentPassword?: string
}

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
    invalidate: [siteSecondFactorGetStatusQueryKey(), siteWorkspacesListQueryKey()],
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
        <TextInput
          label={t(($) => $.security.codeLabel)}
          inputMode="numeric"
          autoComplete="one-time-code"
          required
          {...form.getInputProps('code')}
        />
        <Group justify="flex-end">
          <Button type="submit" loading={confirm.isPending}>
            {t(($) => $.security.confirmButton)}
          </Button>
        </Group>
      </Stack>
    </form>
  )
}

// Disabled is the state without a Second factor: start enrolling, proving the
// password (both enrollment steps require it, so a hijacked session cannot enroll).
function Disabled({
  passwordless,
  onCodes,
}: {
  passwordless: boolean
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
        <Text size="sm" c="dimmed">
          {t(($) => $.security.offDescription)}
        </Text>
        <PasswordField passwordless={passwordless} {...form.getInputProps('currentPassword')} />
        <Group justify="flex-end">
          <Button type="submit" loading={start.isPending}>
            {t(($) => $.security.setUpButton)}
          </Button>
        </Group>
      </Stack>
    </form>
  )
}

// Enabled is the state with a Second factor: regenerate the Recovery codes or
// disable it.
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
  const regenerateForm = useForm({ initialValues: { currentPassword: '' } })
  const disableForm = useForm({ initialValues: { currentPassword: '', code: '' } })

  const regenerate = useResourceMutation({
    mutation: siteSecondFactorRegenerateRecoveryCodesMutation(),
    invalidate: [siteSecondFactorGetStatusQueryKey()],
    errorTitle: t(($) => $.security.regenerateErrorTitle),
    onDone: (data) => {
      regenerateForm.reset()
      onCodes(data.codes)
    },
  })
  const disable = useResourceMutation({
    mutation: siteSecondFactorDisableMutation(),
    invalidate: [siteSecondFactorGetStatusQueryKey(), siteWorkspacesListQueryKey()],
    successMessage: t(($) => $.security.disabledMessage),
    errorTitle: t(($) => $.security.disableErrorTitle),
  })

  return (
    <Stack>
      <Text size="sm">
        {t(($) => $.security.codesRemaining, { count: status.recoveryCodesRemaining })}
      </Text>

      <form
        onSubmit={regenerateForm.onSubmit((v) =>
          regenerate.mutate({ body: proofOf(passwordless, v.currentPassword) }),
        )}
      >
        <Stack>
          <Divider label={t(($) => $.security.regenerateTitle)} />
          <Text size="sm" c="dimmed">
            {t(($) => $.security.regenerateDescription)}
          </Text>
          <PasswordField
            passwordless={passwordless}
            {...regenerateForm.getInputProps('currentPassword')}
          />
          <Group justify="flex-end">
            <Button type="submit" variant="light" loading={regenerate.isPending}>
              {t(($) => $.security.regenerateButton)}
            </Button>
          </Group>
        </Stack>
      </form>

      <form
        onSubmit={disableForm.onSubmit((v) =>
          disable.mutate({
            body: { ...proofOf(passwordless, v.currentPassword), code: v.code.trim() },
          }),
        )}
      >
        <Stack>
          <Divider label={t(($) => $.security.disableTitle)} />
          <PasswordField
            passwordless={passwordless}
            {...disableForm.getInputProps('currentPassword')}
          />
          <TextInput
            label={t(($) => $.security.disableCodeLabel)}
            autoComplete="one-time-code"
            required
            {...disableForm.getInputProps('code')}
          />
          <Group justify="flex-end">
            <Button type="submit" color="red" variant="light" loading={disable.isPending}>
              {t(($) => $.security.disableButton)}
            </Button>
          </Group>
        </Stack>
      </form>
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

// SecurityPage is the User's Second factor (ADR 0020): enroll an authenticator
// app, see the Recovery codes once, regenerate them, or disable the factor.
export function SecurityPage() {
  const { t } = useTranslation()
  const statusQuery = useQuery(siteSecondFactorGetStatusOptions())
  const [codes, setCodes] = useState<string[] | null>(null)

  return (
    <Stack maw={520}>
      <Title order={2}>{t(($) => $.security.title)}</Title>
      <Group justify="space-between">
        <Title order={4}>{t(($) => $.security.twoFactorTitle)}</Title>
        {statusQuery.data ? (
          <Badge color={statusQuery.data.enabled ? 'teal' : 'gray'} variant="light">
            {statusQuery.data.enabled ? t(($) => $.security.on) : t(($) => $.security.off)}
          </Badge>
        ) : null}
      </Group>

      {statusQuery.isError ? (
        <Alert color="red" title={t(($) => $.security.loadErrorTitle)} />
      ) : null}
      {statusQuery.isLoading ? <Loader /> : null}

      {codes ? <RecoveryCodes codes={codes} onDone={() => setCodes(null)} /> : null}

      {statusQuery.data && !codes ? (
        <SecurityBody status={statusQuery.data} onCodes={setCodes} />
      ) : null}

      <Passkeys returnTo={securityRoute.to} />

      <LinkedIdentities />
    </Stack>
  )
}
