import { useEffect } from 'react'
import type { RefObject } from 'react'

import type { SchemaForm } from '../../resources/schemaForm.ts'

// Checks every field of the open form as a submit does, showing each error under its field and
// focusing the first: the editor's Save asks it before it sends.
export type StepValidator = RefObject<(() => boolean) | null>

// useValidator hands the editor the check of the open form, for as long as it is open: the fields
// given (a fork's, one per path), else every top-level field.
export function useValidator<TValues extends object, TOutput>(
  form: SchemaForm<TValues, TOutput>,
  validatorRef: StepValidator,
  fields?: string[],
) {
  const checked = fields?.join(',')
  useEffect(() => {
    validatorRef.current = () =>
      form.validateFields(
        checked === undefined ? Object.keys(form.getValues()) : checked.split(','),
      )
    return () => {
      validatorRef.current = null
    }
  }, [form, validatorRef, checked])
}
