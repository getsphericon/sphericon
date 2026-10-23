import { createLazyFileRoute } from '@tanstack/react-router'

import { OAuthConsentPage } from '../../pages/oauth/consent.tsx'

export const Route = createLazyFileRoute('/oauth/consent')({
  component: OAuthConsentPage,
})
