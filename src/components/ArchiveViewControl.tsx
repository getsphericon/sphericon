import { SegmentedControl } from '@mantine/core'
import type { SegmentedControlProps } from '@mantine/core'
import { useTranslation } from 'react-i18next'

// ArchiveViewControl switches a list between its live records and its archived ones: two views
// of one list, not a filter laid over it, so the choice reads as a place you are in.
export function ArchiveViewControl({
  archived,
  onChange,
  ...spacing
}: Pick<SegmentedControlProps, 'mb'> & {
  archived: boolean
  onChange: (archived: boolean) => void
}) {
  const { t } = useTranslation()
  return (
    <SegmentedControl
      aria-label={t(($) => $.archive.view)}
      w="fit-content"
      {...spacing}
      value={archived ? 'archived' : 'active'}
      onChange={(value) => onChange(value === 'archived')}
      data={[
        { value: 'active', label: t(($) => $.archive.active) },
        { value: 'archived', label: t(($) => $.archive.archived) },
      ]}
    />
  )
}
