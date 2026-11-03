import { Select } from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import type { ControlProps } from '@workflowbuilder/sdk'
import type { ComponentType } from 'react'
import { useTranslation } from 'react-i18next'

import { siteIntegrationsListOptions } from '../../generated/site/@tanstack/react-query.gen.ts'
import type { SiteIntegrationChannel } from '../../generated/site/types.gen.ts'
import { Route as workspaceRoute } from '../../routes/workspaces/$workspaceId/route.tsx'

// The picker's value for "no Integration named", which sends through the Channel's default. The
// node holds '' for it (the form holds only strings).
const DEFAULT_INTEGRATION = 'default'

// integrationControl is the picker of the Integration a send step on the Channel sends through:
// the Channel's kept Integrations, or its default (ADR 0081).
export function integrationControl(channel: SiteIntegrationChannel): ComponentType<ControlProps> {
  return function IntegrationControl(props: ControlProps) {
    const { data, label, path } = props
    const { t } = useTranslation()
    const { workspaceId } = workspaceRoute.useParams()
    const integrations = useQuery(
      siteIntegrationsListOptions({ path: { workspaceId }, query: { archived: false } }),
    )
    const value = typeof data === 'string' && data !== '' ? data : DEFAULT_INTEGRATION

    return (
      <Select
        label={label}
        data={[
          { value: DEFAULT_INTEGRATION, label: t(($) => $.automations.integrationDefault) },
          ...(integrations.data ?? [])
            .filter((integration) => integration.channel === channel)
            .map((integration) => ({ value: integration.id, label: integration.name })),
        ]}
        allowDeselect={false}
        value={value}
        onChange={(picked) =>
          props.handleChange(path, picked === null || picked === DEFAULT_INTEGRATION ? '' : picked)
        }
      />
    )
  }
}
