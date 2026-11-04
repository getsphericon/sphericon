// Package site stands in for an ogen server package.
package site

type ContactResource struct{ Email string }
type Opt[T any] struct{ Value T }

// CreateAutomationInput is a request body: the generated type, not a module input.
type CreateAutomationInput struct{ Name string }

type ContactList struct{ Items []ContactResource }

type InvitationLookup struct {
	WorkspaceName string
	Email         string
}

type WorkspaceID string

type SoleOwnedWorkspace struct {
	ID   WorkspaceID
	Name string
}

type DeleteProblem struct {
	Detail     string
	Workspaces []SoleOwnedWorkspace
}

type PasskeyList struct {
	Items       []ContactResource
	PasswordSet bool
}

type UsedBy struct {
	ID   int64
	Name string
}
