import { createFileRoute } from '@tanstack/react-router'

import { AcceptInvitationPage } from '../../pages/invitations/accept.tsx'

export const Route = createFileRoute('/invitations/$token')({
  component: AcceptInvitationPage,
})
