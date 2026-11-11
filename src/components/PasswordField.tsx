import { PasswordInput } from '@mantine/core'
import type { PasswordInputProps } from '@mantine/core'

// PasswordField is the password input of every form. Mantine's PasswordInput leaves aria-invalid
// off its input (withAria: false), so a screen reader would not hear that the password was refused;
// this field marks the input invalid whenever it shows an error.
export function PasswordField(props: PasswordInputProps) {
  return <PasswordInput aria-invalid={props.error ? true : undefined} {...props} />
}
