import type { ComponentType } from 'react'

import type { AnyCondition, ConditionDraft } from './condition.ts'

// The values offered for a property's text test, by the property's key.
export type PropertyValues = Partial<Record<string, readonly string[]>>

// The editor of the filter of an event test's Events (ADR 0153): the builder of an Event filter on
// the action, which ConditionBuilder hands its value editors, since they sit below it.
export interface EventFilterEditorProps {
  workspaceId: string
  action: string
  initialValue: AnyCondition
  onChange: (filter: ConditionDraft) => void
}

// What the builder hands its value editor through the query builder's context.
export interface ValueEditorContext {
  propertyValues?: PropertyValues
  // The keys seen on recent Events of the action a filter is about: a suggestion, not a limit.
  propertyKeys?: readonly string[]
  // The Workspace, the Event filter editor and how far back analytical Events are kept, which an
  // event test's value editor reads; absent in a builder that tests no Contact.
  workspaceId?: string
  eventFilterEditor?: ComponentType<EventFilterEditorProps>
  eventsKeptDays?: number
  keptActions?: readonly string[]
}
