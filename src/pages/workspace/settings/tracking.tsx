import { Badge, Button, Card, Code, CopyButton, Group, Stack, Text, Title } from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { AnchorLink } from '../../../components/RouterLink.tsx'
import { siteEventsListOptions } from '../../../generated/site/@tanstack/react-query.gen.ts'
import { Route as activityRoute } from '../../../routes/workspaces/$slug/activity.tsx'
import { Route as trackingRoute } from '../../../routes/workspaces/$slug/settings/tracking.tsx'
import { SettingsFrame } from './SettingsFrame.tsx'

// InstallStatus polls the events feed and tells the user whether their tracker
// is connected yet. Accurate only because the dashboard no longer self-tracks
// into the workspace (see App.tsx) — every event here comes from their own site.
function InstallStatus({ slug }: { slug: string }) {
  const { t } = useTranslation()
  const statusQuery = useQuery({
    ...siteEventsListOptions({
      path: { slug },
      query: { page: 1, pageSize: 1 },
    }),
    refetchInterval: 5000,
  })

  const totalItems = statusQuery.data?.totalItems ?? 0
  const received = totalItems > 0

  return (
    <Group mt="md" gap="sm">
      <Text fw={600}>{t(($) => $.settings.installStatusTitle)}:</Text>
      {received ? (
        <Badge color="teal" variant="light">
          {t(($) => $.settings.installStatusReceived)} ({totalItems})
        </Badge>
      ) : (
        <Badge color="gray" variant="light">
          {t(($) => $.settings.installStatusWaiting)}
        </Badge>
      )}
      <AnchorLink to={activityRoute.to} params={{ slug }} size="sm">
        {t(($) => $.settings.viewActivity)}
      </AnchorLink>
    </Group>
  )
}

// TestEvent shows a copy-paste curl command that posts a real event through the
// /collect pipeline (RudderStack-style): the user runs it, then watches it land
// in the activity feed. No in-app injection, so test traffic stays explicit.
function TestEvent({ collectKey }: { collectKey: string }) {
  const { t } = useTranslation()
  const origin = window.location.origin
  const command = [
    `curl -X POST "${origin}/collect/events" \\`,
    `  -H "content-type: application/json" \\`,
    `  -H "x-collect-key: ${collectKey}" \\`,
    `  -d '{"events":[{"visitorId":"test","action":"test.event"}]}'`,
  ].join('\n')

  return (
    <Card withBorder>
      <Title order={4} mb="xs">
        {t(($) => $.settings.testEventTitle)}
      </Title>
      <Text c="dimmed" size="sm" mb="sm">
        {t(($) => $.settings.testEventDescription)}
      </Text>
      <Code block>{command}</Code>
      <CopyButton value={command}>
        {({ copied, copy }) => (
          <Button mt="sm" variant="light" onClick={copy}>
            {copied ? t(($) => $.settings.copied) : t(($) => $.settings.copy)}
          </Button>
        )}
      </CopyButton>
    </Card>
  )
}

// TrackingSection shows the embed snippet customers paste into their site. The
// host is derived from the current origin; the collect key identifies the workspace.
function TrackingSection({ collectKey, slug }: { collectKey: string; slug: string }) {
  const { t } = useTranslation()
  const origin = window.location.origin
  const snippet = `<script async src="${origin}/t.js" data-collect-key="${collectKey}" data-collect-url="${origin}"></script>`

  return (
    <Card withBorder>
      <Title order={4} mb="xs">
        {t(($) => $.settings.trackingTitle)}
      </Title>
      <Text c="dimmed" size="sm" mb="sm">
        {t(($) => $.settings.trackingDescription)}
      </Text>
      <Code block>{snippet}</Code>
      <CopyButton value={snippet}>
        {({ copied, copy }) => (
          <Button mt="sm" variant="light" onClick={copy}>
            {copied ? t(($) => $.settings.copied) : t(($) => $.settings.copy)}
          </Button>
        )}
      </CopyButton>
      <InstallStatus slug={slug} />
    </Card>
  )
}

export function TrackingPage() {
  const { t } = useTranslation()
  const { slug } = trackingRoute.useParams()

  return (
    <SettingsFrame slug={slug} title={t(($) => $.settings.nav.tracking)}>
      {(workspace) => (
        <Stack>
          <TrackingSection collectKey={workspace.collectKey} slug={workspace.slug} />
          <TestEvent collectKey={workspace.collectKey} />
        </Stack>
      )}
    </SettingsFrame>
  )
}
