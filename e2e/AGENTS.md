# End-to-end suite

End-to-end suite (ADR 0024, `e2e/` behind the `e2e` build tag, not part of `mise run test`):
`mise run test:e2e` rebuilds the checkout's `<PGDATABASE>_e2e` database, starts its own Mailpit and boots the
app in-process. `HOLD=true mise run test:e2e` keeps the app and Mailpit up after the run (their URLs are
printed) until Ctrl-C, to inspect a failed scenario in the Mailpit UI. Scenarios are domain steps on
`e2e.Workspace` (`env.NewWorkspace(t).Ready()`, `ImportContacts`, `SendBroadcast`), one flat struct whose steps live in non-test files by concept
(`workspace.go`, `integration.go`, `contacts.go`, `broadcast.go`, `automation.go`, `consent.go`); `_test.go`
files hold only scenarios, and bodies are authored with `e2e.HTML(text)`. Mail is observed only through
the Workspace's `Inbox`: one wait (`w.Inbox.Wait(e2e.Match{To, Subject})`) and one absence check
(`w.Inbox.RequireNone`); it deletes every message it saw on cleanup.
