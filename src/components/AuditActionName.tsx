import { useAuditActionLabel } from '../hooks/useAuditActionLabel.ts'
import type { AnyAuditAction } from '../hooks/useAuditActionLabel.ts'
import { ActionName } from './ActionName.tsx'

// AuditActionName shows the action of an Audit entry by its words, its code on hover.
export function AuditActionName({ action }: { action: AnyAuditAction }) {
  return <ActionName action={action} label={useAuditActionLabel()(action)} />
}
