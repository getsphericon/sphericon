import { Select } from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { siteIntegrationsListOptions } from '../../generated/site/@tanstack/react-query.gen.ts'
import type { IntegrationChannel } from '../../generated/site/types.gen.ts'
import { Route as workspaceRoute } from '../../routes/workspaces/$workspaceId/route.tsx'

// The picker's value for "no Integration named", which sends through the Channel's default. The
// Step's draft holds '' for it.
const DEFAULT_INTEGRATION = 'default'

// IntegrationControl is the picker of the Integration a send step on the Channel sends through:
// the Channel's kept Integrations, or its default (ADR 0081).
export function IntegrationControl({
  channel,
  value,
  onChange,
}: {
  channel: IntegrationChannel
  value: string
  onChange: (integrationId: string) => void
}) {
  const { t } = useTranslation()
  const { workspaceId } = workspaceRoute.useParams()
  const integrations = useQuery(siteIntegrationsListOptions({ path: { workspaceId } }))

  return (
    <Select
      label={t(($) => $.automations.integrationLabel)}
      data={[
        { value: DEFAULT_INTEGRATION, label: t(($) => $.automations.integrationDefault) },
        ...(integrations.data ?? [])
          .filter((integration) => integration.channel === channel)
          .map((integration) => ({ value: integration.id, label: integration.name })),
      ]}
      allowDeselect={false}
      value={value === '' ? DEFAULT_INTEGRATION : value}
      onChange={(picked) =>
        onChange(picked === null || picked === DEFAULT_INTEGRATION ? '' : picked)
      }
    />
  )
}
