import {
  Alert,
  Box,
  Button,
  Code,
  CopyButton,
  EmptyState,
  Group,
  Modal,
  Select,
  Stack,
  Text,
  TextInput,
  Title,
} from '@mantine/core'
import { useForm } from '@mantine/form'
import { useDisclosure } from '@mantine/hooks'
import { useQuery, useSuspenseQuery } from '@tanstack/react-query'
import { DataTable } from 'mantine-datatable'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { QueryBoundary } from '../../components/QueryBoundary.tsx'
import { ScopesBadge } from '../../components/ScopesBadge.tsx'
import { TableSkeleton } from '../../components/Skeletons.tsx'
import {
  sitePersonalTokensCreateMutation,
  sitePersonalTokensDeleteMutation,
  sitePersonalTokensGrantableScopesOptions,
  sitePersonalTokensListOptions,
  sitePersonalTokensListQueryKey,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import type { PersonalScope, SiteWorkspaceResource } from '../../generated/site/types.gen.ts'
import { useConfirmation } from '../../hooks/useConfirmation.tsx'
import { useDateFormat } from '../../hooks/useDateFormat.ts'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'
import { can } from '../../utils/permissions.ts'
import { ScopePicker } from './ScopePicker.tsx'

// The lifetimes the mint form offers, in days; 'never' mints a token without an expiry.
const LIFETIMES = ['never', '30', '90', '365'] as const
type Lifetime = (typeof LIFETIMES)[number]

const DAY_MS = 24 * 60 * 60 * 1000

// expiryOf is the expiry a lifetime gives a token minted now, or null for none.
function expiryOf(lifetime: Lifetime): string | null {
  return lifetime === 'never'
    ? null
    : new Date(Date.now() + Number(lifetime) * DAY_MS).toISOString()
}

// PersonalTokensTable lists the tokens; the section's boundary shows the load and its failure.
function PersonalTokensTable({
  workspaceId,
  onRevoke,
}: {
  workspaceId: string
  onRevoke: (id: string) => void
}) {
  const { t } = useTranslation()
  const records = useSuspenseQuery(sitePersonalTokensListOptions({ path: { workspaceId } })).data

  return (
    <DataTable
      minHeight={records.length === 0 ? 160 : 0}
      withTableBorder
      records={records}
      idAccessor="id"
      columns={[
        { accessor: 'name', title: t(($) => $.settings.personalTokens.nameLabel) },
        {
          accessor: 'userName',
          title: t(($) => $.settings.personalTokens.user),
          render: (record) => `${record.userName} (${record.userEmail})`,
        },
        {
          accessor: 'scopes',
          title: t(($) => $.settings.personalTokens.scopes),
          render: (record) => <ScopesBadge scopes={record.scopes} />,
        },
        {
          accessor: 'createdAt',
          title: t(($) => $.settings.personalTokens.created),
          render: (record) => formatDate(record.createdAt),
        },
        {
          accessor: 'expiresAt',
          title: t(($) => $.settings.personalTokens.expires),
          render: (record) =>
            record.expiresAt
              ? formatDate(record.expiresAt)
              : t(($) => $.settings.personalTokens.never),
        },
        {
          accessor: 'actions',
          title: '',
          textAlign: 'right',
          render: (record) => (
            <Button
              size="xs"
              variant="subtle"
              color="red"
              onClick={() => onRevoke(record.id)}
              aria-label={t(($) => $.settings.personalTokens.revokeLabel, { name: record.name })}
            >
              {t(($) => $.settings.personalTokens.revoke)}
            </Button>
          ),
        },
      ]}
      emptyState={<EmptyState title={t(($) => $.settings.personalTokens.empty)} />}
    />
  )
}

// PersonalTokensSection lists the Personal tokens the caller may see (their own, or everyone's with
// members:manage, which the server decides) and mints one of the caller's own (ADR 0061). The mint
// form offers only the scopes the server says the caller's Role may grant.
export function PersonalTokensSection({ workspace }: { workspace: SiteWorkspaceResource }) {
  const { formatDate } = useDateFormat()
  const { t } = useTranslation()
  const confirmRevoke = useConfirmation()
  const workspaceId = workspace.id
  const [newSecret, setNewSecret] = useState<string | null>(null)
  const [createOpened, { open: openCreate, close }] = useDisclosure(false)
  const closeCreate = () => {
    close()
    setNewSecret(null)
  }

  const queryKey = sitePersonalTokensListQueryKey({ path: { workspaceId } })
  const grantable = useQuery({
    ...sitePersonalTokensGrantableScopesOptions({ path: { workspaceId } }),
    enabled: createOpened,
  })

  const form = useForm<{ name: string; scopes: PersonalScope[]; lifetime: Lifetime }>({
    initialValues: { name: '', scopes: [], lifetime: 'never' },
  })

  const createMutation = useResourceMutation({
    mutation: sitePersonalTokensCreateMutation(),
    invalidate: [queryKey],
    errorTitle: t(($) => $.settings.personalTokens.createError),
    onDone: (data) => {
      setNewSecret(data.token)
      form.reset()
    },
  })

  const revoke = useResourceMutation({
    mutation: sitePersonalTokensDeleteMutation(),
    invalidate: [queryKey],
    errorTitle: t(($) => $.settings.personalTokens.revokeError),
  })

  const onRevoke = (id: string) => {
    confirmRevoke({
      title: t(($) => $.settings.personalTokens.revokeConfirmTitle),
      description: t(($) => $.settings.personalTokens.revokeConfirmDescription),
      confirmLabel: t(($) => $.settings.personalTokens.revoke),
      onConfirm: () => revoke.mutate({ path: { workspaceId, id } }),
    })
  }

  return (
    <Box>
      <Group justify="space-between" mb="xs">
        <Title order={4}>{t(($) => $.settings.personalTokens.title)}</Title>
        <Button size="xs" onClick={openCreate}>
          {t(($) => $.settings.personalTokens.create)}
        </Button>
      </Group>
      <Text c="dimmed" size="sm" mb="sm">
        {can(workspace, 'members:manage')
          ? t(($) => $.settings.personalTokens.descriptionManager)
          : t(($) => $.settings.personalTokens.description)}
      </Text>
      <QueryBoundary
        fallback={<TableSkeleton />}
        errorTitle={t(($) => $.settings.personalTokens.loadError)}
      >
        <PersonalTokensTable workspaceId={workspaceId} onRevoke={onRevoke} />
      </QueryBoundary>
      <Modal
        opened={createOpened}
        onClose={closeCreate}
        title={t(($) => $.settings.personalTokens.create)}
      >
        {newSecret ? (
          <Alert color="yellow" title={t(($) => $.settings.personalTokens.secretNotice)}>
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
                body: {
                  name: values.name.trim(),
                  scopes: values.scopes,
                  expiresAt: expiryOf(values.lifetime),
                },
              }),
            )}
          >
            <Stack>
              <TextInput
                label={t(($) => $.settings.personalTokens.nameLabel)}
                required
                data-autofocus
                {...form.getInputProps('name')}
              />
              <Select
                label={t(($) => $.settings.personalTokens.expires)}
                allowDeselect={false}
                data={LIFETIMES.map((lifetime) => ({
                  value: lifetime,
                  label:
                    lifetime === 'never'
                      ? t(($) => $.settings.personalTokens.never)
                      : t(($) => $.settings.personalTokens.inDays, { count: Number(lifetime) }),
                }))}
                value={form.values.lifetime}
                onChange={(value) =>
                  form.setFieldValue(
                    'lifetime',
                    LIFETIMES.find((lifetime) => lifetime === value) ?? 'never',
                  )
                }
              />
              {grantable.isLoading ? <TableSkeleton /> : null}
              <ScopePicker
                label={t(($) => $.settings.personalTokens.scopes)}
                options={grantable.data ?? []}
                value={form.values.scopes}
                onChange={(scopes) => form.setFieldValue('scopes', scopes)}
              />
              <Button type="submit" loading={createMutation.isPending}>
                {t(($) => $.settings.personalTokens.create)}
              </Button>
            </Stack>
          </form>
        )}
      </Modal>
    </Box>
  )
}
