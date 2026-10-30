import { createLazyFileRoute } from '@tanstack/react-router'

import { LegalDocumentPage } from '../pages/legal/document.tsx'

export const Route = createLazyFileRoute('/legal/$key')({
  component: LegalDocumentPage,
})
