import {
  Anchor,
  Box,
  Badge,
  Button,
  Code,
  CopyButton,
  Group,
  Select,
  Stack,
  Text,
  Title,
} from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { AnchorLink } from '../../../components/RouterLink.tsx'
import {
  siteEventsListOptions,
  siteSendingDomainsListOptions,
} from '../../../generated/site/@tanstack/react-query.gen.ts'
import type { SiteWorkspaceResource } from '../../../generated/site/types.gen.ts'
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

// TestEvent shows a copy-paste curl command that posts a real event through the
// /collect pipeline (RudderStack-style): the user runs it, then watches it land
// in the activity feed. No in-app injection, so test traffic stays explicit.
function TestEvent({ collectKey, trackerUrl }: { collectKey: string; trackerUrl: string }) {
  const { t } = useTranslation()
  const command = [
    `curl -X POST "${trackerUrl}/collect/events" \\`,
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
  // The stub creates the command queue the tracker drains once t.js has loaded, so
  // `_omq.push(['track', …])` works from the first line while t.js loads asynchronously.
  const snippet = `<script>
  window._omq = window._omq || [];
  _omq.push(['init', { collectKey: ${JSON.stringify(collectKey)}, baseUrl: ${JSON.stringify(trackerUrl)} }]);
  (function () {
    var s = document.createElement('script');
    s.async = true;
    s.src = ${JSON.stringify(`${trackerUrl}/t.js`)};
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
  const links = [
    { href: `${DOCS_URL}/guide/tracking`, label: t(($) => $.settings.sendEventsGuideLink) },
    { href: `${SDK_SOURCE_URL}/analytics`, label: t(($) => $.settings.browserSdkLink) },
    { href: `${SDK_SOURCE_URL}/node`, label: t(($) => $.settings.nodeSdkLink) },
  ]

  return (
    <Box>
      <Title order={4} mb="xs">
        {t(($) => $.settings.sendEventsTitle)}
      </Title>
      <Text c="dimmed" size="sm" mb="sm">
        {t(($) => $.settings.sendEventsDescription)}
      </Text>
      <Stack gap="xs">
        {links.map((link) => (
          <Anchor key={link.href} href={link.href} target="_blank" rel="noreferrer" size="sm">
            {link.label}
          </Anchor>
        ))}
      </Stack>
    </Box>
  )
}

const MAX_SENDING_DOMAINS = 100

// TrackerSetup lets the Owner pick the host the snippet is built on: the Instance's tracker origin, or
// the Tracking domain of a Sending domain once it serves (ADR 0059). A host under the site's own
// registrable domain is first-party, which the hint says; the app cannot know the site, so it checks nothing.
function TrackerSetup({ workspace }: { workspace: SiteWorkspaceResource }) {
  const { t } = useTranslation()
  const domainsQuery = useQuery(
    siteSendingDomainsListOptions({
      path: { workspaceId: workspace.id },
      // A Workspace has few Sending domains; one large page lists every serving host.
      query: { archived: false, pageSize: MAX_SENDING_DOMAINS },
    }),
  )
  const [selected, setSelected] = useState(workspace.trackerUrl)

  const hosts = (domainsQuery.data?.items ?? []).flatMap((domain) =>
    domain.trackingRecord?.state === 'serving' ? [`https://${domain.trackingRecord.host}`] : [],
  )
  const options = [workspace.trackerUrl, ...hosts]
  // A host that stopped serving (or was archived) drops out of the list: fall back to the default.
  const trackerUrl = options.includes(selected) ? selected : workspace.trackerUrl

  return (
    <Stack gap="xl">
      {hosts.length > 0 ? (
        <Select
          maw={480}
          label={t(($) => $.settings.trackerHostLabel)}
          description={t(($) => $.settings.trackerHostDescription)}
          data={options}
          value={trackerUrl}
          allowDeselect={false}
          onChange={(value) => setSelected(value ?? '')}
        />
      ) : null}
      <TrackingSection
        collectKey={workspace.collectKey}
        trackerUrl={trackerUrl}
        workspaceId={workspace.id}
      />
      <TestEvent collectKey={workspace.collectKey} trackerUrl={trackerUrl} />
      <SendEvents />
    </Stack>
  )
}

export function TrackingPage() {
  const { t } = useTranslation()
  const { workspaceId } = trackingRoute.useParams()

  return (
    <SettingsFrame workspaceId={workspaceId} title={t(($) => $.settings.nav.tracking)}>
      {(workspace) => <TrackerSetup workspace={workspace} />}
    </SettingsFrame>
  )
}
