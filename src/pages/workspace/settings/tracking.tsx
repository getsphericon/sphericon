import {
  Anchor,
  Box,
  Badge,
  Button,
  Code,
  CopyButton,
  Group,
  Stack,
  Text,
  Title,
} from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { AnchorLink } from '../../../components/RouterLink.tsx'
import { siteEventsListOptions } from '../../../generated/site/@tanstack/react-query.gen.ts'
import { Route as recentActivityRoute } from '../../../routes/workspaces/$workspaceId/analytics/recent-activity.tsx'
import { Route as trackingRoute } from '../../../routes/workspaces/$workspaceId/settings/tracking.tsx'
import { DOCS_URL, SDK_SOURCE_URL } from '../../../utils/docs.ts'
import { SettingsFrame } from './SettingsFrame.tsx'

// InstallStatus polls the events feed and tells the user whether their tracker
// is connected yet. Accurate only because the dashboard no longer self-tracks
// into the workspace (see App.tsx) — every event here comes from their own site.
function InstallStatus({ workspaceId }: { workspaceId: string }) {
  const { t } = useTranslation()
  const statusQuery = useQuery({
    ...siteEventsListOptions({
      path: { workspaceId },
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
      <AnchorLink to={recentActivityRoute.to} params={{ workspaceId }} size="sm">
        {t(($) => $.settings.viewActivity)}
      </AnchorLink>
    </Group>
  )
}

// trackerOrigin is the tracker's public origin without a trailing slash, so paths append cleanly.
function trackerOrigin(trackerUrl: string) {
  return trackerUrl.replace(/\/$/, '')
}

// TestEvent shows a copy-paste curl command that posts a real event through the
// /collect pipeline (RudderStack-style): the user runs it, then watches it land
// in the activity feed. No in-app injection, so test traffic stays explicit.
function TestEvent({ collectKey, trackerUrl }: { collectKey: string; trackerUrl: string }) {
  const { t } = useTranslation()
  const origin = trackerOrigin(trackerUrl)
  const command = [
    `curl -X POST "${origin}/collect/events" \\`,
    `  -H "content-type: application/json" \\`,
    `  -H "x-collect-key: ${collectKey}" \\`,
    `  -d '{"events":[{"visitorId":"test","action":"test.event"}]}'`,
  ].join('\n')

  return (
    <Box>
      <Title order={4} mb="xs">
        {t(($) => $.settings.testEventTitle)}
      </Title>
      <Text c="dimmed" size="sm" mb="sm">
        {t(($) => $.settings.testEventDescription)}
      </Text>
      <Code block>{command}</Code>
      <CopyButton value={command}>
        {({ copied, copy }) => (
          <Button size="xs" mt="sm" variant="light" onClick={copy}>
            {copied ? t(($) => $.settings.copied) : t(($) => $.settings.copy)}
          </Button>
        )}
      </CopyButton>
    </Box>
  )
}

// TrackingSection shows the embed snippet customers paste into their site. The
// host is the instance's tracker origin; the collect key identifies the workspace.
function TrackingSection({
  collectKey,
  trackerUrl,
  workspaceId,
}: {
  collectKey: string
  trackerUrl: string
  workspaceId: string
}) {
  const { t } = useTranslation()
  const origin = trackerOrigin(trackerUrl)
  // The stub creates the command queue the tracker drains once t.js has loaded, so
  // `_omq.push(['track', …])` works from the first line while t.js loads asynchronously.
  const snippet = `<script>
  window._omq = window._omq || [];
  _omq.push(['init', { collectKey: ${JSON.stringify(collectKey)}, baseUrl: ${JSON.stringify(origin)} }]);
  (function () {
    var s = document.createElement('script');
    s.async = true;
    s.src = ${JSON.stringify(`${origin}/t.js`)};
    document.head.appendChild(s);
  })();
</script>`

  return (
    <Box>
      <Title order={4} mb="xs">
        {t(($) => $.settings.trackingTitle)}
      </Title>
      <Text c="dimmed" size="sm" mb="sm">
        {t(($) => $.settings.trackingDescription)}
      </Text>
      <Code block>{snippet}</Code>
      <CopyButton value={snippet}>
        {({ copied, copy }) => (
          <Button size="xs" mt="sm" variant="light" onClick={copy}>
            {copied ? t(($) => $.settings.copied) : t(($) => $.settings.copy)}
          </Button>
        )}
      </CopyButton>
      <InstallStatus workspaceId={workspaceId} />
    </Box>
  )
}

// SendEvents points to the ways of sending events beyond the snippet: the guide and the SDK sources.
function SendEvents() {
  const { t } = useTranslation()

  return (
    <Box>
      <Title order={4} mb="xs">
        {t(($) => $.settings.sendEventsTitle)}
      </Title>
      <Text c="dimmed" size="sm" mb="sm">
        {t(($) => $.settings.sendEventsDescription)}
      </Text>
      <Stack gap="xs">
        <Anchor href={`${DOCS_URL}/guide/tracking`} target="_blank" rel="noreferrer" size="sm">
          {t(($) => $.settings.sendEventsGuideLink)}
        </Anchor>
        <Anchor href={`${SDK_SOURCE_URL}/analytics`} target="_blank" rel="noreferrer" size="sm">
          {t(($) => $.settings.browserSdkLink)}
        </Anchor>
        <Anchor href={`${SDK_SOURCE_URL}/node`} target="_blank" rel="noreferrer" size="sm">
          {t(($) => $.settings.nodeSdkLink)}
        </Anchor>
      </Stack>
    </Box>
  )
}

export function TrackingPage() {
  const { t } = useTranslation()
  const { workspaceId } = trackingRoute.useParams()

  return (
    <SettingsFrame workspaceId={workspaceId} title={t(($) => $.settings.nav.tracking)}>
      {(workspace) => (
        <Stack gap="xl">
          <TrackingSection
            collectKey={workspace.collectKey}
            trackerUrl={workspace.trackerUrl}
            workspaceId={workspace.id}
          />
          <TestEvent collectKey={workspace.collectKey} trackerUrl={workspace.trackerUrl} />
          <SendEvents />
        </Stack>
      )}
    </SettingsFrame>
  )
}
