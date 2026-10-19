import { Stack } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import { Route as sendingRoute } from '../../../routes/workspaces/$slug/settings/sending.tsx'
import { IntegrationsSection } from '../IntegrationsSection.tsx'
import { SendingDomainsSection } from '../SendingDomainsSection.tsx'
import { SuppressionsSection } from '../SuppressionsSection.tsx'
import { SettingsFrame } from './SettingsFrame.tsx'

export function SendingPage() {
  const { t } = useTranslation()
  const { slug } = sendingRoute.useParams()

  return (
    <SettingsFrame slug={slug} title={t(($) => $.settings.nav.sending)}>
      {(workspace) => (
        <Stack>
          <IntegrationsSection slug={workspace.slug} />
          <SendingDomainsSection slug={workspace.slug} />
          <SuppressionsSection slug={workspace.slug} />
        </Stack>
      )}
    </SettingsFrame>
  )
}
