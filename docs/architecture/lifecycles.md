# Lifecycles and concurrency

[Architecture](../architecture.md) → Lifecycles and concurrency

## Owners and suspension

**Owners and suspension are defined once.** `internal/domain/identity/memberships` owns "Owner of a
Workspace": `Owners`, `OwnersOf`, `LockOwners`, `IsOwner`, `OwnerRole`;
`internal/domain/identity/suspension` owns "suspended" (`Is`, `Suspended`, `NotSuspended`,
`BySystem`). A Workspace is suspended through the container's one `suspension.Suspender` (the
command line, the Staff console and the abuse detector), whose event tells its Owners (see
[Notices](events-and-jobs.md#notices)). Call them; do not test `suspended_at` or the owner Role anywhere else.

## Removal checks

**Refused removals (ADR 0049)**: a row in Live use or in Definition use is not removed (nor is a
Custom field retyped while a Condition tests it with a leaf the new type does not take).
`internal/domain/orchestration/liveuse` is the one shape of that refusal (`*liveuse.Error`: the
removed `Subject` and the `Users` that hold it, with helpers like `liveuse.Broadcasts` and
`liveuse.Enrollments`), and every API surface answers it with the contract's `Problem` (409,
`usedBy`). When the holders are modules above the removed row's module, the check is the Removal
check, `internal/domain/orchestration/removalcheck`: one method per subject (`Tag`,
`CustomFieldRetype`, `Integration`, `SendingDomain`), each returning nil or the `*liveuse.Error`
with its `Subject` set, called by the use case that removes the row under the Workspace lock before
its unchecked removal (`tagdeletion` before `tagsunchecked.DeleteIn`, `customfieldretyping` before
`contactsunchecked.UpdateCustomFieldIn`, `integrationdeletion` and `sendingdomaindeletion` before
`integrationsunchecked.DeleteIn` and `sendingdomainsunchecked.DeleteIn`; see [Use cases](use-cases.md)). It is the one list of the
readers of what may hold a row. Each owner keeps its readers in a subpackage `<module>use` (ADR
0110's fence by place), which may import its owner and which the owner does not import: Definition
use is `segmentsuse.UsingTag` and `UnfitForCustomFieldType`, `automationsuse.UsingTag`,
`rulesuse.UsingTag` and `UnfitForCustomFieldType` and `broadcastsuse.UsingTag` and
`UnfitForCustomFieldType`; the Live use that sends is `broadcastsuse.LiveSends` and
`automationsuse.LiveSends`, which return each send as a `liveuse.Send` (who sends, on which
Channel, through which Integration by `messaging.Through`, from which From), so the Removal check
and the send path agree on where work sends; a Sending domain's check reads the From an
Integration sends with from the resolver a send builds its sender with
(`messaging.EffectiveFrom`). Two easement components, one per context (`domain-contacts-removalcheck-readers`,
`domain-orchestration-removalcheck-readers`), own the four subpackages with `allow_importers:
[domain-orchestration-removalcheck]` (ADR 0110), so no other package reads a holder and a new holder is a reader the Removal check adds. The Removal check and its readers
take the scope of the Workspace lock, `*ent.WorkspaceLocked` (ADR 0108), so none of them runs
without it. A domain module that checks the holders of its own
row (an Automation's archive) returns the same `*liveuse.Error`. `liveuse` is a leaf that holds no
raw `*ent.Client`: it only maps rows to users and states these shapes.

## Stored Automation Steps

**Stored Automation Steps:** `internal/domain/orchestration/automationstep` owns the stored Step model, its kinds,
decoding and whether a definition sends email. Publish, Needs attention and live-use checks
share that interpretation: an empty stored string means no Steps; damaged JSON returns an error,
so a removal cannot treat an unreadable definition as unused. The reader chooses the version:
dependencies use live, an Enrollment uses its pinned version. `internal/domain/orchestration/automations` keeps
preparation for writing (normalization, HTML sanitization, validation and secret sealing), masked
API views and the version lifecycle. The shared reader neither queries nor executes Steps.

## Workspace lock

**The Workspace lock (ADR 0099, ADR 0108)**: a removal that checks Live use or Definition use and
a write that moves a use run in `events.Bus.WithinWorkspaceLockTx` (or
`WithinWorkspaceLockEnqueueTx`), whose first statement takes the Workspace's transaction-scoped
advisory lock, so a check never passes on work that becomes live before the removal commits. A
write moves a use when, after it, live work uses a different row. The lock is a type: the bus hands
its callback `*ent.WorkspaceLocked` (`ent/template/scoped_lock.tmpl`), the one caller of
`ent.Scoped.LockWorkspace` (forbidigo), and the list of what needs the lock is the set of
signatures that take that type, so a forgotten lock does not compile:

- the readers of Live use and Definition use: the Removal check's methods, the `<module>use`
  readers (`UsingTag`, `UnfitForCustomFieldType`, `LiveSends`) and `rules.ActiveEnrollingInto`,
  which an Automation's Archive reads through `versioneddefinition.Definition.Archivable`;
- the unchecked removals behind their check: `tagsunchecked.DeleteIn`,
  `contactsunchecked.UpdateCustomFieldIn`, `integrationsunchecked.DeleteIn` and
  `sendingdomainsunchecked.DeleteIn`. `integrationsunchecked.DeleteIn` takes the id and gives up
  the Integration's default in the statement that discards it (`schema.GivenUpOnDiscard`);
- the writes that move a use: a version of a Versioned definition (`Definition.Write`, on Create
  and Edit) and its Publish, a Broadcast's claim, saving a Segment, and an Integration's save
  (`integrations.Create` and `Update`, whose promotion to its Channel's default, `clearDefault`,
  moves every send that names no Integration, and whose config may change the From its sends go
  out with).

A Tracking domain's moves take the lock to run one at a time; they hold no use. The lock comes
before any row lock. A hot path (a Rule firing, an Enrollment, a send) never takes it: it locks the
row it acts on and checks it again under that lock, and the lock is a value the write takes. An
Enrollment is written (`automations.begin`) with the Contact locked (`contactlock.Locked`) and the
Automation share-locked on its live version; a Firing (`rules.recordFiring`) with its Rule
share-locked while it still fires on that version.
