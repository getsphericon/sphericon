import { Alert, Card, Stack, Switch, Text, Title } from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { ApiErrorAlert } from '../components/ApiErrorAlert.tsx'
import { BlockSkeleton } from '../components/Skeletons.tsx'
import { ReleaseAlert } from '../components/UpdateBanner.tsx'
import {
  staffInstanceGetStateOptions,
  staffInstanceGetStateQueryKey,
  staffInstanceSetTelemetryMutation,
} from '../generated/staff/@tanstack/react-query.gen.ts'
import { StaffInstanceTelemetryLock } from '../generated/staff/types.gen.ts'
import type { StaffInstanceState } from '../generated/staff/types.gen.ts'
import { useResourceMutation } from '../hooks/useResourceMutation.ts'
import { formatLongDate } from '../utils/datetime.ts'
import { TelemetryReportActions } from './TelemetryReportActions.tsx'

// The switch of the anonymous Telemetry report (ADR 0032). When the deployment decides (the saas
// Mode or DO_NOT_TRACK), it is read-only and says why.
function TelemetrySection({ state }: { state: StaffInstanceState }) {
  const { t } = useTranslation()
  const { telemetry } = state
  const mutation = useResourceMutation({
    mutation: staffInstanceSetTelemetryMutation(),
    invalidate: [staffInstanceGetStateQueryKey()],
    successMessage: t(($) => $.console.instance.telemetry.saved),
    errorTitle: t(($) => $.console.instance.telemetry.errorTitle),
  })

  return (
    <Card withBorder>
      <Stack gap="sm">
        <Title order={4}>{t(($) => $.console.instance.telemetry.title)}</Title>
        <Text c="dimmed" size="sm">
          {t(($) => $.console.instance.telemetry.description)}
        </Text>
        <TelemetryReportActions schemaUrl={state.telemetrySchemaUrl} />
        <Switch
          label={t(($) => $.console.instance.telemetry.switchLabel)}
          checked={telemetry.enabled}
          disabled={telemetry.locked || mutation.isPending}
          onChange={(event) => mutation.mutate({ body: { enabled: event.currentTarget.checked } })}
        />
        {telemetry.locked ? (
          <Text size="sm">
            {telemetry.lockedBy === StaffInstanceTelemetryLock.DO_NOT_TRACK
              ? t(($) => $.console.instance.telemetry.lockedDoNotTrack)
              : t(($) => $.console.instance.telemetry.lockedEnvironment)}
          </Text>
        ) : null}
      </Stack>
    </Card>
  )
}

// What the Instance tells its Staff member about itself: a newer release the Release check found,
// and a License key that expired (ADR 0035), shown here only, never to a Workspace.
function InstanceAlerts({ state }: { state: StaffInstanceState }) {
  const { t } = useTranslation()
  const expiredOn = state.licenseExpiredAt ? formatLongDate(state.licenseExpiredAt) : null

  return (
    <>
      {expiredOn ? (
        <Alert color="yellow" title={t(($) => $.console.instance.licenseExpired.title)}>
          <Text size="sm">
            {t(($) => $.console.instance.licenseExpired.description, { date: expiredOn })}
          </Text>
        </Alert>
      ) : null}
      {state.latestRelease ? <ReleaseAlert version={state.latestRelease} /> : null}
    </>
  )
}

// The Instance's own settings (ADR 0063), open to every signed-in Staff member.
export function ConsoleInstancePage() {
  const { t } = useTranslation()
  const state = useQuery(staffInstanceGetStateOptions())

  return (
    <Stack maw={720}>
      <Title order={2}>{t(($) => $.console.instance.title)}</Title>
      {state.isPending ? <BlockSkeleton /> : null}
      {state.isError ? (
        <ApiErrorAlert
          error={state.error}
          title={t(($) => $.console.instance.loadError)}
          fallback={t(($) => $.console.instance.loadError)}
        />
      ) : null}
      {state.data ? (
        <>
          <InstanceAlerts state={state.data} />
          <TelemetrySection state={state.data} />
        </>
      ) : null}
    </Stack>
  )
}
