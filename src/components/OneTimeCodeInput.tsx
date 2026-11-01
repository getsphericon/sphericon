import { Anchor, Input, PinInput, TextInput } from '@mantine/core'
import { useId } from '@mantine/hooks'
import type { ReactNode } from 'react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

// CODE_LENGTH is the length of an authenticator app's code (internal/otpcode).
const CODE_LENGTH = 6

// The link sits under the field, below its error.
const order: ('label' | 'input' | 'error' | 'description')[] = [
  'label',
  'input',
  'error',
  'description',
]

interface Props {
  label: string
  value?: string
  onChange: (value: string) => void
  error?: ReactNode
  // data-autofocus marks the first box for the focus of the page or modal, as on any field.
  'data-autofocus'?: boolean
  // recovery lets the field take a Recovery code instead: a link swaps the boxes for a text field.
  recovery?: boolean
}

// OneTimeCodeInput is the code of an authenticator app, one box per digit. The label names the
// first box, which also takes a pasted or autofilled code whole.
export function OneTimeCodeInput({
  label,
  value = '',
  onChange,
  error,
  'data-autofocus': dataAutofocus,
  recovery,
}: Props) {
  const { t } = useTranslation()
  const id = useId()
  const [byRecoveryCode, setByRecoveryCode] = useState(false)

  const toggle = recovery ? (
    <Anchor
      component="button"
      type="button"
      size="sm"
      onClick={() => {
        setByRecoveryCode(!byRecoveryCode)
        onChange('')
      }}
    >
      {byRecoveryCode
        ? t(($) => $.oneTimeCode.useAppCode)
        : t(($) => $.oneTimeCode.useRecoveryCode)}
    </Anchor>
  ) : null

  if (byRecoveryCode) {
    return (
      <TextInput
        label={t(($) => $.oneTimeCode.recoveryLabel)}
        description={toggle}
        inputWrapperOrder={order}
        value={value}
        onChange={(e) => onChange(e.currentTarget.value)}
        error={error}
        autoComplete="off"
        required
        data-autofocus
      />
    )
  }

  return (
    <Input.Wrapper
      label={label}
      id={`${id}-1`}
      description={toggle}
      inputWrapperOrder={order}
      error={error}
      required
    >
      <PinInput
        id={id}
        length={CODE_LENGTH}
        type="number"
        oneTimeCode
        value={value}
        onChange={onChange}
        error={Boolean(error)}
        getInputProps={(index) =>
          index === 0
            ? { 'aria-label': undefined, 'data-autofocus': dataAutofocus }
            : { 'aria-label': t(($) => $.oneTimeCode.digit, { n: index + 1 }) }
        }
      />
    </Input.Wrapper>
  )
}
