import { Alert } from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { siteWorkspacesListOptions } from '../generated/site/@tanstack/react-query.gen.ts'
import { Route as generalRoute } from '../routes/workspaces/$workspaceId/settings/index.tsx'
import { AnchorLink } from './RouterLink.tsx'

// PostalAddressNotice is the Content check of the Workspace's missing postal address (ADR 0156,
// ADR 0130): marketing email is not scheduled, sent or published without one, so the email editor
// says so before it is refused, with a link to the setting. A Test send still goes out. It renders
// nothing while the Workspace has an address, or until the Workspace is read.
export function PostalAddressNotice({ workspaceId }: { workspaceId: string }) {
  const { t } = useTranslation()
  const { data: workspaces } = useQuery(siteWorkspacesListOptions())
  const workspace = workspaces?.find((w) => w.id === workspaceId)
  if (workspace?.postalAddress.trim() !== '') {
    return null
  }
  return (
    <Alert color="yellow" variant="light">
      {t(($) => $.postalAddressNotice.text)}{' '}
      <AnchorLink to={generalRoute.to} params={{ workspaceId }}>
        {t(($) => $.postalAddressNotice.link)}
      </AnchorLink>
    </Alert>
  )
}
