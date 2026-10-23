import { createLazyFileRoute } from '@tanstack/react-router'

import { AcceptInvitationPage } from '../../pages/invitations/accept.tsx'

export const Route = createLazyFileRoute('/invitations/$token')({
  component: AcceptInvitationPage,
})
