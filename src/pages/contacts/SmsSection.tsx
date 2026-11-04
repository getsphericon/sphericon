import { Button, Group, Modal, Stack, Table, Text, Textarea, TextInput, Title } from '@mantine/core'
import { useDisclosure } from '@mantine/hooks'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import * as z from 'zod'

import {
  siteSmsConsentsConfirmMutation,
  siteSmsConsentsGetOptions,
  siteSmsConsentsOptOutMutation,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import type { ConfirmationProvenance } from '../../generated/site/types.gen.ts'
import { zConsentEvidence } from '../../generated/site/zod.gen.ts'
import { useConfirmation } from '../../hooks/useConfirmation.tsx'
import { useFormat } from '../../hooks/useFormat.ts'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'
import { trimmed, useSchemaForm } from '../../resources/schemaForm.ts'

interface Props {
  workspaceId: string
  contactId: string
}

// The evidence a member records, each part trimmed and judged by the contract's rules.
const evidenceSchema = z.object({
  wording: trimmed(zConsentEvidence.shape.wording),
  source: trimmed(zConsentEvidence.shape.source),
  brand: trimmed(zConsentEvidence.shape.brand),
})

// Whether the contact's phone may be texted (ADR 0077): a text goes only to a number with an SMS
// Confirmation and no SMS opt-out. The card shows both, keyed by the number, and records what
// reached the Workspace by another route: consent collected elsewhere, with its evidence, and an
// opt-out. A contact without a phone has nothing to show.
export function SmsSection({ workspaceId, contactId }: Props) {
  const { t } = useTranslation()
  const { formatDateTime } = useFormat()
  const confirmOptOut = useConfirmation()
  const [consentOpened, { open: openConsent, close: closeConsent }] = useDisclosure(false)
  const path = { workspaceId, contactId }
  const consent = useQuery(siteSmsConsentsGetOptions({ path }))
  const form = useSchemaForm(evidenceSchema, { wording: '', source: '', brand: '' })

  const provenances: Record<ConfirmationProvenance, string> = {
    double_opt_in: t(($) => $.contacts.smsProvenanceDoubleOptIn),
    keyword: t(($) => $.contacts.smsProvenanceKeyword),
    imported: t(($) => $.contacts.smsProvenanceImported),
    grandfathered: t(($) => $.contacts.smsProvenanceGrandfathered),
  }

  const confirm = useResourceMutation({
    mutation: siteSmsConsentsConfirmMutation(),
    successMessage: t(($) => $.contacts.smsConsentRecorded),
    errorTitle: t(($) => $.contacts.smsConsentError),
    onDone: () => {
      form.reset()
      closeConsent()
    },
  })
  const optOut = useResourceMutation({
    mutation: siteSmsConsentsOptOutMutation(),
    successMessage: t(($) => $.contacts.smsOptOutRecorded),
    errorTitle: t(($) => $.contacts.smsOptOutError),
  })

  const state = consent.data
  if (!state?.phone) {
    return null
  }
  const confirmation = state.confirmation
  const optedOutAt = state.optedOutAt

  const onOptOut = () =>
    confirmOptOut({
      title: t(($) => $.contacts.smsOptOutConfirmTitle),
      description: t(($) => $.contacts.smsOptOutConfirmDescription),
      confirmLabel: t(($) => $.contacts.smsRecordOptOut),
      onConfirm: () => optOut.mutate({ path }),
    })

  return (
    <Stack>
      <Group justify="space-between">
        <Title order={5}>{t(($) => $.contacts.smsTitle)}</Title>
        <Group gap="xs">
          <Button variant="default" onClick={openConsent}>
            {t(($) => $.contacts.smsRecordConsent)}
          </Button>
          {optedOutAt ? null : (
            <Button variant="default" color="red" loading={optOut.isPending} onClick={onOptOut}>
              {t(($) => $.contacts.smsRecordOptOut)}
            </Button>
          )}
        </Group>
      </Group>
      {optedOutAt ? (
        <Text c="red" size="sm">
          {t(($) => $.contacts.smsOptedOut, { at: formatDateTime(optedOutAt) })}
        </Text>
      ) : null}
      {confirmation ? (
        <Table variant="vertical" layout="fixed" withTableBorder>
          <Table.Tbody>
            <Table.Tr>
              <Table.Th w={180}>{t(($) => $.contacts.smsConfirmedAt)}</Table.Th>
              <Table.Td>{formatDateTime(confirmation.confirmedAt)}</Table.Td>
            </Table.Tr>
            <Table.Tr>
              <Table.Th>{t(($) => $.contacts.smsProvenance)}</Table.Th>
              <Table.Td>{provenances[confirmation.provenance]}</Table.Td>
            </Table.Tr>
            <Table.Tr>
              <Table.Th>{t(($) => $.contacts.smsWording)}</Table.Th>
              <Table.Td>{confirmation.wording ?? '—'}</Table.Td>
            </Table.Tr>
            <Table.Tr>
              <Table.Th>{t(($) => $.contacts.smsSource)}</Table.Th>
              <Table.Td>{confirmation.source ?? '—'}</Table.Td>
            </Table.Tr>
            <Table.Tr>
              <Table.Th>{t(($) => $.contacts.smsBrand)}</Table.Th>
              <Table.Td>{confirmation.brand ?? '—'}</Table.Td>
            </Table.Tr>
          </Table.Tbody>
        </Table>
      ) : (
        <Text c="dimmed" size="sm">
          {t(($) => $.contacts.smsNoConsent)}
        </Text>
      )}
      <Modal
        opened={consentOpened}
        onClose={closeConsent}
        title={t(($) => $.contacts.smsRecordConsent)}
      >
        <form onSubmit={form.onSubmit((body) => confirm.mutate({ path, body }))}>
          <Stack>
            <Text size="sm" c="dimmed">
              {t(($) => $.contacts.smsConsentHint)}
            </Text>
            <Textarea
              label={t(($) => $.contacts.smsWording)}
              description={t(($) => $.contacts.smsWordingHint)}
              withAsterisk
              autosize
              minRows={2}
              data-autofocus
              {...form.getInputProps('wording')}
            />
            <TextInput
              label={t(($) => $.contacts.smsSource)}
              description={t(($) => $.contacts.smsSourceHint)}
              withAsterisk
              {...form.getInputProps('source')}
            />
            <TextInput
              label={t(($) => $.contacts.smsBrand)}
              description={t(($) => $.contacts.smsBrandHint)}
              withAsterisk
              {...form.getInputProps('brand')}
            />
            <Group justify="flex-end">
              <Button variant="default" onClick={closeConsent}>
                {t(($) => $.actions.cancel)}
              </Button>
              <Button type="submit" loading={confirm.isPending}>
                {t(($) => $.contacts.smsRecordConsent)}
              </Button>
            </Group>
          </Stack>
        </form>
      </Modal>
    </Stack>
  )
}
