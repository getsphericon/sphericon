import {
  Box,
  EmptyState,
  Alert,
  Badge,
  Button,
  Group,
  Loader,
  Menu,
  Modal,
  Stack,
  TextInput,
  Title,
} from '@mantine/core'
import { useForm } from '@mantine/form'
import { useCounter, useDisclosure } from '@mantine/hooks'
import { useQuery } from '@tanstack/react-query'
import { DataTable } from 'mantine-datatable'
import { useTranslation } from 'react-i18next'

import { RowActionsMenu } from '../../components/RowActionsMenu.tsx'
import {
  siteSuppressionsCreateMutation,
  siteSuppressionsDeleteMutation,
  siteSuppressionsListOptions,
  siteSuppressionsListQueryKey,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import type { SiteSuppressionReason } from '../../generated/site/types.gen.ts'
import { useDeleteConfirmation } from '../../hooks/useDeleteConfirmation.tsx'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'

const REASON_COLORS: Record<SiteSuppressionReason, string> = {
  bounce: 'orange',
  complaint: 'red',
  manual: 'gray',
}

// The list grows automatically (bounces/complaints), so it is paginated
// server-side rather than rendering only the first page.
const PAGE_SIZE = 20

// SuppressionsSection manages the workspace do-not-send list: hard bounces and
// complaints land here automatically; destinations can also be added manually.
// The send path skips every destination listed here. (Unsubscribes are a
// separate, per-source opt-out, not a suppression.)
export function SuppressionsSection({ workspaceId }: { workspaceId: string }) {
  const { t } = useTranslation()
  const confirmDelete = useDeleteConfirmation()
  const [page, pageHandlers] = useCounter(1, { min: 1 })
  const [createOpened, { open: openCreate, close: closeCreate }] = useDisclosure(false)

  const queryKey = siteSuppressionsListQueryKey({ path: { workspaceId } })
  const suppressionsQuery = useQuery(
    siteSuppressionsListOptions({
      path: { workspaceId },
      query: { page, pageSize: PAGE_SIZE },
    }),
  )

  const form = useForm<{ destination: string }>({ initialValues: { destination: '' } })

  const createMutation = useResourceMutation({
    mutation: siteSuppressionsCreateMutation(),
    invalidate: [queryKey],
    errorTitle: t(($) => $.settings.suppressions.createError),
    onDone: () => {
      form.reset()
      closeCreate()
      pageHandlers.set(1) // newest is first (id desc), so jump to page 1 to show it
    },
  })

  const deleteMutation = useResourceMutation({
    mutation: siteSuppressionsDeleteMutation(),
    invalidate: [queryKey],
    errorTitle: t(($) => $.settings.suppressions.deleteError),
  })

  const onDelete = (id: string) =>
    confirmDelete({
      title: t(($) => $.settings.suppressions.deleteConfirmTitle),
      onConfirm: () => deleteMutation.mutate({ path: { workspaceId, id } }),
    })

  const records = suppressionsQuery.data?.items ?? []
  const totalItems = suppressionsQuery.data?.totalItems ?? 0

  return (
    <Box>
      <Group justify="space-between" mb="sm">
        <Title order={4}>{t(($) => $.settings.suppressions.title)}</Title>
        <Button onClick={openCreate}>{t(($) => $.settings.suppressions.create)}</Button>
      </Group>

      {suppressionsQuery.isError ? (
        <Alert color="red" title={t(($) => $.settings.suppressions.loadError)} mb="md">
          {t(($) => $.settings.suppressions.loadError)}
        </Alert>
      ) : null}

      {suppressionsQuery.isLoading ? <Loader /> : null}

      <DataTable
        minHeight={200}
        withTableBorder
        records={records}
        idAccessor="id"
        columns={[
          { accessor: 'destination', title: t(($) => $.settings.suppressions.emailColumn) },
          {
            accessor: 'reason',
            title: t(($) => $.settings.suppressions.reasonColumn),
            render: (record) => (
              <Badge variant="light" color={REASON_COLORS[record.reason]}>
                {t(($) => $.settings.suppressions.reasons[record.reason])}
              </Badge>
            ),
          },
          {
            accessor: 'actions',
            title: '',
            render: (record) => (
              <Group justify="flex-end">
                <RowActionsMenu>
                  <Menu.Item color="red" onClick={() => onDelete(record.id)}>
                    {t(($) => $.settings.suppressions.delete)}
                  </Menu.Item>
                </RowActionsMenu>
              </Group>
            ),
          },
        ]}
        emptyState={<EmptyState title={t(($) => $.settings.suppressions.empty)} />}
        totalRecords={totalItems}
        recordsPerPage={PAGE_SIZE}
        page={page}
        onPageChange={pageHandlers.set}
      />
      <Modal
        opened={createOpened}
        onClose={closeCreate}
        title={t(($) => $.settings.suppressions.create)}
      >
        <form
          onSubmit={form.onSubmit((values) =>
            createMutation.mutate({
              path: { workspaceId },
              body: { destination: values.destination.trim() },
            }),
          )}
        >
          <Stack>
            <TextInput
              label={t(($) => $.settings.suppressions.emailLabel)}
              description={t(($) => $.settings.suppressions.emailHint)}
              placeholder="blocked@example.com"
              type="email"
              required
              data-autofocus
              {...form.getInputProps('destination')}
            />
            <Button type="submit" loading={createMutation.isPending}>
              {t(($) => $.settings.suppressions.create)}
            </Button>
          </Stack>
        </form>
      </Modal>
    </Box>
  )
}
