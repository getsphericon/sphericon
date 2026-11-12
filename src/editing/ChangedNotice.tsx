import { Alert, Button, Group, Text } from '@mantine/core'
import { useTranslation } from 'react-i18next'

import { useConfirmation } from '../hooks/useConfirmation.tsx'

// ChangedNotice tells the person that the resource changed since the editor showed it (ADR 0119):
// a save was refused with revision_changed, or a refresh brought a newer revision. Nothing is
// merged: the content stays as it is, unsaved, until they load the saved version (asked first over
// unsaved edits, in the leave dialog's words) or save theirs over it.
export function ChangedNotice({
  isDirty,
  busy,
  onLoad,
  onOverwrite,
}: {
  // Whether the editor holds unsaved edits, read when Load is chosen.
  isDirty: () => boolean
  busy: boolean
  onLoad: () => void
  onOverwrite: () => void
}) {
  const { t } = useTranslation()
  const confirm = useConfirmation()
  const load = () => {
    if (!isDirty()) {
      onLoad()
      return
    }
    confirm({
      title: t(($) => $.changed.loadUnsavedTitle),
      description: t(($) => $.leave.message),
      confirmLabel: t(($) => $.changed.load),
      cancelLabel: t(($) => $.leave.stay),
      onConfirm: onLoad,
    })
  }
  return (
    <Alert color="yellow" title={t(($) => $.changed.title)} role="alert">
      <Text size="sm" mb="sm">
        {t(($) => $.changed.message)}
      </Text>
      <Group gap="xs">
        <Button size="xs" variant="default" disabled={busy} onClick={load}>
          {t(($) => $.changed.load)}
        </Button>
        <Button size="xs" disabled={busy} onClick={onOverwrite}>
          {t(($) => $.changed.overwrite)}
        </Button>
      </Group>
    </Alert>
  )
}
