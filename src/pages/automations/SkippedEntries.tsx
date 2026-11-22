import { Button, Drawer, Stack, Table, Text, TextInput } from '@mantine/core'
import { useDisclosure } from '@mantine/hooks'
import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ApiErrorAlert } from '../../components/ApiErrorAlert.tsx'
import { siteSkippedEntriesListOptions } from '../../generated/site/@tanstack/react-query.gen.ts'
import { useFormat } from '../../hooks/useFormat.ts'

/**
 * A button that opens the entries the automation's entry checks refused, newest first, each with
 * its reason, optionally of one contact. The list loads only once it is opened.
 */
export function SkippedEntries({
  workspaceId,
  automationId,
}: {
  workspaceId: string
  automationId: string
}) {
  const { t } = useTranslation()
  const { formatDateTime } = useFormat()
  const [opened, { open, close }] = useDisclosure(false)
  const [contactId, setContactId] = useState('')
  const contact = contactId.trim()
  const skippedQuery = useQuery({
    ...siteSkippedEntriesListOptions({
      path: { workspaceId, automationId },
      ...(contact === '' ? {} : { query: { contactId: contact } }),
    }),
    enabled: opened,
  })
  const title = t(($) => $.automations.skippedEntriesTitle)
  const entries = skippedQuery.data?.items ?? []

  return (
    <>
      <Button size="xs" variant="default" onClick={open}>
        {title}
      </Button>
      <Drawer opened={opened} onClose={close} title={title} position="right" size="xl">
        <Stack>
          <TextInput
            label={t(($) => $.automations.skippedEntriesContactFilter)}
            value={contactId}
            onChange={(event) => setContactId(event.currentTarget.value)}
          />
          {skippedQuery.isError && (
            <ApiErrorAlert
              error={skippedQuery.error}
              title={t(($) => $.automations.skippedEntriesLoadError)}
            />
          )}
          {skippedQuery.isSuccess && entries.length === 0 && (
            <Text size="sm" c="dimmed">
              {t(($) => $.automations.noSkippedEntries)}
            </Text>
          )}
          {entries.length > 0 && (
            <Table.ScrollContainer minWidth={420}>
              <Table>
                <Table.Thead>
                  <Table.Tr>
                    <Table.Th>{t(($) => $.automations.skippedEntryContact)}</Table.Th>
                    <Table.Th>{t(($) => $.automations.skippedEntryReason)}</Table.Th>
                    <Table.Th>{t(($) => $.automations.skippedEntryOccurredAt)}</Table.Th>
                  </Table.Tr>
                </Table.Thead>
                <Table.Tbody>
                  {entries.map(({ id, contactId: entryContact, reason, occurredAt }) => (
                    <Table.Tr key={id}>
                      <Table.Td>{entryContact}</Table.Td>
                      <Table.Td>{t(($) => $.automations.skippedReason[reason])}</Table.Td>
                      <Table.Td>{formatDateTime(occurredAt)}</Table.Td>
                    </Table.Tr>
                  ))}
                </Table.Tbody>
              </Table>
            </Table.ScrollContainer>
          )}
        </Stack>
      </Drawer>
    </>
  )
}
