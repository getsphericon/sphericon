import { Badge, Tooltip } from '@mantine/core'
import { useTranslation } from 'react-i18next'

// ScopesBadge shows a credential's scopes in a table cell: their count on a badge, the scopes
// themselves in its tooltip. The tables of API tokens, Connections and Personal tokens share it.
export function ScopesBadge({ scopes }: { scopes: readonly string[] }) {
  const { t } = useTranslation()

  return (
    <Tooltip label={scopes.join(', ')} multiline maw={320} withArrow>
      <Badge variant="light" color="gray">
        {t(($) => $.settings.scopesCount, { count: scopes.length })}
      </Badge>
    </Tooltip>
  )
}
