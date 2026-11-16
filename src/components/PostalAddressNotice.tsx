import { Alert } from '@mantine/core'
import { useSuspenseQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { siteWorkspacesListOptions } from '../generated/site/@tanstack/react-query.gen.ts'
import { Route as generalRoute } from '../routes/workspaces/$workspaceId/settings/index.tsx'
import { QueryBoundary } from './QueryBoundary.tsx'
import { AnchorLink } from './RouterLink.tsx'

interface PostalAddressNoticeProps {
  workspaceId: string
}

// PostalAddressNotice is the Content check of the Workspace's missing postal address (ADR 0156,
// ADR 0130): marketing email is not scheduled, sent or published without one, so the email editor
// says so before it is refused, with a link to the setting. A Test send still goes out.
export function PostalAddressNotice(props: PostalAddressNoticeProps) {
  const { t } = useTranslation()
  return (
    <QueryBoundary fallback={null} errorTitle={t(($) => $.postalAddressNotice.loadError)}>
      <Notice {...props} />
    </QueryBoundary>
  )
}

// Notice renders nothing while the Workspace has an address.
function Notice({ workspaceId }: PostalAddressNoticeProps) {
  const { t } = useTranslation()
  const { data: workspaces } = useSuspenseQuery(siteWorkspacesListOptions())
  const workspace = workspaces.find((w) => w.id === workspaceId)
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
