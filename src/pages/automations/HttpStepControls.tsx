import { Alert, Button, Code, Group, Select, Stack, Text } from '@mantine/core'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ApiErrorAlert } from '../../components/ApiErrorAlert.tsx'
import {
  siteAutomationsDryRunHttpStepMutation,
  siteContactsListOptions,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import { Route as workspaceRoute } from '../../routes/workspaces/$workspaceId/route.tsx'

// The dry run of an HTTP step's body: the server renders the body being edited for a chosen
// Contact and checks it is valid JSON, calling nothing, so the body is checked before it is saved.
export function HttpDryRun({ body }: { body: string }) {
  const { t } = useTranslation()
  const { workspaceId } = workspaceRoute.useParams()
  const [contactId, setContactId] = useState<string | null>(null)
  const contacts = useQuery(
    siteContactsListOptions({ path: { workspaceId }, query: { page: 1, pageSize: 50 } }),
  )
  const dryRun = useMutation(siteAutomationsDryRunHttpStepMutation())
  const result = dryRun.data

  return (
    <Stack gap="xs">
      <Group align="flex-end" wrap="nowrap">
        <Select
          label={t(($) => $.automations.httpDryRunContactLabel)}
          placeholder={t(($) => $.automations.httpDryRunContactPlaceholder)}
          searchable
          data={(contacts.data?.items ?? []).map((contact) => ({
            value: contact.id,
            label: contact.email ?? contact.id,
          }))}
          value={contactId}
          onChange={setContactId}
        />
        <Button
          variant="default"
          disabled={contactId === null}
          loading={dryRun.isPending}
          onClick={() => {
            if (contactId !== null) {
              dryRun.mutate({ path: { workspaceId }, body: { body, contactId } })
            }
          }}
        >
          {t(($) => $.automations.httpDryRun)}
        </Button>
      </Group>
      {dryRun.isError ? (
        <ApiErrorAlert error={dryRun.error} title={t(($) => $.automations.httpDryRunError)} />
      ) : null}
      {result?.errorKind ? (
        <Alert color="red" title={t(($) => $.automations.httpDryRunInvalid)}>
          {result.detail}
        </Alert>
      ) : null}
      {result?.body === undefined ? null : (
        <Stack gap={4}>
          <Text size="sm">{t(($) => $.automations.httpDryRunRendered)}</Text>
          <Code block>{result.body}</Code>
        </Stack>
      )}
    </Stack>
  )
}
