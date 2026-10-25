import { Alert, Button, Group, Stack, Text } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import {
  siteInstanceAcknowledgeNoticeMutation,
  siteInstanceGetStateQueryKey,
} from '../generated/site/@tanstack/react-query.gen.ts'
import { useInstanceState } from '../hooks/useInstanceState.ts'
import { useResourceMutation } from '../hooks/useResourceMutation.ts'
import { Route as instanceRoute } from '../routes/account/instance.tsx'
import { ButtonLink } from './RouterLink.tsx'
import { TelemetryReportActions } from './TelemetryReportActions.tsx'

// InstanceNotice tells an Owner of the oldest Workspace, once, that the Instance sends an
// anonymous Telemetry report (ADR 0032) and lets them decide. It shows until acknowledged
// and renders nothing for anyone else (the state is refused to them). It also renders nothing
// while the environment keeps the report off (the SaaS deployment, DO_NOT_TRACK): nothing is
// sent, so there is nothing to tell or to decide.
export function InstanceNotice() {
  const { t } = useTranslation()
  const state = useInstanceState()
  const acknowledge = useResourceMutation({
    mutation: siteInstanceAcknowledgeNoticeMutation(),
    invalidate: [siteInstanceGetStateQueryKey()],
    errorTitle: t(($) => $.instance.notice.errorTitle),
  })

  if (!state.data || state.data.noticeAcknowledged) {
    return null
  }
  if (state.data.telemetry.locked && !state.data.telemetry.enabled) {
    return null
  }
  return (
    <Alert color="blue" title={t(($) => $.instance.notice.title)} mb="md">
      <Group justify="space-between" align="flex-end">
        <Stack gap="xs">
          <Text size="sm" maw={720}>
            {state.data.telemetry.locked
              ? t(($) => $.instance.notice.descriptionLocked)
              : t(($) => $.instance.notice.description)}
          </Text>
          <TelemetryReportActions schemaUrl={state.data.telemetrySchemaUrl} size="xs" />
        </Stack>
        <Group gap="xs">
          <ButtonLink to={instanceRoute.to} size="xs" variant="light">
            {t(($) => $.instance.notice.settings)}
          </ButtonLink>
          <Button size="xs" loading={acknowledge.isPending} onClick={() => acknowledge.mutate({})}>
            {t(($) => $.instance.notice.acknowledge)}
          </Button>
        </Group>
      </Group>
    </Alert>
  )
}
