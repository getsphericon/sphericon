import { useTranslation } from 'react-i18next'

import type { SiteIntegrationResource } from '../../generated/site/types.gen.ts'
import { hasErrorStatus } from '../../utils/apiErrors.ts'
import { credentialsEditor } from './credentialsEditor.ts'
import { credentialsVariant } from './integrationForm.ts'
import { useIntegrationGone } from './integrationGone.ts'

interface CredentialsModalProps {
  workspaceId: string
  integration: SiteIntegrationResource | null
  onClose: () => void
}

// Saves an Integration's credentials again, in an editor session (ADR 0119): new ones, or the
// stored ones (a blank secret), which the provider checks anew. A provider that refused them at a
// send holds the Integration until it accepts them here. Only the config is sent, so the name, the
// limits and the default stay.
export function CredentialsModal({ workspaceId, integration, onClose }: CredentialsModalProps) {
  const { t } = useTranslation()
  const gone = useIntegrationGone(workspaceId, onClose)
  const errorTitle = t(($) => $.settings.integrations.credentials.saveError)
  const editor = integration ? credentialsEditor(credentialsVariant(integration)) : undefined
  if (!editor) {
    return null
  }
  return (
    <editor.EditModal
      id={integration?.id ?? null}
      onClose={onClose}
      title={t(($) => $.settings.integrations.credentials.title, { name: integration?.name ?? '' })}
      // The refusal shows in the form, which stays open to correct it; a provider deleted meanwhile
      // closes it instead.
      onFailed={(error) => {
        if (hasErrorStatus(error, [404])) {
          void gone(errorTitle)(error)
        }
        return true
      }}
    />
  )
}
