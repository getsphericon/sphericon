# UI patterns

How a screen behaves: what it asks before a delete, when a form complains, what an empty list says.
The source is [Cloudscape](https://cloudscape.design/patterns), the only one (ADR 0091). We take its
rules and render them with Mantine; its components are not used. How a screen is styled is in
`CODING_STANDARDS.md` (no custom CSS, theming), where the code lives is in `src/AGENTS.md`.

Each rule names its Cloudscape page and the component that already follows it. **Target** marks a
rule the code does not follow yet: a new screen follows it, an existing one is brought in line by
the ticket the marker names. A screen that departs from a rule says so in its pull request.

## Deleting

[Delete patterns](https://cloudscape.design/patterns/resource-management/delete/). Pick the lowest
friction that fits all four of: how severe the deletion is, how many resources it touches, how
costly it is to recreate them, and whether something must be done first.

- **One click, no confirmation:** low risk, easy to recreate, nothing depends on it, no
  prerequisite. The row leaves the list and no message follows.
- **Simple confirmation:** one or several resources that are not quickly recreated but unlikely to
  break anything running. A modal that names the consequence (required, no default text); the toast
  after it reports the result. This is
  `useConfirmation` (`src/hooks/useConfirmation.tsx`) and, with the request, `useDeleteResource`.
- **Additional confirmation:** serious, irreversible or cascading: the person types the resource's
  name before the button enables. This is `DeleteWorkspaceControl` and `useConfirmation` with
  `typedName`, both rendering `TypedConfirmation`.

The modal says what the deletion does and what to do first (a deactivation, a copy), in its own
words rather than the default text. How a row is removed (physically or archived) is rule 9 of
`review-rules.md`, not a choice made here.

`useConfirmation` and `useDeleteResource` take the consequence text as required input (there is no
default) and `typedName` (the additional confirmation, the same `TypedConfirmation` component
`DeleteWorkspaceControl` renders). A credential or endpoint something live depends on (an API key,
a Webhook endpoint) takes the typed one.

## Forms and validation

[Validation](https://cloudscape.design/patterns/general/errors/validation/).

- A field is not validated when the page opens. It is validated when it loses focus after a value
  was entered, or left empty if required; once an error shows, it is re-checked on each keystroke.
  Everything is validated again on submit.
- An error about one field, from the browser or from the server (a name already taken), shows under
  that field. An error with no field (a Permission) is an `ApiErrorAlert` above the submit button.
  No "fix all errors" banner.
- On submit the page scrolls to the first error and focuses its field.
- The submit button stays enabled: it is how the person finds what is missing. It is disabled only
  when a prerequisite is missing, or in a confirmation modal.
- Put the format in the field's constraint text, not in the error, and not as a placeholder
  (Mantine `description`); the error says which requirement failed.
- Messages: "Name is required." / "Enter a valid email address." / "Name is too long."

**Target (#999):** forms validate on submit only. A form built by `useSchemaForm`
(`src/resources/schemaForm.ts`, the one place a schema form is made) gains the blur and keystroke
validation there, and the second argument of `form.onSubmit` is how the first invalid field gets
focus. A few submit buttons are disabled in place of validation (`signup-survey.tsx`,
`oauth/consent.tsx`, `account/acceptance.tsx` are the ones to look at).

## Lists and tables

[Table view](https://cloudscape.design/patterns/resource-management/view/table-view/) and
[global actions](https://cloudscape.design/patterns/general/actions/global-actions/).

- A list of resources is a `PagedTable` (`mantine-datatable`) on a route with a `page` search
  param. Columns run left to right by importance; a header does not change when a filter applies.
- Header actions act on the collection or the selection; a row's actions are in its
  `RowActionsMenu`. One primary button per page. Order: view, edit, other, delete, create.
- Filters search the whole collection, not the loaded page. A text filter for a known value; a
  single-value filter per property for a few properties; a property filter (a query builder) when
  there are many.
- Paging clears the selection; so do sorting, a filter and a change of columns.
- Selection (not built yet): a header checkbox selects the rows of the page only, and an action
  applies to those rows. A bulk delete confirms with the number of resources (rule above).
- Errors of an action are a toast (`useNotify`); an error loading the list is the `ApiErrorAlert`
  `PagedTable` shows above the table.
- `PagedTable` has a refresh button above the table and the time of the last successful refresh
  beside it, in a polite live region. The rows stay on screen while it runs, and a failed refresh
  leaves the previous time. No list adds its own refresh.

## Empty states

[Empty states](https://cloudscape.design/patterns/general/empty-states/).

- **Empty** (no resource exists, or all were deleted) and **zero results** (a filter matches none)
  are two states. Zero results never invites the person to create something: `contacts/list.tsx`
  passes no empty-state text while a filter is on.
- A heading that names the state ("No contacts", "No matches"), bold, no end punctuation. A
  description only when it says why it is empty or what to do, below a heading, with end
  punctuation, never repeating the heading or the button.
- One action, a default-variant button: for empty, the same label as the list's create button; for
  zero results, "Clear filter".
- In Mantine: `EmptyState` (`@mantine/core`) with its `title` and `description`, and the action as
  a `Button variant="default"` inside `EmptyState.Actions`.
- The count shows with the collection ("Contacts (0)").
- An error is not an empty state: Cloudscape's empty-states page says to use an alert or a toast.
  (Its table-view page puts a fetch error in an alert inside the table's empty area; `PagedTable`
  puts the same `ApiErrorAlert` above the table. That is a departure, kept because the table keeps
  its place while a refresh fails.)
- An empty value in a cell or a key-value row is a hyphen, not a blank and not `0`.

`PagedTable` takes the anatomy as `collection` (the name in the count, "Contacts (0)") and `empty`
(`title`, an optional `description`, and the `action` button). A list that is not a `PagedTable`
renders the same by hand.

## Loading

[Loading and refreshing](https://cloudscape.design/patterns/general/loading-and-refreshing/).

- First load of a table is a skeleton with the columns' layout, not a spinner. A spinner is for
  loading more rows or a row's children.
- A query under `QueryBoundary` shows the skeleton, the error and the retry; a refresh keeps the
  current data on screen.
- Loading text has no article and no end punctuation ("Loading contacts").

## Errors

[Error messages](https://cloudscape.design/patterns/general/errors/error-messages/).

- Every message says what happened, why when it is known, and what to do. No raw machine text as
  the message. `ApiErrorAlert` shows a title and the API's own message; a development server's
  fault shows under it in `ProblemDebugPanel`, which a production Instance never sends.
- Where it shows: under the field (a field's own), an `ApiErrorAlert` near what failed (a form or
  a section), a toast (the outcome of an action), `AppErrorPage` or a section's boundary (a failure
  of the whole page or of one panel).
- Do not promise what we do not do ("our team was notified" only when something reports it), and do
  not blame the person.

## Unsaved changes

[Communicating unsaved changes](https://cloudscape.design/patterns/general/unsaved-changes/). A
form a person can leave half-typed renders `LeaveDialog`: it covers a link, Back and a closed tab
and names what would be lost. No "don't ask again". It is not shown when nothing changed or the
changes are saved.

## Writing

Every string in the app, a message, a button, a hint:

- Sentence case; proper nouns keep their capitals. End punctuation on sentences, none on headings,
  buttons and labels. No exclamation points.
- Present tense, active voice, an action verb on a button ("Create contact", not "Contact creation").
- No "please", "thank you", ellipses, ampersands, "e.g.", "i.e." or "etc.".
- "Select" or "choose", not "click". "Previous" and "following", not "above" and "below".
- English only, a string in the `en` catalog (`src/AGENTS.md`).

## Not covered

Cloudscape has no pattern for Condition and Automation builders or for the Broadcast flow, and the
AWS-specific parts of it (service dashboards, density, card view, saved filter sets) are not used.
What is settled for those goes here when it is.
