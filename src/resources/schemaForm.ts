import { schemaResolver, useForm } from '@mantine/form'
import { omit } from 'es-toolkit'
import { useEffect, useId, useRef } from 'react'
import type { FocusEvent } from 'react'
import * as z from 'zod'

import { getFieldErrors } from '../utils/apiErrors.ts'

// The messages a problem lists by field (its `errors` member).
type FieldErrors = NonNullable<ReturnType<typeof getFieldErrors>>

// The form useSchemaForm returns. @mantine/form's own form type is not importable (its .d.mts
// re-exports a missing file), so it is derived from the hook; the members typed by the
// validation rules are left out, they are the hook's business.
export type SchemaForm<TValues extends object, TOutput> = Omit<
  ReturnType<typeof useForm<TValues, TOutput>>,
  'validate' | 'validateField' | 'isValid'
> & {
  showFieldErrors: (error: unknown) => FieldErrors | undefined
  validateFields: (paths: string[]) => boolean
}

// useSchemaForm is a form validated by a zod schema built from the generated (TypeSpec) ones, so
// the contract is the one source of its rules; a submit hands over the schema's output. The
// messages come from the zod error map in src/i18n.ts.
//
// Validation follows docs/standards/ui.md: a field is not validated when the form opens, it is
// validated when it loses focus, an error that shows is re-checked on each keystroke, and a failed
// submit focuses the first invalid field in the page's order. A field that loses focus to a mouse
// press on its own form's submit button is not validated then but when the press is over: the error
// would push the button down and the press would come up beside it, with no click and no submit.
// The press may end with no submit (it is dragged off the button, the browser cancels it, or the
// browser's own validation of a required field stops it), so the field is checked when the press
// ends, unless the form was submitted: the submit has checked it, and a form that sets errors of
// its own on submit must keep them.
//
// A refusal from the server follows the same rules (docs/standards/ui.md, Errors): showFieldErrors
// puts each message of the problem's `errors` under the field the form renders for its path, and
// focuses the first of them in the page's order. It hands back what it could not place: the
// messages of paths the form does not render, or an empty object when the problem names no field,
// so the page decides how they show. Undefined means every refusal shows under its field.
//
// validateFields checks a part of the form as a submit checks the whole (a step of a form shown
// one screen at a time): it shows the errors of the paths given, focuses the first of them in the
// page's order and answers whether they are all valid.
export function useSchemaForm<TValues extends object, TOutput>(
  schema: z.ZodType<TOutput, TValues>,
  initialValues: TValues,
): SchemaForm<TValues, TOutput> {
  // A name of its own scopes the form's fields (their data-path) to it, so finding an invalid
  // field never lands on another form's field with the same path. Mantine takes letters, digits and
  // dashes only, so each other character of the React id becomes a dash, which keeps ids apart.
  const name = `form${useId().replaceAll(/[^a-zA-Z0-9-]/g, '-')}`
  // The submit button the main mouse button is down on, from before the field loses focus to the
  // press coming up. A button is not focused by a press in every browser, so the focus cannot tell.
  const pressedSubmit = useRef<HTMLButtonElement | HTMLInputElement | null>(null)
  // The fields that lost focus to that press, and their form: their blur validation waits for its
  // end and is dropped when the form is submitted first.
  const skippedBlurs = useRef(new Set<string>())
  const skippedForm = useRef<HTMLFormElement | null>(null)
  const validateField = useRef<(path: string) => unknown>(() => undefined)
  useEffect(() => {
    let timer: ReturnType<typeof setTimeout> | undefined
    const down = (event: PointerEvent) => {
      const target = event.target instanceof Element ? event.target : null
      const control =
        event.pointerType === 'mouse' && event.button === 0
          ? target?.closest<HTMLButtonElement | HTMLInputElement>('button, input')
          : null
      // A button with no type submits; an image input submits too.
      pressedSubmit.current =
        control?.type === 'submit' || control?.type === 'image' ? control : null
    }
    const up = () => {
      pressedSubmit.current = null
      if (skippedBlurs.current.size === 0) {
        return
      }
      // After the click that ends the press, so an error that shifts the layout cannot take it.
      timer = setTimeout(() => {
        const paths = [...skippedBlurs.current]
        skippedBlurs.current.clear()
        for (const path of paths) {
          validateField.current(path)
        }
      }, 0)
    }
    // A capture listener sees the submit whether or not the form goes through form.onSubmit.
    const submitted = (event: SubmitEvent) => {
      if (event.target === skippedForm.current) {
        skippedBlurs.current.clear()
      }
    }
    document.addEventListener('pointerdown', down, true)
    document.addEventListener('pointerup', up, true)
    document.addEventListener('pointercancel', up, true)
    document.addEventListener('submit', submitted, true)
    return () => {
      clearTimeout(timer)
      document.removeEventListener('pointerdown', down, true)
      document.removeEventListener('pointerup', up, true)
      document.removeEventListener('pointercancel', up, true)
      document.removeEventListener('submit', submitted, true)
    }
  }, [])
  const form = useForm<TValues, TOutput>({
    name,
    initialValues,
    validate: schemaResolver(schema, { sync: true }),
    transformValues: (values) => schema.parse(values),
    validateInputOnBlur: true,
    enhanceGetInputProps: ({ inputProps, field, form: renderedForm }) => {
      // Mantine assigns what is returned here over inputProps, so the handlers are read first.
      const { onBlur, onChange } = inputProps
      return {
        // A field with no focus handling (a radio) has no onBlur to wrap.
        ...(onBlur && {
          onBlur: (event: FocusEvent<HTMLElement & { form?: HTMLFormElement | null }>) => {
            const pressed = pressedSubmit.current
            if (!pressed?.form || pressed.form !== event.target.form) {
              onBlur(event)
            } else {
              skippedForm.current = pressed.form
              skippedBlurs.current.add(field)
            }
          },
        }),
        // The error showing at render time is the one the keystroke is about to clear.
        ...(renderedForm.errors[field] && {
          onChange: (...args: unknown[]) => {
            onChange(...args)
            renderedForm.validateField(field)
          },
        }),
      }
    },
  })

  useEffect(() => {
    validateField.current = form.validateField
  })

  const onSubmit: typeof form.onSubmit = (handleSubmit, handleValidationFailure) =>
    form.onSubmit(handleSubmit, (errors, values, event) => {
      focusFirstInvalid(form.getInputNode, Object.keys(errors))
      handleValidationFailure?.(errors, values, event)
    })

  const showFieldErrors = (error: unknown) => {
    const fieldErrors = getFieldErrors(error) ?? {}
    const placed = Object.keys(fieldErrors).filter(
      (path) => fieldErrors[path]?.[0] && form.getInputNode(path),
    )
    for (const path of placed) {
      form.setFieldError(path, fieldErrors[path]?.[0])
    }
    focusFirstInvalid(form.getInputNode, placed)
    const unplaced = omit(fieldErrors, placed)
    return placed.length > 0 && Object.keys(unplaced).length === 0 ? undefined : unplaced
  }

  const validateFields = (paths: string[]) => {
    const invalid = paths.filter((path) => {
      const result = form.validateField(path)
      // The resolver is synchronous: no result is a promise.
      return result instanceof Promise || result.hasError
    })
    focusFirstInvalid(form.getInputNode, invalid)
    return invalid.length === 0
  }

  return { ...form, onSubmit, showFieldErrors, validateFields }
}

// focusFirstInvalid focuses the field, among the invalid ones, that comes first in the document:
// the order of the schema's keys is not the order of the page.
function focusFirstInvalid(
  getInputNode: (path: string) => HTMLElement | null,
  paths: string[],
): void {
  const nodes = paths.flatMap((path) => getInputNode(path) ?? [])
  nodes
    .toSorted((a, b) => (a.compareDocumentPosition(b) & Node.DOCUMENT_POSITION_FOLLOWING ? -1 : 1))
    .at(0)
    ?.focus()
}

// trimmed reads a contract string field from a text input: the spaces around what was typed are
// not part of the value, so a blank input is an empty value the field's own rules judge.
export function trimmed<TOutput>(field: z.ZodType<TOutput, string>) {
  return z.string().trim().pipe(field)
}
