import {
  Box,
  EmptyState,
  Alert,
  Badge,
  Button,
  Group,
  Menu,
  Modal,
  Stack,
  Switch,
  TextInput,
  Title,
} from '@mantine/core'
import { useForm } from '@mantine/form'
import { useCounter, useDisclosure } from '@mantine/hooks'
import { useQuery } from '@tanstack/react-query'
import { DataTable } from 'mantine-datatable'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { RowActionsMenu } from '../../components/RowActionsMenu.tsx'
import { TableSkeleton } from '../../components/Skeletons.tsx'
import {
  siteSuppressionsCreateMutation,
  siteSuppressionsDeleteMutation,
  siteSuppressionsListOptions,
  siteSuppressionsListQueryKey,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import type { SiteSuppressionReason } from '../../generated/site/types.gen.ts'
import { useConfirmation } from '../../hooks/useConfirmation.tsx'
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
// The send path skips every active destination listed here; a lifted one stays as a record
// and is shown on request. (Unsubscribes are a
// separate, per-source opt-out, not a suppression.)
export function SuppressionsSection({ workspaceId }: { workspaceId: string }) {
  const { t } = useTranslation()
  const confirmLift = useConfirmation()
  const [page, pageHandlers] = useCounter(1, { min: 1 })
  const [lifted, setLifted] = useState(false)
  const [createOpened, { open: openCreate, close: closeCreate }] = useDisclosure(false)

  const queryKey = siteSuppressionsListQueryKey({ path: { workspaceId } })
  const suppressionsQuery = useQuery(
    siteSuppressionsListOptions({
      path: { workspaceId },
      query: { page, pageSize: PAGE_SIZE, lifted },
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

  const liftMutation = useResourceMutation({
    mutation: siteSuppressionsDeleteMutation(),
    invalidate: [queryKey],
    errorTitle: t(($) => $.settings.suppressions.liftError),
  })

  const onLift = (id: string) =>
    confirmLift({
      title: t(($) => $.settings.suppressions.liftConfirmTitle),
      description: t(($) => $.settings.suppressions.liftConfirmDescription),
      confirmLabel: t(($) => $.settings.suppressions.lift),
      onConfirm: () => liftMutation.mutate({ path: { workspaceId, id } }),
    })

  const records = suppressionsQuery.data?.items ?? []
  const totalItems = suppressionsQuery.data?.totalItems ?? 0

  return (
    <Box>
      <Group justify="space-between" mb="sm">
        <Title order={4}>{t(($) => $.settings.suppressions.title)}</Title>
        <Button size="xs" onClick={openCreate}>
          {t(($) => $.settings.suppressions.create)}
        </Button>
      </Group>

      <Switch
        mb="sm"
        label={t(($) => $.settings.suppressions.showLifted)}
        checked={lifted}
        onChange={(event) => {
          setLifted(event.currentTarget.checked)
          pageHandlers.set(1)
        }}
      />

      {suppressionsQuery.isError ? (
        <Alert color="red" title={t(($) => $.settings.suppressions.loadError)} mb="md">
          {t(($) => $.settings.suppressions.loadError)}
        </Alert>
      ) : null}

      {suppressionsQuery.isLoading ? <TableSkeleton /> : null}

      <DataTable
        minHeight={records.length === 0 ? 200 : 0}
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
            render: (record) =>
              record.liftedAt ? (
                <Badge variant="light" color="gray">
                  {t(($) => $.settings.suppressions.liftedBadge)}
                </Badge>
              ) : (
                <Group justify="flex-end">
                  <RowActionsMenu>
                    <Menu.Item onClick={() => onLift(record.id)}>
                      {t(($) => $.settings.suppressions.lift)}
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
