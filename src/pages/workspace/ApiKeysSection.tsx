import {
  Alert,
  Button,
  Card,
  Code,
  CopyButton,
  Group,
  Loader,
  MultiSelect,
  TextInput,
  Title,
} from '@mantine/core'
import { useForm } from '@mantine/form'
import { useQuery } from '@tanstack/react-query'
import { DataTable } from 'mantine-datatable'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  siteTokensCreateMutation,
  siteTokensDeleteMutation,
  siteTokensListOptions,
  siteTokensListQueryKey,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import { useDeleteConfirmation } from '../../hooks/useDeleteConfirmation.tsx'
import { useResourceMutation } from '../../hooks/useResourceMutation.ts'
import { formatDate } from '../../utils/datetime.ts'

// Scopes offered in the UI. The backend stores whatever strings it receives;
// these mirror the external API's scope vocabulary.
const SCOPE_OPTIONS = [
  'contacts:read',
  'contacts:write',
  'contacts:erase',
  'events:read',
  'events:write',
  'segments:read',
  'segments:write',
  'broadcasts:read',
  'broadcasts:write',
  'automations:read',
  'automations:write',
  'templates:read',
  'templates:write',
  'webhooks:read',
  'webhooks:write',
  'custom_fields:read',
  'sending_domains:read',
  'sending_domains:write',
  'integrations:read',
  'integrations:write',
  'emails:send',
  'broadcasts:send',
  'automations:activate',
  'mcp:send',
  'tokens:read',
  'tokens:write',
  'audit:read',
]

export function ApiKeysSection({ slug }: { slug: string }) {
  const { t } = useTranslation()
  const confirmRevoke = useDeleteConfirmation()
  const [newSecret, setNewSecret] = useState<string | null>(null)

  const queryKey = siteTokensListQueryKey({ path: { slug: slug } })
  const tokensQuery = useQuery(siteTokensListOptions({ path: { slug: slug } }))

  const form = useForm<{ name: string; scopes: string[] }>({
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
      onConfirm: () => deleteMutation.mutate({ path: { slug: slug, id } }),
    })
  }

  const records = tokensQuery.data ?? []

  return (
    <Card withBorder>
      <Title order={4} mb="sm">
        {t(($) => $.settings.apiKeysTitle)}
      </Title>

      {newSecret ? (
        <Alert
          color="yellow"
          title={t(($) => $.settings.tokens.secretNotice)}
          mb="md"
          withCloseButton
          onClose={() => setNewSecret(null)}
        >
          <Group align="center" gap="sm" wrap="nowrap">
            <Code>{newSecret}</Code>
            <CopyButton value={newSecret}>
              {({ copied, copy }) => (
                <Button size="compact-sm" variant="light" onClick={copy}>
                  {copied ? t(($) => $.settings.copied) : t(($) => $.settings.copy)}
                </Button>
              )}
            </CopyButton>
          </Group>
        </Alert>
      ) : null}

      {tokensQuery.isError ? (
        <Alert color="red" title={t(($) => $.settings.tokens.loadError)} mb="md">
          {t(($) => $.settings.tokens.loadError)}
        </Alert>
      ) : null}

      <form
        onSubmit={form.onSubmit((values) =>
          createMutation.mutate({
            path: { slug: slug },
            body: { name: values.name.trim(), scopes: values.scopes },
          }),
        )}
      >
        <Group align="flex-end" gap="sm" mb="md">
          <TextInput
            label={t(($) => $.settings.tokens.nameLabel)}
            required
            {...form.getInputProps('name')}
          />
          <MultiSelect
            label={t(($) => $.settings.tokens.scopesLabel)}
            data={SCOPE_OPTIONS}
            {...form.getInputProps('scopes')}
            w={320}
          />
          <Button type="submit" loading={createMutation.isPending}>
            {t(($) => $.settings.tokens.create)}
          </Button>
        </Group>
      </form>

      {tokensQuery.isLoading ? <Loader /> : null}

      <DataTable
        withTableBorder
        records={records}
        idAccessor="id"
        columns={[
          { accessor: 'name' },
          { accessor: 'prefix', title: t(($) => $.settings.tokens.prefix) },
          {
            accessor: 'scopes',
            title: t(($) => $.settings.tokens.scopesLabel),
            render: (record) => record.scopes.join(', '),
          },
          {
            accessor: 'createdAt',
            title: t(($) => $.settings.tokens.created),
            render: (record) => formatDate(record.createdAt),
          },
          {
            accessor: 'actions',
            title: '',
            render: (record) => (
              <Button
                size="compact-sm"
                color="red"
                variant="light"
                onClick={() => onRevoke(record.id)}
              >
                {t(($) => $.settings.tokens.revoke)}
              </Button>
            ),
          },
        ]}
        noRecordsText={t(($) => $.settings.tokens.empty)}
      />
    </Card>
  )
}
