import { Alert, Button, Code, Group, Select, Stack, Text, Textarea } from '@mantine/core'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useJsonForms } from '@workflowbuilder/sdk'
import type { ControlProps } from '@workflowbuilder/sdk'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ApiErrorAlert } from '../../components/ApiErrorAlert.tsx'
import { HttpHeaderRows } from '../../components/HttpHeaderRows.tsx'
import {
  siteAutomationsDryRunHttpStepMutation,
  siteContactsListOptions,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import { Route as workspaceRoute } from '../../routes/workspaces/$workspaceId/route.tsx'
import { headersOf, headersText } from './definition.ts'

// The step's headers as rows, as a Rule's http Action edits them. Only the names of saved headers
// come back from the server, so a saved step (one with an id) keeps a header whose value is left
// empty.
export function HttpHeadersControl(props: ControlProps) {
  const { data, label, path } = props
  const { core } = useJsonForms()
  const saved = typeof core?.data?.stepId === 'string' && core.data.stepId !== ''

  return (
    <Stack gap={4}>
      <Text size="sm" fw={500}>
        {label}
      </Text>
      <HttpHeaderRows
        headers={headersOf(typeof data === 'string' ? data : '')}
        saved={saved}
        onChange={(headers) => props.handleChange(path, headersText(headers))}
      />
    </Stack>
  )
}

// The step's body, with a dry run: the server renders it for a chosen Contact and checks it is
// valid JSON, calling nothing, so the body being edited is checked before it is saved.
export function HttpBodyControl(props: ControlProps) {
  const { data, label, path } = props
  const { t } = useTranslation()
  const body = typeof data === 'string' ? data : ''

  return (
    <Stack gap="xs">
      <Textarea
        label={label}
        description={t(($) => $.automations.httpBodyHint)}
        autosize
        minRows={6}
        value={body}
        onChange={(event) => props.handleChange(path, event.currentTarget.value)}
      />
      <HttpDryRun body={body} />
    </Stack>
  )
}

function HttpDryRun({ body }: { body: string }) {
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
        <ApiErrorAlert
          error={dryRun.error}
          title={t(($) => $.automations.httpDryRunError)}
          fallback={t(($) => $.automations.httpDryRunError)}
        />
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
