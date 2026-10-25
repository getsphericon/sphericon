import {
  Box,
  EmptyState,
  Alert,
  Badge,
  Button,
  Code,
  CopyButton,
  Group,
  Menu,
  Modal,
  Stack,
  TextInput,
  Title,
  Tooltip,
} from '@mantine/core'
import { useForm } from '@mantine/form'
import { useDisclosure } from '@mantine/hooks'
import { useQuery } from '@tanstack/react-query'
import { DataTable } from 'mantine-datatable'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { RowActionsMenu } from '../../components/RowActionsMenu.tsx'
import { TableSkeleton } from '../../components/Skeletons.tsx'
import {
  siteTokensCreateMutation,
  siteTokensDeleteMutation,
  siteTokensListOptions,
  siteTokensListQueryKey,
  siteTokensUpdateMutation,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import type { ApiTokenScope } from '../../generated/site/types.gen.ts'
import { zApiTokenScope } from '../../generated/site/zod.gen.ts'
import { useConfirmation } from '../../hooks/useConfirmation.tsx'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'
import { formatDate } from '../../utils/datetime.ts'
import { ScopePicker } from './ScopePicker.tsx'

// Scopes offered in the UI: the contract's ApiTokenScope vocabulary.
const SCOPE_OPTIONS = zApiTokenScope.options

export function ApiKeysSection({ workspaceId }: { workspaceId: string }) {
  const { t } = useTranslation()
  const confirmRevoke = useConfirmation()
  const [renaming, setRenaming] = useState<{ id: string; name: string } | null>(null)
  const [newSecret, setNewSecret] = useState<string | null>(null)
  const [createOpened, { open: openCreate, close }] = useDisclosure(false)
  const closeCreate = () => {
    close()
    setNewSecret(null)
  }

  const queryKey = siteTokensListQueryKey({ path: { workspaceId } })
  const tokensQuery = useQuery(siteTokensListOptions({ path: { workspaceId } }))

  const renameForm = useForm({ initialValues: { name: '' } })
  const openRename = (record: { id: string; name: string }) => {
    renameForm.setValues({ name: record.name })
    setRenaming(record)
  }

  const form = useForm<{ name: string; scopes: ApiTokenScope[] }>({
    initialValues: { name: '', scopes: [] },
  })

  const createMutation = useResourceMutation({
    mutation: siteTokensCreateMutation(),
    invalidate: [queryKey],
    errorTitle: t(($) => $.settings.tokens.createError),
    forbiddenMessage: t(($) => $.settings.tokens.forbidden),
    onDone: (data) => {
      setNewSecret(data.token)
      form.reset()
    },
  })

  const renameMutation = useResourceMutation({
    mutation: siteTokensUpdateMutation(),
    invalidate: [queryKey],
    errorTitle: t(($) => $.settings.tokens.renameError),
    forbiddenMessage: t(($) => $.settings.tokens.forbidden),
    onDone: () => setRenaming(null),
  })

  const deleteMutation = useResourceMutation({
    mutation: siteTokensDeleteMutation(),
    invalidate: [queryKey],
    errorTitle: t(($) => $.settings.tokens.revokeError),
    forbiddenMessage: t(($) => $.settings.tokens.forbidden),
  })

  const onRevoke = (id: string) => {
    confirmRevoke({
      title: t(($) => $.settings.tokens.revokeConfirmTitle),
      description: t(($) => $.settings.tokens.revokeConfirmDescription),
      onConfirm: () => deleteMutation.mutate({ path: { workspaceId, id } }),
    })
  }

  const records = tokensQuery.data ?? []

  return (
    <Box>
      <Group justify="space-between" mb="sm">
        <Title order={4}>{t(($) => $.settings.apiKeysTitle)}</Title>
        <Button size="xs" onClick={openCreate}>
          {t(($) => $.settings.tokens.create)}
        </Button>
      </Group>

      {tokensQuery.isError ? (
        <Alert color="red" title={t(($) => $.settings.tokens.loadError)} mb="md">
          {t(($) => $.settings.tokens.loadError)}
        </Alert>
      ) : null}

      {tokensQuery.isLoading ? <TableSkeleton /> : null}

      <DataTable
        minHeight={records.length === 0 ? 200 : 0}
        withTableBorder
        records={records}
        idAccessor="id"
        columns={[
          { accessor: 'name' },
          {
            accessor: 'scopes',
            title: t(($) => $.settings.tokens.scopesLabel),
            render: (record) => (
              <Tooltip label={record.scopes.join(', ')} multiline maw={320} withArrow>
                <Badge variant="light" color="gray">
                  {t(($) => $.settings.tokens.scopesCount, { count: record.scopes.length })}
                </Badge>
              </Tooltip>
            ),
          },
          {
            accessor: 'createdAt',
            title: t(($) => $.settings.tokens.created),
            render: (record) => formatDate(record.createdAt),
          },
          {
            accessor: 'expiresAt',
            title: t(($) => $.settings.tokens.expires),
            render: (record) =>
              record.expiresAt ? formatDate(record.expiresAt) : t(($) => $.settings.tokens.never),
          },
          {
            accessor: 'actions',
            title: '',
            render: (record) =>
              record.revokedAt ? (
                <Badge color="red" variant="light">
                  {t(($) => $.settings.tokens.revoked)}
                </Badge>
              ) : (
                <Group justify="flex-end">
                  <RowActionsMenu>
                    <Menu.Item onClick={() => openRename(record)}>
                      {t(($) => $.settings.tokens.rename)}
                    </Menu.Item>
                    <Menu.Item color="red" onClick={() => onRevoke(record.id)}>
                      {t(($) => $.settings.tokens.revoke)}
                    </Menu.Item>
                  </RowActionsMenu>
                </Group>
              ),
          },
        ]}
        emptyState={<EmptyState title={t(($) => $.settings.tokens.empty)} />}
      />
      <Modal
        opened={renaming !== null}
        onClose={() => setRenaming(null)}
        title={t(($) => $.settings.tokens.rename)}
      >
        <form
          onSubmit={renameForm.onSubmit((values) => {
            if (renaming) {
              renameMutation.mutate({
                path: { workspaceId, id: renaming.id },
                body: { name: values.name.trim() },
              })
            }
          })}
        >
          <Stack>
            <TextInput
              label={t(($) => $.settings.tokens.nameLabel)}
              required
              data-autofocus
              {...renameForm.getInputProps('name')}
            />
            <Button type="submit" loading={renameMutation.isPending}>
              {t(($) => $.actions.save)}
            </Button>
          </Stack>
        </form>
      </Modal>
      <Modal opened={createOpened} onClose={closeCreate} title={t(($) => $.settings.tokens.create)}>
        {newSecret ? (
          <Alert color="yellow" title={t(($) => $.settings.tokens.secretNotice)}>
            <Group align="center" gap="sm" wrap="nowrap">
              <Code>{newSecret}</Code>
              <CopyButton value={newSecret}>
                {({ copied, copy }) => (
                  <Button size="compact-xs" variant="light" onClick={copy}>
                    {copied ? t(($) => $.settings.copied) : t(($) => $.settings.copy)}
                  </Button>
                )}
              </CopyButton>
            </Group>
          </Alert>
        ) : (
          <form
            onSubmit={form.onSubmit((values) =>
              createMutation.mutate({
                path: { workspaceId },
                body: { name: values.name.trim(), scopes: values.scopes },
              }),
            )}
          >
            <Stack>
              <TextInput
                label={t(($) => $.settings.tokens.nameLabel)}
                required
                data-autofocus
                {...form.getInputProps('name')}
              />
              <ScopePicker
                label={t(($) => $.settings.tokens.scopesLabel)}
                options={SCOPE_OPTIONS}
                value={form.values.scopes}
                onChange={(scopes) => form.setFieldValue('scopes', scopes)}
              />
              <Button type="submit" loading={createMutation.isPending}>
                {t(($) => $.settings.tokens.create)}
              </Button>
            </Stack>
          </form>
        )}
      </Modal>
    </Box>
  )
}
