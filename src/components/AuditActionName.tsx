import { useAuditActionLabel } from '../hooks/useAuditActionLabel.ts'
import { ActionName } from './ActionName.tsx'

// AuditActionName shows the action of an Audit entry by its words, its code on hover. An action an
// older release recorded and the contract no longer declares shows as its code.
export function AuditActionName({ action }: { action: string }) {
  return <ActionName action={action} label={useAuditActionLabel()(action)} />
}
