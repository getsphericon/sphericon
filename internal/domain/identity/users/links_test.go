package users_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/domain/identity/users"
	"github.com/getsphericon/sphericon/internal/fixtures"
	"github.com/getsphericon/sphericon/internal/kernel/authtoken"
	"github.com/getsphericon/sphericon/internal/testhelper"
)

// emailChangeRig mints John's link to change his address and confirms it.
type emailChangeRig struct {
	links *users.Links
	john  *ent.User
	token string
}

func newEmailChangeRig(t *testing.T) emailChangeRig {
	t.Helper()
	env := testhelper.Setup(t)
	acc := users.New(env.DB, env.Bus, testhelper.Passwords(), env.ConsentRecords, time.Now)
	links := users.NewLinks(acc, authtoken.New("links-test-secret", time.Now))
	john, err := acc.User(t.Context(), fixtures.OwnerJohnID)
	require.NoError(t, err)
	token, err := links.EmailChangeLink(john, "john.new@example.com")
	require.NoError(t, err)
	return emailChangeRig{links: links, john: john, token: token}
}

// ADR 0020: confirming the change ends every session of the User; the confirming request's own
// session goes on when it is a current one of that User.
func TestAConfirmedEmailChangeContinuesTheUsersCurrentSession(t *testing.T) {
	r := newEmailChangeRig(t)

	confirmed, err := r.links.ConfirmEmailChange(t.Context(), r.token, users.SessionHolder{UserID: r.john.ID, Epoch: r.john.SessionEpoch})

	require.NoError(t, err)
	assert.True(t, confirmed.Continues)
	assert.Equal(t, "john.new@example.com", confirmed.User.Email)
	assert.Equal(t, r.john.SessionEpoch+1, confirmed.User.SessionEpoch)
}

// The link alone never signs anyone in: no session, an ended one or another User's does not go on.
func TestAConfirmedEmailChangeContinuesNoOtherSession(t *testing.T) {
	holders := map[string]func(john *ent.User) users.SessionHolder{
		"none": func(*ent.User) users.SessionHolder { return users.SessionHolder{} },
		"an ended one": func(john *ent.User) users.SessionHolder {
			return users.SessionHolder{UserID: john.ID, Epoch: john.SessionEpoch - 1}
		},
		"another Users": func(john *ent.User) users.SessionHolder {
			return users.SessionHolder{UserID: fixtures.OwnerJaneID, Epoch: john.SessionEpoch}
		},
	}
	for name, holder := range holders {
		t.Run(name, func(t *testing.T) {
			r := newEmailChangeRig(t)

			confirmed, err := r.links.ConfirmEmailChange(t.Context(), r.token, holder(r.john))

			require.NoError(t, err)
			assert.False(t, confirmed.Continues)
			assert.Equal(t, "john.new@example.com", confirmed.User.Email)
		})
	}
}
