import { NavLink, Stack } from '@mantine/core'
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
import { useMatchRoute, useNavigate } from '@tanstack/react-router'
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

// AppNavbar renders the workspace-scoped sidebar. Active sections are built
// real now; roadmap sections are disabled placeholders that signal direction.
export function AppNavbar({ slug }: { slug: string }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const matchRoute = useMatchRoute()

  const sections = [
    {
      key: 'overview',
      label: t(($) => $.nav.overview),
      icon: <IconLayoutDashboard size={18} />,
      active: Boolean(matchRoute({ to: overviewRoute.to, params: { slug } })),
      onClick: () => navigate({ to: overviewRoute.to, params: { slug } }),
    },
    {
      key: 'contacts',
      label: t(($) => $.nav.contacts),
      icon: <IconUsers size={18} />,
      active: Boolean(matchRoute({ to: contactsRoute.to, params: { slug }, fuzzy: true })),
      onClick: () => navigate({ to: contactsRoute.to, params: { slug } }),
    },
    {
      key: 'segments',
      label: t(($) => $.nav.segments),
      icon: <IconUsersGroup size={18} />,
      active: Boolean(matchRoute({ to: segmentsRoute.to, params: { slug }, fuzzy: true })),
      onClick: () => navigate({ to: segmentsRoute.to, params: { slug } }),
    },
    {
      key: 'broadcasts',
      label: t(($) => $.nav.broadcasts),
      icon: <IconMailbox size={18} />,
      active: Boolean(matchRoute({ to: broadcastsRoute.to, params: { slug }, fuzzy: true })),
      onClick: () => navigate({ to: broadcastsRoute.to, params: { slug } }),
    },
    {
      key: 'templates',
      label: t(($) => $.nav.templates),
      icon: <IconTemplate size={18} />,
      active: Boolean(matchRoute({ to: templatesRoute.to, params: { slug }, fuzzy: true })),
      onClick: () => navigate({ to: templatesRoute.to, params: { slug } }),
    },
    {
      key: 'automations',
      label: t(($) => $.nav.automations),
      icon: <IconRobot size={18} />,
      active: Boolean(matchRoute({ to: automationsRoute.to, params: { slug }, fuzzy: true })),
      onClick: () => navigate({ to: automationsRoute.to, params: { slug } }),
    },
    {
      key: 'transactional-emails',
      label: t(($) => $.nav.transactionalEmails),
      icon: <IconMailFast size={18} />,
      active: Boolean(
        matchRoute({ to: transactionalEmailsRoute.to, params: { slug }, fuzzy: true }),
      ),
      onClick: () => navigate({ to: transactionalEmailsRoute.to, params: { slug } }),
    },
    {
      key: 'activity',
      label: t(($) => $.nav.activity),
      icon: <IconActivity size={18} />,
      active: Boolean(matchRoute({ to: activityRoute.to, params: { slug }, fuzzy: true })),
      onClick: () => navigate({ to: activityRoute.to, params: { slug } }),
    },
    {
      key: 'settings',
      label: t(($) => $.nav.settings),
      icon: <IconSettings size={18} />,
      active: Boolean(matchRoute({ to: settingsRoute.to, params: { slug }, fuzzy: true })),
      onClick: () => navigate({ to: settingsRoute.to, params: { slug } }),
    },
  ]

  return (
    <Stack gap="xs">
      {sections.map((s) => (
        <NavLink
          key={s.key}
          label={s.label}
          leftSection={s.icon}
          active={s.active}
          onClick={s.onClick}
        />
      ))}
    </Stack>
  )
}
