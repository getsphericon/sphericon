import { Badge, NavLink, Stack } from '@mantine/core'
import {
  IconBell,
  IconBolt,
  IconBrowser,
  IconChartBar,
  IconForms,
  IconLayoutBottombar,
  IconLayoutDashboard,
  IconMail,
  IconMailbox,
  IconMailFast,
  IconMailOpened,
  IconPalette,
  IconRobot,
  IconRoute,
  IconSettings,
  IconTag,
  IconTemplate,
  IconUser,
  IconUsers,
  IconUsersGroup,
} from '@tabler/icons-react'
import { useMatchRoute, useSearch } from '@tanstack/react-router'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { zSiteAnalyticsRange } from '../generated/site/zod.gen.ts'
import { Route as clicksByLinkRoute } from '../routes/workspaces/$workspaceId/analytics/clicks-by-link.tsx'
import { Route as emailMetricsRoute } from '../routes/workspaces/$workspaceId/analytics/email-metrics.tsx'
import { Route as eventsRoute } from '../routes/workspaces/$workspaceId/analytics/events.tsx'
import { Route as newPeopleRoute } from '../routes/workspaces/$workspaceId/analytics/new-people.tsx'
import { Route as opensByHourRoute } from '../routes/workspaces/$workspaceId/analytics/opens-by-hour.tsx'
import { Route as recentActivityRoute } from '../routes/workspaces/$workspaceId/analytics/recent-activity.tsx'
import { Route as subscribedPeopleRoute } from '../routes/workspaces/$workspaceId/analytics/subscribed-people.tsx'
import { Route as automationsRoute } from '../routes/workspaces/$workspaceId/automations/index.tsx'
import { Route as brandKitRoute } from '../routes/workspaces/$workspaceId/brand-kit.tsx'
import { Route as broadcastsRoute } from '../routes/workspaces/$workspaceId/broadcasts/index.tsx'
import { Route as capturedMessagesRoute } from '../routes/workspaces/$workspaceId/captured-messages/index.tsx'
import { Route as contactsRoute } from '../routes/workspaces/$workspaceId/contacts/index.tsx'
import { Route as customFieldsRoute } from '../routes/workspaces/$workspaceId/custom-fields/index.tsx'
import { Route as overviewRoute } from '../routes/workspaces/$workspaceId/index.tsx'
import { Route as rulesRoute } from '../routes/workspaces/$workspaceId/rules/index.tsx'
import { Route as segmentsRoute } from '../routes/workspaces/$workspaceId/segments/index.tsx'
import { Route as settingsRoute } from '../routes/workspaces/$workspaceId/settings/index.tsx'
import { Route as tagsRoute } from '../routes/workspaces/$workspaceId/tags/index.tsx'
import { Route as templatesRoute } from '../routes/workspaces/$workspaceId/templates/index.tsx'
import { Route as transactionalEmailsRoute } from '../routes/workspaces/$workspaceId/transactional-emails/index.tsx'
import { NavLinkLink } from './RouterLink.tsx'

// AppNavbar renders the workspace-scoped sidebar: one link per section, active on the
// section's route or anything under it (the overview only on itself). The router's own active
// state drives the highlight: Mantine styles `aria-current="page"`, which the router sets.
export function AppNavbar({ workspaceId }: { workspaceId: string }) {
  const { t } = useTranslation()
  const matchRoute = useMatchRoute()
  // The analytics window rides along from one report to the next; outside analytics it is the default.
  const range = zSiteAnalyticsRange.catch('30d').parse(useSearch({ strict: false }).range)

  const link = ({
    route,
    label,
    icon,
    exact = false,
  }: {
    route: { to: string }
    label: string
    icon: ReactNode
    exact?: boolean
  }) => (
    <NavLinkLink
      key={route.to}
      to={route.to}
      params={{ workspaceId }}
      label={label}
      leftSection={icon}
      activeOptions={{ exact }}
    />
  )

  // A group is closed by default and open when one of its pages is being viewed.
  const viewing = (items: { route: { to: string } }[]) =>
    items.some((item) => matchRoute({ to: item.route.to, params: { workspaceId }, fuzzy: true }))

  // Everything that describes or selects the people a workspace talks to.
  const people = [
    { route: contactsRoute, label: t(($) => $.nav.contacts), icon: <IconUser size={18} /> },
    { route: segmentsRoute, label: t(($) => $.nav.segments), icon: <IconUsersGroup size={18} /> },
    { route: tagsRoute, label: t(($) => $.nav.tags), icon: <IconTag size={18} /> },
    {
      route: customFieldsRoute,
      label: t(($) => $.nav.customFields),
      icon: <IconForms size={18} />,
    },
  ]

  // Email-only sections live under one group: the other channels (SMS, loyalty, forms, bots) get
  // groups of their own beside it. Automations and Rules stay out of it: they span channels.
  const emails = [
    { route: broadcastsRoute, label: t(($) => $.nav.broadcasts), icon: <IconMailbox size={18} /> },
    { route: templatesRoute, label: t(($) => $.nav.templates), icon: <IconTemplate size={18} /> },
    {
      route: transactionalEmailsRoute,
      label: t(($) => $.nav.transactionalEmails),
      icon: <IconMailFast size={18} />,
    },
    { route: brandKitRoute, label: t(($) => $.nav.brandKit), icon: <IconPalette size={18} /> },
    {
      route: capturedMessagesRoute,
      label: t(($) => $.nav.capturedMessages),
      icon: <IconMailOpened size={18} />,
    },
  ]

  // What a workspace does on its own: Automations walk a contact through steps, Rules react to
  // a single Event.
  const workflows = [
    { route: automationsRoute, label: t(($) => $.nav.automations), icon: <IconRobot size={18} /> },
    { route: rulesRoute, label: t(($) => $.nav.rules), icon: <IconBolt size={18} /> },
  ]

  // What a workspace shows on its own site. None of it exists yet: the pages are announced, not linked.
  const website = [
    { label: t(($) => $.nav.forms), icon: <IconForms size={18} /> },
    { label: t(($) => $.nav.popups), icon: <IconLayoutBottombar size={18} /> },
    { label: t(($) => $.nav.banners), icon: <IconBell size={18} /> },
  ]

  // The reports, grouped by what they measure; every link carries the chosen window.
  const reports = [
    { route: newPeopleRoute, label: t(($) => $.analytics.nav.newPeople) },
    { route: subscribedPeopleRoute, label: t(($) => $.analytics.nav.subscribedPeople) },
    { route: emailMetricsRoute, label: t(($) => $.analytics.nav.emailMetrics) },
    { route: clicksByLinkRoute, label: t(($) => $.analytics.nav.clicksByLink) },
    { route: opensByHourRoute, label: t(($) => $.analytics.nav.opensByHour) },
    { route: eventsRoute, label: t(($) => $.analytics.nav.events) },
    { route: recentActivityRoute, label: t(($) => $.analytics.nav.recentActivity) },
  ]

  return (
    <Stack gap="xs">
      {link({
        route: overviewRoute,
        label: t(($) => $.nav.overview),
        icon: <IconLayoutDashboard size={18} />,
        exact: true,
      })}
      <NavLink
        label={t(($) => $.nav.people)}
        leftSection={<IconUsers size={18} />}
        defaultOpened={viewing(people)}
      >
        {people.map((item) => link(item))}
      </NavLink>
      <NavLink
        label={t(($) => $.nav.emails)}
        leftSection={<IconMail size={18} />}
        defaultOpened={viewing(emails)}
      >
        {emails.map((item) => link(item))}
      </NavLink>
      <NavLink
        label={t(($) => $.nav.workflows)}
        leftSection={<IconRoute size={18} />}
        defaultOpened={viewing(workflows)}
      >
        {workflows.map((item) => link(item))}
      </NavLink>
      <NavLink label={t(($) => $.nav.website)} leftSection={<IconBrowser size={18} />}>
        {website.map((item) => (
          <NavLink
            key={item.label}
            label={item.label}
            leftSection={item.icon}
            rightSection={<Badge variant="light">{t(($) => $.nav.soon)}</Badge>}
            disabled
          />
        ))}
      </NavLink>
      <NavLink
        label={t(($) => $.nav.analytics)}
        leftSection={<IconChartBar size={18} />}
        defaultOpened={viewing(reports)}
      >
        {reports.map((item) => (
          <NavLinkLink
            key={item.route.to}
            to={item.route.to}
            params={{ workspaceId }}
            search={{ range }}
            label={item.label}
          />
        ))}
      </NavLink>
      {link({
        route: settingsRoute,
        label: t(($) => $.nav.settings),
        icon: <IconSettings size={18} />,
      })}
    </Stack>
  )
}
