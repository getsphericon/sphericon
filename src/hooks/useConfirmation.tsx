import { Text } from '@mantine/core'
import { modals } from '@mantine/modals'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { TypedConfirmation } from '../components/TypedConfirmation.tsx'

interface ConfirmationOptions {
  title?: ReactNode
  // What the action does and what to do first, in the resource's own words. Required: there is
  // no generic text (docs/standards/ui.md, "Deleting").
  description: ReactNode
  confirmLabel?: ReactNode
  // The additional confirmation: the confirm button stays disabled until this exact name is typed.
  // For a credential or endpoint something live depends on.
  typedName?: string | undefined
  onConfirm: () => void
}

export function useConfirmation() {
  const { t } = useTranslation()

  return ({ title, description, confirmLabel, typedName, onConfirm }: ConfirmationOptions) => {
    const resolvedTitle = title ?? t(($) => $.confirmations.deleteTitle)
    const resolvedLabel = confirmLabel ?? t(($) => $.actions.delete)

    if (typedName === undefined) {
      modals.openConfirmModal({
        title: resolvedTitle,
        children: <Text>{description}</Text>,
        labels: { cancel: t(($) => $.actions.cancel), confirm: resolvedLabel },
        confirmProps: { color: 'red' },
        onConfirm,
      })
      return
    }

    const id = modals.open({
      title: resolvedTitle,
      children: (
        <TypedConfirmation
          warning={description}
          inputLabel={t(($) => $.confirmations.typeToConfirm, { name: typedName })}
          name={typedName}
          confirmLabel={resolvedLabel}
          onCancel={() => modals.close(id)}
          onConfirm={() => {
            modals.close(id)
            onConfirm()
          }}
        />
      ),
    })
  }
}
