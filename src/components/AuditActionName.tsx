import { useAuditActionLabel } from '../hooks/useAuditActionLabel.ts'
import { ActionName, ActionOption } from './ActionName.tsx'

// AuditActionName shows the action of an Audit entry by its words, its code on hover.
export function AuditActionName({ action }: { action: string }) {
  return <ActionName action={action} label={useAuditActionLabel()(action)} />
}

// AuditActionOption is an option of the audit log's action filter: the words, then the code it
// filters by.
export function AuditActionOption({ action }: { action: string }) {
  return <ActionOption action={action} label={useAuditActionLabel()(action)} />
}
