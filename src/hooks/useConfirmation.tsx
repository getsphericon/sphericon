import { Text } from '@mantine/core'
import { modals } from '@mantine/modals'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

interface ConfirmationOptions {
  title?: ReactNode
  description?: ReactNode
  confirmLabel?: ReactNode
  onConfirm: () => void
}

export function useConfirmation() {
  const { t } = useTranslation()

  return ({ title, description, confirmLabel, onConfirm }: ConfirmationOptions) => {
    modals.openConfirmModal({
      title: title ?? t(($) => $.confirmations.deleteTitle),
      children: <Text>{description ?? t(($) => $.confirmations.deleteDescription)}</Text>,
      labels: {
        cancel: t(($) => $.actions.cancel),
        confirm: confirmLabel ?? t(($) => $.actions.delete),
      },
      confirmProps: { color: 'red' },
      onConfirm,
    })
  }
}
