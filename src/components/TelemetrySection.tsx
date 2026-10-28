import { Box, Stack, Switch, Text, Title } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import {
  siteInstanceGetStateQueryKey,
  siteInstanceSetTelemetryMutation,
} from '../generated/site/@tanstack/react-query.gen.ts'
import type { SiteInstanceState } from '../generated/site/types.gen.ts'
import { useResourceMutation } from '../hooks/useResourceMutation.ts'
import { TelemetryReportActions } from './TelemetryReportActions.tsx'

// TelemetrySection is the switch of the anonymous Telemetry report (ADR 0032). When the
// deployment decides (the saas Mode or DO_NOT_TRACK), it is read-only and says why.
export function TelemetrySection({ state }: { state: SiteInstanceState }) {
  const { t } = useTranslation()
  const { telemetry } = state
  const mutation = useResourceMutation({
    mutation: siteInstanceSetTelemetryMutation(),
    invalidate: [siteInstanceGetStateQueryKey()],
    successMessage: t(($) => $.instance.telemetry.saved),
    errorTitle: t(($) => $.instance.telemetry.errorTitle),
    forbiddenMessage: t(($) => $.instance.telemetry.forbidden),
  })

  return (
    <Box>
      <Title order={4} mb="xs">
        {t(($) => $.instance.telemetry.title)}
      </Title>
      <Stack gap="sm">
        <Text c="dimmed" size="sm">
          {t(($) => $.instance.telemetry.description)}
        </Text>
        <TelemetryReportActions schemaUrl={state.telemetrySchemaUrl} />
        <Switch
          label={t(($) => $.instance.telemetry.switchLabel)}
          checked={telemetry.enabled}
          disabled={telemetry.locked || mutation.isPending}
          onChange={(event) => mutation.mutate({ body: { enabled: event.currentTarget.checked } })}
        />
        {telemetry.locked ? (
          <Text size="sm">
            {telemetry.lockedBy === 'do_not_track'
              ? t(($) => $.instance.telemetry.lockedDoNotTrack)
              : t(($) => $.instance.telemetry.lockedEnvironment)}
          </Text>
        ) : null}
      </Stack>
    </Box>
  )
}
