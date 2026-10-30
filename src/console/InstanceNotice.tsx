import { Alert, Button, Group, Stack, Text } from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { ButtonLink } from '../components/RouterLink.tsx'
import {
  staffInstanceAcknowledgeNoticeMutation,
  staffInstanceGetStateOptions,
  staffInstanceGetStateQueryKey,
} from '../generated/staff/@tanstack/react-query.gen.ts'
import { useResourceMutation } from '../hooks/useResourceMutation.ts'
import { Route as consoleInstanceRoute } from '../routes/console/_authed/instance.tsx'
import { TelemetryReportActions } from './TelemetryReportActions.tsx'

// InstanceNotice tells the Staff member, once, that the Instance sends an anonymous Telemetry
// report (ADR 0032) and lets them decide; the first report waits for the acknowledgement. It shows
// on every page of the console until acknowledged, and not at all while the environment keeps the
// report off (the saas Mode, DO_NOT_TRACK): nothing is sent, so there is nothing to tell.
export function InstanceNotice() {
  const { t } = useTranslation()
  const state = useQuery({ ...staffInstanceGetStateOptions(), retry: false })
  const acknowledge = useResourceMutation({
    mutation: staffInstanceAcknowledgeNoticeMutation(),
    invalidate: [staffInstanceGetStateQueryKey()],
    errorTitle: t(($) => $.console.instance.notice.errorTitle),
  })
  const data = state.data

  if (!data || data.noticeAcknowledged) {
    return null
  }
  if (data.telemetry.locked && !data.telemetry.enabled) {
    return null
  }
  return (
    <Alert color="blue" title={t(($) => $.console.instance.notice.title)} mb="md">
      <Group justify="space-between" align="flex-end">
        <Stack gap="xs">
          <Text size="sm" maw={720}>
            {data.telemetry.locked
              ? t(($) => $.console.instance.notice.descriptionLocked)
              : t(($) => $.console.instance.notice.description)}
          </Text>
          <TelemetryReportActions schemaUrl={data.telemetrySchemaUrl} />
        </Stack>
        <Group gap="xs">
          <ButtonLink to={consoleInstanceRoute.to} size="xs" variant="light">
            {t(($) => $.console.instance.notice.settings)}
          </ButtonLink>
          <Button size="xs" loading={acknowledge.isPending} onClick={() => acknowledge.mutate({})}>
            {t(($) => $.console.instance.notice.acknowledge)}
          </Button>
        </Group>
      </Group>
    </Alert>
  )
}
