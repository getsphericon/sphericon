import { createLazyFileRoute } from '@tanstack/react-router'

import { ProfilePage } from '../../pages/account/profile.tsx'

export const Route = createLazyFileRoute('/account/')({
  component: ProfilePage,
})
