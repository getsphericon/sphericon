import { Stack } from '@mantine/core'
import { IconArrowLeft, IconServer, IconShieldLock, IconUser } from '@tabler/icons-react'
import { useMatchRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { useInstanceState } from '../hooks/useInstanceState.ts'
import { Route as profileRoute } from '../routes/account/index.tsx'
import { Route as instanceRoute } from '../routes/account/instance.tsx'
import { Route as securityRoute } from '../routes/account/security.tsx'
import { Route as indexRoute } from '../routes/index.tsx'
import { NavLinkLink } from './RouterLink.tsx'

// AccountNavbar is the sidebar for the workspace-independent account area:
// a way back to the workspace dashboard plus account sections.
export function AccountNavbar() {
  const { t } = useTranslation()
  const matchRoute = useMatchRoute()
  const instance = useInstanceState()

  return (
    <Stack gap="xs">
      <NavLinkLink
        to={indexRoute.to}
        label={t(($) => $.account.backToDashboard)}
        leftSection={<IconArrowLeft size={18} />}
      />
      <NavLinkLink
        to={profileRoute.to}
        label={t(($) => $.account.profile)}
        leftSection={<IconUser size={18} />}
        active={Boolean(matchRoute({ to: profileRoute.to }))}
      />
      <NavLinkLink
        to={securityRoute.to}
        label={t(($) => $.account.security)}
        leftSection={<IconShieldLock size={18} />}
        active={Boolean(matchRoute({ to: securityRoute.to }))}
      />
      {instance.data ? (
        <NavLinkLink
          to={instanceRoute.to}
          label={t(($) => $.account.instance)}
          leftSection={<IconServer size={18} />}
          active={Boolean(matchRoute({ to: instanceRoute.to }))}
        />
      ) : null}
    </Stack>
  )
}
