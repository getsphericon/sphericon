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
import { useTranslation } from 'react-i18next'

import { toCondition, emptyGroup } from '../../components/conditions/condition.ts'
import type { ConditionDraft } from '../../components/conditions/condition.ts'
import { ConditionBuilder } from '../../components/conditions/ConditionBuilder.tsx'
import { EventActionOption } from '../../components/EventActionName.tsx'
import { ButtonLink } from '../../components/RouterLink.tsx'
import {
  siteAutomationsListOptions,
  siteEventsActionsOptions,
} from '../../generated/site/@tanstack/react-query.gen.ts'
import type { CreateRuleInput } from '../../generated/site/types.gen.ts'
import { zSiteAutomationsListQuery } from '../../generated/site/zod.gen.ts'
import { useEventActionFilter } from '../../hooks/useEventActionLabel.ts'
import type { ResourceFormProps } from '../../resources/defineResource.tsx'
import { Route as workspaceRoute } from '../../routes/workspaces/$workspaceId/route.tsx'
import { Route as rulesRoute } from '../../routes/workspaces/$workspaceId/rules/index.tsx'
import { maxPageSize } from '../../utils/bounds.ts'
import { newAction, ruleActionKinds, takesAutomation, takesCall, takesTag } from './actions.ts'
import { HttpActionFields } from './HttpActionFields.tsx'

// What the form submits: the create input, with the Condition and the Actions always given.
export type RulePayload = Pick<CreateRuleInput, 'name' | 'triggerEvents'> &
  Required<Pick<CreateRuleInput, 'condition' | 'actions'>>

// What the form holds: the create input, with the Condition as a draft the builder edits.
export type RuleValues = Required<Pick<CreateRuleInput, 'name' | 'triggerEvents' | 'actions'>> & {
  condition: ConditionDraft
}

export function RuleForm({
  form,
  isPending,
  onSubmit,
}: ResourceFormProps<RulePayload, RuleValues>) {
  const { t } = useTranslation()
  const actionFilter = useEventActionFilter()
  const { workspaceId } = workspaceRoute.useParams()

  const { actions } = form.values

  const knownEvents = useQuery(siteEventsActionsOptions({ path: { workspaceId } }))
  // The automations an enroll action can pick from; read only once an action needs them.
  const automationsQuery = useQuery({
    ...siteAutomationsListOptions({
      path: { workspaceId },
      query: { pageSize: maxPageSize(zSiteAutomationsListQuery) },
    }),
    enabled: actions.some((action) => takesAutomation(action.type)),
  })
  const automationOptions = (automationsQuery.data?.items ?? []).map((automation) => ({
    value: automation.id,
    label: automation.name,
  }))
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
          {...form.getInputProps('triggerEvents')}
        />
        <Input.Wrapper
          label={t(($) => $.rules.conditionLabel)}
          description={t(($) => $.rules.conditionHint)}
        >
          <ConditionBuilder
            workspaceId={workspaceId}
            triggerLeaves
            initialValue={toCondition(form.values.condition) ?? emptyGroup}
            onChange={(next) => form.setFieldValue('condition', next)}
          />
        </Input.Wrapper>
        <Input.Wrapper
          label={t(($) => $.rules.actionsLabel)}
          description={t(($) => $.rules.actionsHint)}
        >
          <Stack gap="xs" mt="xs">
            {actions.map((action, index) => (
              <Stack key={form.key(`actions.${index}`)} gap="xs">
                <Group align="flex-end" wrap="nowrap">
                  <Select
                    label={t(($) => $.rules.actionTypeLabel)}
                    allowDeselect={false}
                    data={ruleActionKinds.map((type) => ({
                      value: type,
                      label: t(($) => $.rules.actionType[type]),
                    }))}
                    {...form.getInputProps(`actions.${index}.type`)}
                  />
                  {takesTag(action.type) ? (
                    <TextInput
                      label={t(($) => $.rules.actionTagLabel)}
                      {...form.getInputProps(`actions.${index}.tag`)}
                      value={action.tag ?? ''}
                    />
                  ) : null}
                  {takesAutomation(action.type) ? (
                    <Select
                      label={t(($) => $.rules.actionAutomationLabel)}
                      searchable
                      allowDeselect={false}
                      data={automationOptions}
                      {...form.getInputProps(`actions.${index}.automationId`)}
                      value={action.automationId ?? null}
                    />
                  ) : null}
                  <ActionIcon
                    variant="subtle"
                    color="red"
                    aria-label={t(($) => $.rules.actionRemove)}
                    onClick={() => form.removeListItem('actions', index)}
                  >
                    <IconTrash size={16} />
                  </ActionIcon>
                </Group>
                {takesCall(action.type) ? (
                  <HttpActionFields
                    action={action}
                    inputProps={(field) => form.getInputProps(`actions.${index}.${field}`)}
                  />
                ) : null}
              </Stack>
            ))}
            <Group>
              <Button
                variant="light"
                size="compact-sm"
                onClick={() => form.insertListItem('actions', newAction())}
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
