# Frontend resource forms

[Architecture](../architecture.md) → Frontend resource forms

`src/resources` owns resource form hydration, request conversion and the create/edit lifecycle.
Editors hold typed values, including collections, Condition drafts and message documents, and pass
them directly to the form. `resourceFormSchema` combines explicit empty editor values with the
generated request schema: omitted updates stay absent, nullable clears stay null, and empty
collections remain supplied. Scalar blanks follow the contract's clearing behavior; a null empty
editor value can omit a nonnullable field, such as a document when editing HTML source. Generated
schemas validate the resulting request, including incomplete drafts. `defineResource` preserves
unfinished edits across refetches, reloads on an intentional resource revision change and resets
stateful editors to the normalized saved resource after a successful save.
