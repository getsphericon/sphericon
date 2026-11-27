import { Button, Checkbox, Group, Modal, SimpleGrid, Stack, Text } from '@mantine/core'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { siteContactCsvExportsCreateMutation } from '../generated/site/@tanstack/react-query.gen.ts'
import { ContactCsvColumn } from '../generated/site/types.gen.ts'
import { useResourceMutation } from '../hooks/useResourceMutation.ts'
import { csvExportGroups, everyCsvColumn } from '../utils/contactCsvExport.ts'

// ContactCsvExportModal asks for the CSV export of a Segment's Contacts, or of all of them, with the
// columns the Member chooses (ADR 0138). The answer is a one-time link the browser follows, so the
// file streams to disk as the server writes it, never through the page.
export function ContactCsvExportModal({
  workspaceId,
  segmentId,
  opened,
  onClose,
}: {
  workspaceId: string
  // The Segment whose Contacts are exported; every Contact of the Workspace without one.
  segmentId?: string
  opened: boolean
  onClose: () => void
}) {
  const { t } = useTranslation()
  const [columns, setColumns] = useState<ContactCsvColumn[]>(everyCsvColumn)
  const labels: Record<ContactCsvColumn, string> = {
    [ContactCsvColumn.SUBJECT_ID]: t(($) => $.contactCsvExport.columns.subjectId),
    [ContactCsvColumn.EMAIL]: t(($) => $.contactCsvExport.columns.email),
    [ContactCsvColumn.PHONE]: t(($) => $.contactCsvExport.columns.phone),
    [ContactCsvColumn.FIRST_NAME]: t(($) => $.contactCsvExport.columns.firstName),
    [ContactCsvColumn.LAST_NAME]: t(($) => $.contactCsvExport.columns.lastName),
    [ContactCsvColumn.TIME_ZONE]: t(($) => $.contactCsvExport.columns.timeZone),
    [ContactCsvColumn.COUNTRY]: t(($) => $.contactCsvExport.columns.country),
    [ContactCsvColumn.LOCALE]: t(($) => $.contactCsvExport.columns.locale),
    [ContactCsvColumn.BIRTHDAY]: t(($) => $.contactCsvExport.columns.birthday),
    [ContactCsvColumn.CITY]: t(($) => $.contactCsvExport.columns.city),
    [ContactCsvColumn.AVATAR]: t(($) => $.contactCsvExport.columns.avatar),
    [ContactCsvColumn.SOURCE]: t(($) => $.contactCsvExport.columns.source),
    [ContactCsvColumn.CREATED_AT]: t(($) => $.contactCsvExport.columns.createdAt),
    [ContactCsvColumn.UPDATED_AT]: t(($) => $.contactCsvExport.columns.updatedAt),
    [ContactCsvColumn.CUSTOM_FIELDS]: t(($) => $.contactCsvExport.columns.customFields),
    [ContactCsvColumn.TAGS]: t(($) => $.contactCsvExport.columns.tags),
    [ContactCsvColumn.EMAIL_CHANNEL]: t(($) => $.contactCsvExport.columns.emailChannel),
    [ContactCsvColumn.SMS_CHANNEL]: t(($) => $.contactCsvExport.columns.smsChannel),
    [ContactCsvColumn.TELEGRAM_CHANNEL]: t(($) => $.contactCsvExport.columns.telegramChannel),
  }
  const groupTitles = {
    keys: t(($) => $.contactCsvExport.groups.keys),
    attributes: t(($) => $.contactCsvExport.groups.attributes),
    fields: t(($) => $.contactCsvExport.groups.fields),
    channels: t(($) => $.contactCsvExport.groups.channels),
  }

  const exportMutation = useResourceMutation({
    mutation: siteContactCsvExportsCreateMutation(),
    invalidate: false,
    errorTitle: t(($) => $.contactCsvExport.errorTitle),
    forbiddenMessage: t(($) => $.contactCsvExport.forbidden),
    onDone: (link) => {
      onClose()
      // The browser follows the link: the file downloads as it is written.
      window.location.assign(link.url)
    },
  })

  return (
    <Modal opened={opened} onClose={onClose} title={t(($) => $.contactCsvExport.title)} size="lg">
      <Stack>
        <Text size="sm" c="dimmed">
          {segmentId
            ? t(($) => $.contactCsvExport.segmentDescription)
            : t(($) => $.contactCsvExport.allDescription)}
        </Text>
        <Checkbox.Group
          value={columns}
          // The modal's order, whatever order the boxes were ticked in.
          onChange={(next) => setColumns(everyCsvColumn.filter((column) => next.includes(column)))}
          error={columns.length === 0 ? t(($) => $.contactCsvExport.noColumns) : undefined}
        >
          <Stack>
            {csvExportGroups.map((group) => (
              <Stack key={group.name} gap="xs">
                <Text fw={500} size="sm">
                  {groupTitles[group.name]}
                </Text>
                <SimpleGrid cols={{ base: 1, sm: 3 }} spacing="xs">
                  {group.columns.map((column) => (
                    <Checkbox key={column} value={column} label={labels[column]} />
                  ))}
                </SimpleGrid>
              </Stack>
            ))}
          </Stack>
        </Checkbox.Group>
        <Group justify="flex-end">
          <Button variant="default" onClick={onClose}>
            {t(($) => $.actions.cancel)}
          </Button>
          <Button
            loading={exportMutation.isPending}
            disabled={columns.length === 0}
            onClick={() =>
              exportMutation.mutate({
                path: { workspaceId },
                body: { columns, ...(segmentId ? { segmentId } : {}) },
              })
            }
          >
            {t(($) => $.contactCsvExport.submit)}
          </Button>
        </Group>
      </Stack>
    </Modal>
  )
}
