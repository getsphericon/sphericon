import { useTranslation } from 'react-i18next'

import { DetailsBadge } from './DetailsBadge.tsx'

// ScopesBadge shows a credential's scopes in a table cell: their count on a badge, the scopes
// themselves in its popover. The tables of API tokens, Connections and Personal tokens share it.
export function ScopesBadge({ scopes }: { scopes: readonly string[] }) {
  const { t } = useTranslation()

  return (
    <DetailsBadge variant="light" color="gray" details={[scopes.join(', ')]}>
      {t(($) => $.settings.scopesCount, { count: scopes.length })}
    </DetailsBadge>
  )
}
