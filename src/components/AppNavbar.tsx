import { Stack } from '@mantine/core'
import {
  IconActivity,
  IconLayoutDashboard,
  IconMailbox,
  IconMailFast,
  IconRobot,
  IconSettings,
  IconTemplate,
  IconUsers,
  IconUsersGroup,
} from '@tabler/icons-react'
import { useMatchRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { Route as activityRoute } from '../routes/workspaces/$slug/activity.tsx'
import { Route as automationsRoute } from '../routes/workspaces/$slug/automations/index.tsx'
import { Route as broadcastsRoute } from '../routes/workspaces/$slug/broadcasts/index.tsx'
import { Route as contactsRoute } from '../routes/workspaces/$slug/contacts/index.tsx'
import { Route as overviewRoute } from '../routes/workspaces/$slug/index.tsx'
import { Route as segmentsRoute } from '../routes/workspaces/$slug/segments/index.tsx'
import { Route as settingsRoute } from '../routes/workspaces/$slug/settings.tsx'
import { Route as templatesRoute } from '../routes/workspaces/$slug/templates/index.tsx'
import { Route as transactionalEmailsRoute } from '../routes/workspaces/$slug/transactional-emails/index.tsx'
import { NavLinkLink } from './RouterLink.tsx'

// AppNavbar renders the workspace-scoped sidebar: one link per section, active on the
// section's route or anything under it (the overview only on itself).
export function AppNavbar({ slug }: { slug: string }) {
  const { t } = useTranslation()
  const matchRoute = useMatchRoute()

  const sections = [
    {
      route: overviewRoute,
      label: t(($) => $.nav.overview),
      icon: <IconLayoutDashboard size={18} />,
      fuzzy: false,
    },
    {
      route: contactsRoute,
      label: t(($) => $.nav.contacts),
      icon: <IconUsers size={18} />,
      fuzzy: true,
    },
    {
      route: segmentsRoute,
      label: t(($) => $.nav.segments),
      icon: <IconUsersGroup size={18} />,
      fuzzy: true,
    },
    {
      route: broadcastsRoute,
      label: t(($) => $.nav.broadcasts),
      icon: <IconMailbox size={18} />,
      fuzzy: true,
    },
    {
      route: templatesRoute,
      label: t(($) => $.nav.templates),
      icon: <IconTemplate size={18} />,
      fuzzy: true,
    },
    {
      route: automationsRoute,
      label: t(($) => $.nav.automations),
      icon: <IconRobot size={18} />,
      fuzzy: true,
    },
    {
      route: transactionalEmailsRoute,
      label: t(($) => $.nav.transactionalEmails),
      icon: <IconMailFast size={18} />,
      fuzzy: true,
    },
    {
      route: activityRoute,
      label: t(($) => $.nav.activity),
      icon: <IconActivity size={18} />,
      fuzzy: true,
    },
    {
      route: settingsRoute,
      label: t(($) => $.nav.settings),
      icon: <IconSettings size={18} />,
      fuzzy: true,
    },
  ]

  return (
    <Stack gap="xs">
      {sections.map(({ route, label, icon, fuzzy }) => (
        <NavLinkLink
          key={route.to}
          to={route.to}
          params={{ slug }}
          label={label}
          leftSection={icon}
          active={Boolean(matchRoute({ to: route.to, params: { slug }, fuzzy }))}
        />
      ))}
    </Stack>
  )
}
