import { useTranslation } from 'react-i18next'

import type { SiteIntegrationResource } from '../../generated/site/types.gen.ts'
import { hasErrorStatus } from '../../utils/apiErrors.ts'
import { useIntegrationGone } from './integrationGone.ts'
import { providerLimitEditor } from './providerLimitEditor.ts'

interface ProviderLimitModalProps {
  workspaceId: string
  integration: SiteIntegrationResource | null
  onClose: () => void
}

// Edits an Integration's two manual Provider limit values in an editor session (ADR 0119). Only the
// limits are sent, so the stored credentials are untouched; a blank input clears that value, except
// on an SES-compatible service, which keeps both.
export function ProviderLimitModal({ workspaceId, integration, onClose }: ProviderLimitModalProps) {
  const { t } = useTranslation()
  const gone = useIntegrationGone(workspaceId, onClose)
  const errorTitle = t(($) => $.settings.integrations.providerLimit.saveError)
  return (
    <providerLimitEditor.EditModal
      id={integration?.id ?? null}
      onClose={onClose}
      title={t(($) => $.settings.integrations.providerLimit.editTitle, {
        name: integration?.name ?? '',
      })}
      // An Integration deleted while the modal was open closes it and reloads the list.
      onFailed={(error) => {
        if (!hasErrorStatus(error, [404])) {
          return false
        }
        void gone(errorTitle)(error)
        return true
      }}
    />
  )
}
