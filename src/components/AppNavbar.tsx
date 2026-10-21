import { NavLink, Stack } from '@mantine/core'
import {
  IconActivity,
  IconChartBar,
  IconLayoutDashboard,
  IconMail,
  IconMailbox,
  IconMailFast,
  IconRobot,
  IconSettings,
  IconTemplate,
  IconUsers,
  IconUsersGroup,
} from '@tabler/icons-react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Route as activityRoute } from '../routes/workspaces/$workspaceId/activity.tsx'
import { Route as analyticsRoute } from '../routes/workspaces/$workspaceId/analytics.tsx'
import { Route as automationsRoute } from '../routes/workspaces/$workspaceId/automations/index.tsx'
import { Route as broadcastsRoute } from '../routes/workspaces/$workspaceId/broadcasts/index.tsx'
import { Route as contactsRoute } from '../routes/workspaces/$workspaceId/contacts/index.tsx'
import { Route as overviewRoute } from '../routes/workspaces/$workspaceId/index.tsx'
import { Route as segmentsRoute } from '../routes/workspaces/$workspaceId/segments/index.tsx'
import { Route as settingsRoute } from '../routes/workspaces/$workspaceId/settings/index.tsx'
import { Route as templatesRoute } from '../routes/workspaces/$workspaceId/templates/index.tsx'
import { Route as transactionalEmailsRoute } from '../routes/workspaces/$workspaceId/transactional-emails/index.tsx'
import { NavLinkLink } from './RouterLink.tsx'

// AppNavbar renders the workspace-scoped sidebar: one link per section, active on the
// section's route or anything under it (the overview only on itself). The router's own active
// state drives the highlight: Mantine styles `aria-current="page"`, which the router sets.
export function AppNavbar({ workspaceId }: { workspaceId: string }) {
  const { t } = useTranslation()

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

  // Email-only sections live under one group, open by default: the other channels (SMS, loyalty,
  // forms, bots) get groups of their own beside it. Automations stay outside: they will span
  // channels.
  const emails = [
    { route: broadcastsRoute, label: t(($) => $.nav.broadcasts), icon: <IconMailbox size={18} /> },
    { route: templatesRoute, label: t(($) => $.nav.templates), icon: <IconTemplate size={18} /> },
    {
      route: transactionalEmailsRoute,
      label: t(($) => $.nav.transactionalEmails),
      icon: <IconMailFast size={18} />,
    },
  ]

  return (
    <Stack gap="xs">
      {link({
        route: overviewRoute,
        label: t(($) => $.nav.overview),
        icon: <IconLayoutDashboard size={18} />,
        exact: true,
      })}
      {link({
        route: contactsRoute,
        label: t(($) => $.nav.contacts),
        icon: <IconUsers size={18} />,
      })}
      {link({
        route: segmentsRoute,
        label: t(($) => $.nav.segments),
        icon: <IconUsersGroup size={18} />,
      })}
      <NavLink label={t(($) => $.nav.emails)} leftSection={<IconMail size={18} />} defaultOpened>
        {emails.map((item) => link(item))}
      </NavLink>
      {link({
        route: automationsRoute,
        label: t(($) => $.nav.automations),
        icon: <IconRobot size={18} />,
      })}
      {link({
        route: analyticsRoute,
        label: t(($) => $.nav.analytics),
        icon: <IconChartBar size={18} />,
      })}
      {link({
        route: activityRoute,
        label: t(($) => $.nav.activity),
        icon: <IconActivity size={18} />,
      })}
      {link({
        route: settingsRoute,
        label: t(($) => $.nav.settings),
        icon: <IconSettings size={18} />,
      })}
    </Stack>
  )
}
