import { Badge, Button, Group, Stack, Text, Title } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import { PagedTable } from '../../components/PagedTable.tsx'
import {
  siteCapturedMessagesDeleteAllMutation,
  siteCapturedMessagesListOptions,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import { useConfirmation } from '../../hooks/useConfirmation.tsx'
import { useDateFormat } from '../../hooks/useDateFormat.ts'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'
import { Route as capturedMessageRoute } from '../../routes/workspaces/$workspaceId/captured-messages/$messageId/index.tsx'
import { Route as capturedMessagesRoute } from '../../routes/workspaces/$workspaceId/captured-messages/index.tsx'
import { withPage } from '../../searchSchemas.ts'

export function CapturedMessagesListPage() {
  const { formatDateTime } = useDateFormat()
  const { page } = capturedMessagesRoute.useSearch()
  const { t } = useTranslation()
  const { workspaceId } = capturedMessagesRoute.useParams()
  const navigate = capturedMessagesRoute.useNavigate()
  const confirm = useConfirmation()

  const deleteAll = useResourceMutation({
    mutation: siteCapturedMessagesDeleteAllMutation(),
    successMessage: t(($) => $.capturedMessages.deletedAll),
    errorTitle: t(($) => $.capturedMessages.deleteError),
  })

  return (
    <Stack>
      <Group justify="space-between" align="center">
        <Title order={2}>{t(($) => $.capturedMessages.title)}</Title>
        <Button
          color="red"
          variant="light"
          onClick={() => {
            confirm({
              title: t(($) => $.capturedMessages.deleteAllTitle),
              description: t(($) => $.capturedMessages.deleteAllDescription),
              confirmLabel: t(($) => $.capturedMessages.deleteAll),
              onConfirm: () => deleteAll.mutate({ path: { workspaceId } }),
            })
          }}
        >
          {t(($) => $.capturedMessages.deleteAll)}
        </Button>
      </Group>
      <Text c="dimmed" size="sm">
        {t(($) => $.capturedMessages.description)}
      </Text>

      <PagedTable
        page={page}
        onPageChange={(next) => void navigate({ search: withPage(next), viewTransition: false })}
        pageSize={20}
        options={(query) => siteCapturedMessagesListOptions({ path: { workspaceId }, query })}
        loadErrorTitle={t(($) => $.capturedMessages.loadError)}
        noRecordsText={t(($) => $.capturedMessages.noRecords)}
        onRowClick={(record) =>
          void navigate({
            to: capturedMessageRoute.to,
            params: { workspaceId, messageId: record.id },
          })
        }
        columns={[
          { accessor: 'recipient', title: t(($) => $.capturedMessages.recipient) },
          {
            accessor: 'subject',
            title: t(($) => $.capturedMessages.subject),
            render: (record) => (
              <Group gap="xs">
                <Text size="sm">{record.subject}</Text>
                {record.fromUnverified ? (
                  <Badge color="yellow" variant="light">
                    {t(($) => $.capturedMessages.fromUnverified)}
                  </Badge>
                ) : null}
              </Group>
            ),
          },
          {
            accessor: 'surface',
            title: t(($) => $.capturedMessages.surface),
            render: (record) => t(($) => $.capturedMessages.surfaces[record.surface]),
          },
          {
            accessor: 'createdAt',
            title: t(($) => $.capturedMessages.capturedAt),
            render: (record) => formatDateTime(record.createdAt),
          },
        ]}
      />
    </Stack>
  )
}
