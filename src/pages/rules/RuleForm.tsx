import {
  ActionIcon,
  Button,
  Group,
  Input,
  Select,
  Stack,
  TagsInput,
  TextInput,
} from '@mantine/core'
import { IconTrash } from '@tabler/icons-react'
import { useQuery } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import * as z from 'zod'

import { conditionCodec, draftText, emptyGroup } from '../../components/conditions/condition.ts'
import { ConditionBuilder } from '../../components/conditions/ConditionBuilder.tsx'
import { EventActionOption } from '../../components/EventActionName.tsx'
import { ButtonLink } from '../../components/RouterLink.tsx'
import {
  siteAutomationsListOptions,
  siteEventsActionsOptions,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import type { SiteCreateRuleInput, SiteRuleAction } from '../../generated/site/types.gen.ts'
import { useEventActionFilter } from '../../hooks/useEventActionLabel.ts'
import type { ResourceFormProps } from '../../resources/defineResource.tsx'
import { Route as workspaceRoute } from '../../routes/workspaces/$workspaceId/route.tsx'
import { Route as rulesRoute } from '../../routes/workspaces/$workspaceId/rules/index.tsx'
import {
  actionsCodec,
  newAction,
  ruleActionKinds,
  takesAutomation,
  takesCall,
  takesTag,
  triggerEventsCodec,
} from './actions.ts'
import { HttpActionFields } from './HttpActionFields.tsx'

// What the form submits: the create input, with the Condition and the Actions always given.
export type RulePayload = Pick<SiteCreateRuleInput, 'name' | 'triggerEvents'> &
  Required<Pick<SiteCreateRuleInput, 'condition' | 'actions'>>

export function RuleForm({ form, isPending, onSubmit }: ResourceFormProps<RulePayload>) {
  const { t } = useTranslation()
  const actionFilter = useEventActionFilter()
  const { workspaceId } = workspaceRoute.useParams()

  // The form holds each structured field as JSON text; a blank or unreadable text is empty.
  const triggerEvents = useMemo(() => {
    const parsed = triggerEventsCodec.safeParse(form.values.triggerEvents)
    return parsed.success ? parsed.data : []
  }, [form.values.triggerEvents])
  // The builder starts from the stored Condition and keeps its own state (see SegmentForm).
  const [initialCondition] = useState(() => {
    const parsed = conditionCodec.safeParse(form.values.condition)
    return parsed.success ? parsed.data : emptyGroup
  })
  const actions = useMemo(() => {
    const parsed = actionsCodec.safeParse(form.values.actions)
    return parsed.success ? parsed.data : []
  }, [form.values.actions])

  const knownEvents = useQuery(siteEventsActionsOptions({ path: { workspaceId } }))
  // The automations an enroll action can pick from; read only once an action needs them.
  const automationsQuery = useQuery({
    ...siteAutomationsListOptions({ path: { workspaceId }, query: { pageSize: 100 } }),
    enabled: actions.some((action) => takesAutomation(action.type)),
  })
  const automationOptions = (automationsQuery.data?.items ?? []).map((automation) => ({
    value: automation.id,
    label: automation.name,
  }))
  const setActions = (next: SiteRuleAction[]) =>
    form.setFieldValue('actions', z.encode(actionsCodec, next))
  const changeAction = (index: number, change: Partial<SiteRuleAction>) =>
    setActions(actions.map((action, at) => (at === index ? { ...action, ...change } : action)))

  return (
    <form onSubmit={form.onSubmit(onSubmit)}>
      <Stack>
        <TextInput
          label={t(($) => $.rules.nameLabel)}
          withAsterisk
          {...form.getInputProps('name')}
        />
        <TagsInput
          label={t(($) => $.rules.triggerEventsLabel)}
          description={t(($) => $.rules.triggerEventsHint)}
          placeholder={t(($) => $.rules.triggerEventsPlaceholder)}
          withAsterisk
          data={knownEvents.data?.actions ?? []}
          renderOption={({ option }) => <EventActionOption action={option.value} />}
          filter={actionFilter}
          value={triggerEvents}
          onChange={(next) =>
            form.setFieldValue(
              'triggerEvents',
              next.length > 0 ? z.encode(triggerEventsCodec, next) : '',
            )
          }
          error={form.errors.triggerEvents}
        />
        <Input.Wrapper
          label={t(($) => $.rules.conditionLabel)}
          description={t(($) => $.rules.conditionHint)}
        >
          <ConditionBuilder
            workspaceId={workspaceId}
            triggerLeaves
            initialValue={initialCondition}
            onChange={(next) => form.setFieldValue('condition', draftText(next))}
          />
        </Input.Wrapper>
        <Input.Wrapper
          label={t(($) => $.rules.actionsLabel)}
          description={t(($) => $.rules.actionsHint)}
        >
          <Stack gap="xs" mt="xs">
            {actions.map((action, index) => (
              <Stack key={index} gap="xs">
                <Group align="flex-end" wrap="nowrap">
                  <Select
                    label={t(($) => $.rules.actionTypeLabel)}
                    allowDeselect={false}
                    data={ruleActionKinds.map((type) => ({
                      value: type,
                      label: t(($) => $.rules.actionType[type]),
                    }))}
                    value={action.type}
                    onChange={(next) => {
                      const type = ruleActionKinds.find((kind) => kind === next)
                      if (type) {
                        changeAction(index, { type })
                      }
                    }}
                  />
                  {takesTag(action.type) ? (
                    <TextInput
                      label={t(($) => $.rules.actionTagLabel)}
                      value={action.tag ?? ''}
                      onChange={(event) => changeAction(index, { tag: event.currentTarget.value })}
                    />
                  ) : null}
                  {takesAutomation(action.type) ? (
                    <Select
                      label={t(($) => $.rules.actionAutomationLabel)}
                      searchable
                      data={automationOptions}
                      value={action.automationId ?? null}
                      onChange={(next) => {
                        if (next) {
                          changeAction(index, { automationId: next })
                        }
                      }}
                    />
                  ) : null}
                  <ActionIcon
                    variant="subtle"
                    color="red"
                    aria-label={t(($) => $.rules.actionRemove)}
                    onClick={() => setActions(actions.filter((_, at) => at !== index))}
                  >
                    <IconTrash size={16} />
                  </ActionIcon>
                </Group>
                {takesCall(action.type) ? (
                  <HttpActionFields
                    action={action}
                    onChange={(change) => changeAction(index, change)}
                  />
                ) : null}
              </Stack>
            ))}
            <Group>
              <Button
                variant="light"
                size="compact-sm"
                onClick={() => setActions([...actions, newAction()])}
              >
                {t(($) => $.rules.addAction)}
              </Button>
            </Group>
          </Stack>
        </Input.Wrapper>

        <Group justify="flex-end">
          <ButtonLink variant="default" to={rulesRoute.to} params={{ workspaceId }}>
            {t(($) => $.actions.cancel)}
          </ButtonLink>
          <Button type="submit" loading={isPending}>
            {t(($) => $.actions.save)}
          </Button>
        </Group>
      </Stack>
    </form>
  )
}
