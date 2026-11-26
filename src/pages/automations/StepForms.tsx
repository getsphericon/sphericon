import {
  Input,
  NumberInput,
  SegmentedControl,
  Stack,
  Text,
  Textarea,
  TextInput,
} from '@mantine/core'
import { useTranslation } from 'react-i18next'

import { HttpHeaderRows } from '../../components/HttpHeaderRows.tsx'
import { SmsContentEditor } from '../../components/sms/SmsContentEditor.tsx'
import { TelegramContentEditor } from '../../components/telegram/TelegramContentEditor.tsx'
import { TelegramUnsubscribeControl } from '../../components/telegram/TelegramUnsubscribeControl.tsx'
import type { WaitUntilMomentInput } from '../../generated/site/types.gen.ts'
import { zWaitUntilMomentInput } from '../../generated/site/zod.gen.ts'
import { useSchemaForm } from '../../resources/schemaForm.ts'
import { EVENT_ACTION_TAG } from '../../utils/mergeTags.ts'
import type { EditedKind, EditedStep } from './definition.ts'
import { HttpDryRun } from './HttpStepControls.tsx'
import { IntegrationControl } from './IntegrationControl.tsx'
import { MessageControl } from './MessageControl.tsx'
import { DateOccurrenceForm, TimeOfDayForm } from './MomentForms.tsx'
import { stepSchemas } from './stepSchemas.ts'
import { useValidator } from './stepValidator.ts'
import type { StepValidator } from './stepValidator.ts'

type StepOf<K extends EditedKind> = Extract<EditedStep, { kind: K }>

interface StepFormProps<K extends EditedKind> {
  step: StepOf<K>
  // Every edit of the form, as the Step with its new draft: the flow holds what is saved.
  onEdit: (step: StepOf<K>) => void
  validatorRef: StepValidator
}

// What a Step whose node shows its errors is also given: whether it was checked (on blur or on
// Save), and what marks it checked.
interface CheckedProps {
  checked: boolean
  onChecked: () => void
}

function EmailForm({ step, onEdit, validatorRef }: StepFormProps<'email'>) {
  const { t } = useTranslation()
  const form = useSchemaForm(stepSchemas.email, step.draft, (draft) => onEdit({ ...step, draft }))
  useValidator(form, validatorRef)
  const { values } = form

  return (
    <Stack>
      <IntegrationControl
        channel="email"
        value={values.integrationId}
        onChange={(integrationId) => form.setFieldValue('integrationId', integrationId)}
      />
      <TextInput
        label={t(($) => $.automations.emailSubjectLabel)}
        {...form.getInputProps('subject')}
      />
      <MessageControl
        doc={values.doc}
        body={values.body}
        onChange={({ doc, body }) => form.setValues({ doc, body })}
      />
    </Stack>
  )
}

function TelegramForm({ step, onEdit, validatorRef }: StepFormProps<'telegram'>) {
  const form = useSchemaForm(stepSchemas.telegram, step.draft, (draft) =>
    onEdit({ ...step, draft }),
  )
  useValidator(form, validatorRef)
  const { values } = form

  return (
    <Stack>
      <IntegrationControl
        channel="telegram"
        value={values.integrationId}
        onChange={(integrationId) => form.setFieldValue('integrationId', integrationId)}
      />
      {/* An Automation's message also binds the Event its Enrollment began on. */}
      <TelegramContentEditor
        extraTags={[EVENT_ACTION_TAG]}
        value={values}
        onChange={({ text, imageUrl, buttons, linkPreview }) =>
          form.setValues({ text, imageUrl, buttons, linkPreview })
        }
      >
        <TelegramUnsubscribeControl
          label={values.unsubscribeButton}
          onChange={(label) => form.setFieldValue('unsubscribeButton', label)}
        />
      </TelegramContentEditor>
    </Stack>
  )
}

function SmsForm({ step, onEdit, validatorRef }: StepFormProps<'sms'>) {
  const form = useSchemaForm(stepSchemas.sms, step.draft, (draft) => onEdit({ ...step, draft }))
  useValidator(form, validatorRef)
  const { values } = form

  return (
    <Stack>
      <IntegrationControl
        channel="sms"
        value={values.integrationId}
        onChange={(integrationId) => form.setFieldValue('integrationId', integrationId)}
      />
      {/* An Automation's message also binds the Event its Enrollment began on. */}
      <SmsContentEditor
        extraTags={[EVENT_ACTION_TAG]}
        value={values}
        onChange={({ text, linkTracking }) => form.setValues({ text, linkTracking })}
      />
    </Stack>
  )
}

// An HTTP step's headers are rows, as a Rule's http Action edits them, and its body has a dry run.
// Only the names of saved headers come back from the server, so a stored step keeps a header whose
// value is left empty, and its URL and secret when they are.
export function HttpForm({ step, onEdit, validatorRef }: StepFormProps<'http'>) {
  const { t } = useTranslation()
  const form = useSchemaForm(stepSchemas.http, step.draft, (draft) => onEdit({ ...step, draft }))
  useValidator(form, validatorRef)
  const { values } = form

  return (
    <Stack>
      <TextInput
        label={t(($) => $.automations.httpUrlLabel)}
        placeholder={t(($) => $.automations.httpUrlHint)}
        {...form.getInputProps('url')}
      />
      <Stack gap={4}>
        <Text size="sm" fw={500}>
          {t(($) => $.automations.httpHeadersLabel)}
        </Text>
        <HttpHeaderRows
          headers={values.headers}
          saved={values.stored}
          onChange={(headers) => form.setFieldValue('headers', headers)}
        />
      </Stack>
      <Textarea
        label={t(($) => $.automations.httpBodyLabel)}
        description={t(($) => $.automations.httpBodyHint, { mergeTag: '{{ first_name }}' })}
        autosize
        minRows={6}
        {...form.getInputProps('body')}
      />
      <HttpDryRun body={values.body} />
      <TextInput
        label={t(($) => $.automations.httpSecretLabel)}
        description={t(($) => $.automations.httpSecretHint)}
        {...form.getInputProps('secret')}
      />
    </Stack>
  )
}

function WaitForm({ step, onEdit, validatorRef }: StepFormProps<'wait'>) {
  const { t } = useTranslation()
  const form = useSchemaForm(stepSchemas.wait, step.draft, (draft) => onEdit({ ...step, draft }))
  useValidator(form, validatorRef)

  return (
    <NumberInput
      label={t(($) => $.automations.waitSecondsLabel)}
      min={1}
      allowDecimal={false}
      allowNegative={false}
      {...form.getInputProps('seconds')}
    />
  )
}

function TagForm({ step, onEdit, validatorRef }: StepFormProps<'apply_tag' | 'remove_tag'>) {
  const { t } = useTranslation()
  const form = useSchemaForm(stepSchemas.apply_tag, step.draft, (draft) =>
    onEdit({ ...step, draft }),
  )
  useValidator(form, validatorRef)

  return <TextInput label={t(($) => $.automations.tagNameLabel)} {...form.getInputProps('tag')} />
}

// The forms a Wait until waits on, in the contract's order.
const WAIT_UNTIL_FORMS = zWaitUntilMomentInput.options.map((option) => option.shape.form.value)

// A Wait until's panel (ADR 0126): the form it waits on, a time of day or a date the Contact
// holds, and that form's own fields. The date form is the date Trigger's. Switching the form keeps
// what the other one holds.
function WaitUntilForm({
  step,
  onEdit,
  validatorRef,
  checked,
  onChecked,
}: StepFormProps<'wait_until'> & CheckedProps) {
  const { t } = useTranslation()
  const { draft } = step
  const forms: Record<WaitUntilMomentInput['form'], string> = {
    time_of_day: t(($) => $.automations.waitUntilForm.time_of_day),
    date: t(($) => $.automations.waitUntilForm.date),
  }
  const moment = { validatorRef, checked, onChecked }

  return (
    <Stack>
      <Input.Wrapper label={t(($) => $.automations.waitUntilFormLabel)}>
        <SegmentedControl
          display="flex"
          data={WAIT_UNTIL_FORMS.map((form) => ({ value: form, label: forms[form] }))}
          value={draft.form}
          onChange={(picked) => {
            const form = WAIT_UNTIL_FORMS.find((candidate) => candidate === picked)
            if (form !== undefined) {
              onEdit({ ...step, draft: { ...draft, form } })
            }
          }}
        />
      </Input.Wrapper>
      {draft.form === 'date' ? (
        <DateOccurrenceForm
          key="date"
          value={draft.date}
          onChange={(date) => onEdit({ ...step, draft: { ...draft, date } })}
          {...moment}
        />
      ) : (
        <TimeOfDayForm
          key="time_of_day"
          value={draft.timeOfDay}
          onChange={(timeOfDay) => onEdit({ ...step, draft: { ...draft, timeOfDay } })}
          {...moment}
        />
      )}
    </Stack>
  )
}

// StepForm is the form of the Step's kind, which the side panel shows for the selected Step.
export function StepForm({
  step,
  onEdit,
  validatorRef,
  checked,
  onChecked,
}: {
  step: EditedStep
  onEdit: (step: EditedStep) => void
  validatorRef: StepValidator
} & CheckedProps) {
  switch (step.kind) {
    case 'email':
      return <EmailForm step={step} onEdit={onEdit} validatorRef={validatorRef} />
    case 'telegram':
      return <TelegramForm step={step} onEdit={onEdit} validatorRef={validatorRef} />
    case 'sms':
      return <SmsForm step={step} onEdit={onEdit} validatorRef={validatorRef} />
    case 'http':
      return <HttpForm step={step} onEdit={onEdit} validatorRef={validatorRef} />
    case 'wait':
      return <WaitForm step={step} onEdit={onEdit} validatorRef={validatorRef} />
    case 'wait_until':
      return (
        <WaitUntilForm
          step={step}
          onEdit={onEdit}
          validatorRef={validatorRef}
          checked={checked}
          onChecked={onChecked}
        />
      )
    case 'apply_tag':
    case 'remove_tag':
    default:
      return <TagForm step={step} onEdit={onEdit} validatorRef={validatorRef} />
  }
}
