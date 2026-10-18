import { createFileRoute } from '@tanstack/react-router'

import { ProfilePage } from '../../pages/account/profile.tsx'

export const Route = createFileRoute('/account/')({
  component: ProfilePage,
})
