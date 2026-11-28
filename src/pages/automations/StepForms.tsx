import {
  Group,
  Input,
  NumberInput,
  SegmentedControl,
  Stack,
  Text,
  Textarea,
  TextInput,
} from '@mantine/core'
import { useRef } from 'react'
import { useTranslation } from 'react-i18next'

import { HttpHeaderRows } from '../../components/HttpHeaderRows.tsx'
import { MergeTagMenu } from '../../components/MergeTagMenu.tsx'
import { PropertyTagInsert } from '../../components/PropertyTagInsert.tsx'
import { SmsContentEditor } from '../../components/sms/SmsContentEditor.tsx'
import { TelegramContentEditor } from '../../components/telegram/TelegramContentEditor.tsx'
import { TelegramUnsubscribeControl } from '../../components/telegram/TelegramUnsubscribeControl.tsx'
import type { WaitUntilMomentInput } from '../../generated/site/types.gen.ts'
import { zWaitUntilMomentInput } from '../../generated/site/zod.gen.ts'
import { useSchemaForm } from '../../resources/schemaForm.ts'
import { CONTACT_TAGS } from '../../utils/mergeTags.ts'
import type { automationTags } from '../../utils/mergeTags.ts'
import type { EditedKind, EditedStep } from './definition.ts'
import { HttpDryRun } from './HttpStepControls.tsx'
import { IntegrationControl } from './IntegrationControl.tsx'
import { MessageControl } from './MessageControl.tsx'
import { DateOccurrenceForm, TimeOfDayForm } from './MomentForms.tsx'
import { stepSchemas } from './stepSchemas.ts'
import { useValidator } from './stepValidator.ts'
import type { StepValidator } from './stepValidator.ts'

type StepOf<K extends EditedKind> = Extract<EditedStep, { kind: K }>

// What a Step's content binds beyond the Contact's (automationTags): the merge tags of the Event its
// Enrollment began on and of the Events the Wait for events before it arrived on, and those Events,
// whose properties it reads by name.
export type BoundTags = ReturnType<typeof automationTags>

interface StepFormProps<K extends EditedKind> {
  step: StepOf<K>
  // Every edit of the form, as the Step with its new draft: the flow holds what is saved.
  onEdit: (step: StepOf<K>) => void
  validatorRef: StepValidator
  bound: BoundTags
}

// What a Step whose node shows its errors is also given: whether it was checked (on blur or on
// Save), and what marks it checked.
interface CheckedProps {
  checked: boolean
  onChecked: () => void
}

function EmailForm({ step, onEdit, validatorRef, bound }: StepFormProps<'email'>) {
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
        bound={bound}
        onChange={({ doc, body }) => form.setValues({ doc, body })}
      />
    </Stack>
  )
}

function TelegramForm({ step, onEdit, validatorRef, bound }: StepFormProps<'telegram'>) {
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
      {/* An Automation's message also binds the Events its Enrollment began on and awaited. */}
      <TelegramContentEditor
        extraTags={bound.tags}
        propertySources={bound.sources}
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

function SmsForm({ step, onEdit, validatorRef, bound }: StepFormProps<'sms'>) {
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
      {/* An Automation's message also binds the Events its Enrollment began on and awaited. */}
      <SmsContentEditor
        extraTags={bound.tags}
        propertySources={bound.sources}
        value={values}
        onChange={({ text, linkTracking }) => form.setValues({ text, linkTracking })}
      />
    </Stack>
  )
}

// An HTTP step's headers are rows, as a Rule's http Action edits them, and its body has a dry run
// and the merge tags it binds, inserted at the caret. Only the names of saved headers come back from
// the server, so a stored step keeps a header whose value is left empty, and its URL and secret when
// they are. Its name, which later Steps read its response by, is fixed once it is saved (ADR 0161).
export function HttpForm({
  step,
  onEdit,
  validatorRef,
  bound,
  storedName,
}: StepFormProps<'http'> & { storedName: string | undefined }) {
  const { t } = useTranslation()
  const form = useSchemaForm(stepSchemas.http, step.draft, (draft) => onEdit({ ...step, draft }))
  useValidator(form, validatorRef)
  const { values } = form
  // A name typed while the first Save was pending is no stored one: it stays editable.
  const locked = storedName !== undefined && values.name === storedName
  const body = useRef<HTMLTextAreaElement>(null)
  const insert = (tag: string) => {
    const at = body.current?.selectionStart ?? values.body.length
    const end = body.current?.selectionEnd ?? at
    form.setFieldValue('body', `${values.body.slice(0, at)}{{ ${tag} }}${values.body.slice(end)}`)
  }

  return (
    <Stack>
      <TextInput
        label={t(($) => $.automations.httpNameLabel)}
        description={
          locked ? t(($) => $.automations.httpNameLocked) : t(($) => $.automations.httpNameHint)
        }
        disabled={locked}
        {...form.getInputProps('name')}
      />
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
      <Stack gap="xs">
        <Textarea
          ref={body}
          label={t(($) => $.automations.httpBodyLabel)}
          description={t(($) => $.automations.httpBodyHint, { mergeTag: '{{ first_name }}' })}
          autosize
          minRows={6}
          {...form.getInputProps('body')}
        />
        <Group gap="xs">
          <MergeTagMenu
            label={t(($) => $.automations.httpInsertMergeTag)}
            tags={[...CONTACT_TAGS, ...bound.tags]}
            onInsert={insert}
          />
          <PropertyTagInsert sources={bound.sources} onInsert={insert} />
        </Group>
      </Stack>
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
  bound,
  storedName,
}: {
  step: EditedStep
  onEdit: (step: EditedStep) => void
  validatorRef: StepValidator
  bound: BoundTags
  // The name stored under the Step's id, fixed (an HTTP step's, ADR 0161); undefined for none.
  storedName: string | undefined
} & CheckedProps) {
  switch (step.kind) {
    case 'email':
      return <EmailForm step={step} onEdit={onEdit} validatorRef={validatorRef} bound={bound} />
    case 'telegram':
      return <TelegramForm step={step} onEdit={onEdit} validatorRef={validatorRef} bound={bound} />
    case 'sms':
      return <SmsForm step={step} onEdit={onEdit} validatorRef={validatorRef} bound={bound} />
    case 'http':
      return (
        <HttpForm
          step={step}
          onEdit={onEdit}
          validatorRef={validatorRef}
          bound={bound}
          storedName={storedName}
        />
      )
    case 'wait':
      return <WaitForm step={step} onEdit={onEdit} validatorRef={validatorRef} bound={bound} />
    case 'wait_until':
      return (
        <WaitUntilForm
          step={step}
          onEdit={onEdit}
          validatorRef={validatorRef}
          bound={bound}
          checked={checked}
          onChecked={onChecked}
        />
      )
    case 'apply_tag':
    case 'remove_tag':
    default:
      return <TagForm step={step} onEdit={onEdit} validatorRef={validatorRef} bound={bound} />
  }
}
