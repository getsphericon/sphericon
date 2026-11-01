import { Alert, Button, Group, List, Select, Stack, Text, Title } from '@mantine/core'
import { useMutation, useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ApiErrorAlert } from '../../components/ApiErrorAlert.tsx'
import {
  siteEventsListOptions,
  siteRulesDryRunMutation,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import { SiteRuleDryRunOutcome } from '../../generated/site/types.gen.ts'
import { useDateFormat } from '../../hooks/useDateFormat.ts'
import { useEventActionLabel } from '../../hooks/useEventActionLabel.ts'

// Tries the saved Rule on one stored Event: whether it would fire and which Actions would run.
// A dry run writes and runs nothing, so it is safe to repeat.
export function RuleDryRun({ workspaceId, ruleId }: { workspaceId: string; ruleId: string }) {
  const { formatDateTime } = useDateFormat()
  const { t } = useTranslation()
  const labelOf = useEventActionLabel()
  const [eventId, setEventId] = useState<string | null>(null)
  const events = useQuery(
    siteEventsListOptions({ path: { workspaceId }, query: { page: 1, pageSize: 50 } }),
  )
  const dryRun = useMutation(siteRulesDryRunMutation())

  return (
    <Stack>
      <Title order={5}>{t(($) => $.rules.dryRunTitle)}</Title>
      <Text size="sm" c="dimmed">
        {t(($) => $.rules.dryRunHint)}
      </Text>
      <Group align="flex-end">
        <Select
          label={t(($) => $.rules.dryRunEventLabel)}
          placeholder={t(($) => $.rules.dryRunEventPlaceholder)}
          searchable
          data={(events.data?.items ?? []).map((event) => ({
            value: event.id,
            label: `${labelOf(event.action)} · ${formatDateTime(event.createdAt)}`,
          }))}
          value={eventId}
          onChange={setEventId}
        />
        <Button
          variant="default"
          disabled={eventId === null}
          loading={dryRun.isPending}
          onClick={() => {
            if (eventId !== null) {
              dryRun.mutate({ path: { workspaceId, id: ruleId }, body: { eventId } })
            }
          }}
        >
          {t(($) => $.rules.dryRunRun)}
        </Button>
      </Group>
      {dryRun.isError ? (
        <ApiErrorAlert
          error={dryRun.error}
          title={t(($) => $.alerts.ruleDryRunErrorTitle)}
          fallback={t(($) => $.alerts.ruleDryRunErrorTitle)}
        />
      ) : null}
      {dryRun.data ? (
        <Alert
          color={dryRun.data.outcome === SiteRuleDryRunOutcome.WOULD_FIRE ? 'teal' : 'gray'}
          title={t(($) => $.rules.dryRunOutcome[dryRun.data.outcome])}
        >
          {dryRun.data.actions.length > 0 ? (
            <List size="sm">
              {dryRun.data.actions.map((action, index) => (
                <List.Item key={index}>{action.summary}</List.Item>
              ))}
            </List>
          ) : null}
        </Alert>
      ) : null}
    </Stack>
  )
}
