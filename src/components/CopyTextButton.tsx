import { Button, CopyButton } from '@mantine/core'
import type { ButtonProps } from '@mantine/core'
import { useTranslation } from 'react-i18next'

interface CopyTextButtonProps extends Omit<ButtonProps, 'children'> {
  // What a click puts on the clipboard.
  value: string
  // The button's text, "Copy" unless given.
  label?: string
  // The text for the moment after a click, "Copied" unless given.
  copiedLabel?: string
  'aria-label'?: string
}

// CopyTextButton is a light button that copies a value and says so for a moment after the click.
export function CopyTextButton({
  value,
  label,
  copiedLabel,
  variant = 'light',
  ...button
}: CopyTextButtonProps) {
  const { t } = useTranslation()
  return (
    <CopyButton value={value}>
      {({ copied, copy }) => (
        <Button variant={variant} onClick={copy} {...button}>
          {copied
            ? (copiedLabel ?? t(($) => $.settings.copied))
            : (label ?? t(($) => $.settings.copy))}
        </Button>
      )}
    </CopyButton>
  )
}
