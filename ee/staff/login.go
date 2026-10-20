package staff

import (
	"context"
	"fmt"

	"entgo.io/ent/dialect/sql"

	"github.com/getsphericon/sphericon/ent"
	entstaffmember "github.com/getsphericon/sphericon/ent/staffmember"
	"github.com/getsphericon/sphericon/internal/accounts"
	"github.com/getsphericon/sphericon/internal/authtoken"
	"github.com/getsphericon/sphericon/internal/login"
	"github.com/getsphericon/sphericon/internal/otpcode"
)

// loginKind is the Kind under which a Staff member's failed logins are counted: its own,
// so a User of the same address never shares a counter.
const loginKind = accounts.KindStaffLogin

// loginMethod is the method a Staff member's first step proves the person by.
const loginMethod = "password"

// staffStore is the Staff member's side of the login module's seam.
type staffStore struct{ m *Module }

// Lookup finds a Staff member by normalized email. A second factor always follows a
// right password: the TOTP is mandatory, enrolled at the first sign-in.
func (s staffStore) Lookup(ctx context.Context, address string) (login.Account[*ent.StaffMember], bool, error) {
	op, err := s.m.ent.StaffMember.Query().Where(entstaffmember.Email(address)).Only(ctx)
	if ent.IsNotFound(err) {
		return login.Account[*ent.StaffMember]{}, false, nil
	}
	if err != nil {
		return login.Account[*ent.StaffMember]{}, false, err
	}
	return login.Account[*ent.StaffMember]{Principal: op, PasswordHash: op.PasswordHash, SecondFactor: true}, true, nil
}

// newLogin builds the login module over the Staff member's adapter (ADR 0020, 0026).
func (m *Module) newLogin(throttle login.Throttle) *login.Module[*ent.StaffMember] {
	return login.New(login.Config[*ent.StaffMember]{
		Throttle:  throttle,
		Store:     staffStore{m},
		Challenge: m.mintChallenge,
		Parse:     m.parseChallenge,
		Verify:    m.verifyCode,
	})
}

// CheckPassword is the first login step, run by the login module: the answer is a
// challenge (a Staff member always has a second step), never a session. ErrNotLicensed
// without the license.
func (m *Module) CheckPassword(ctx context.Context, email, password string) (login.Result[*ent.StaffMember], error) {
	if err := m.licensed(); err != nil {
		return login.Result[*ent.StaffMember]{}, err
	}
	return m.login.Password(ctx, email, password)
}

// Complete is the second login step, run by the login module. On Authenticated the
// principal is the Staff member as stored after the step (a confirming code bumps the
// session epoch, which the session must carry).
func (m *Module) Complete(ctx context.Context, challenge, code string) (login.Result[*ent.StaffMember], error) {
	if err := m.licensed(); err != nil {
		return login.Result[*ent.StaffMember]{}, err
	}
	res, err := m.login.SecondFactor(ctx, challenge, code)
	if err != nil || res.Outcome != login.Authenticated {
		return res, err
	}
	res.Principal, err = m.ent.StaffMember.Get(ctx, res.Principal.ID)
	return res, err
}

// Enrolled reports whether the Staff member has a confirmed TOTP.
func Enrolled(op *ent.StaffMember) bool {
	return op.TotpConfirmedAt != nil && op.TotpSecretEncrypted != ""
}

// Enrolment is the pending TOTP key of a Staff member that has no confirmed one, nil
// for one that has. It is minted once and shown unchanged by every password step until
// confirmed, so a repeated password login can neither swap it nor invalidate the one
// being enrolled.
func (m *Module) Enrolment(ctx context.Context, op *ent.StaffMember) (*otpcode.Key, error) {
	if Enrolled(op) {
		return nil, nil
	}
	key, _, err := m.pendingEnrolment(ctx, op)
	return key, err
}

// mintChallenge is the login module's Challenge: a Staff member without a confirmed
// TOTP gets a pending secret first (the one already pending when there is one), so there
// is never a path from the password alone to a session.
func (m *Module) mintChallenge(ctx context.Context, op *ent.StaffMember) (string, error) {
	if !Enrolled(op) {
		var err error
		if _, op, err = m.pendingEnrolment(ctx, op); err != nil {
			return "", err
		}
	}
	return m.challenges.Mint(authtoken.PurposeStaffLoginChallenge, op.ID, binding(op), challengeTTL, nil)
}

// pendingEnrolment is the Staff member's unconfirmed TOTP key, minted on first use. The
// write is conditional on no secret being stored, so two password steps racing on a
// fresh Staff member agree on the winner's secret; the Staff member is returned as stored.
func (m *Module) pendingEnrolment(ctx context.Context, op *ent.StaffMember) (*otpcode.Key, *ent.StaffMember, error) {
	if op.TotpSecretEncrypted == "" {
		key, err := otpcode.New(Issuer, op.Email)
		if err != nil {
			return nil, nil, err
		}
		sealed, err := m.cipher.Encrypt([]byte(key.Secret))
		if err != nil {
			return nil, nil, err
		}
		n, err := m.ent.StaffMember.Update().
			Where(entstaffmember.ID(op.ID), entstaffmember.Or(entstaffmember.TotpSecretEncryptedIsNil(), entstaffmember.TotpSecretEncrypted(""))).
			SetTotpSecretEncrypted(sealed).
			SetTotpLastStep(0).
			Save(ctx)
		if err != nil {
			return nil, nil, err
		}
		if op, err = m.ent.StaffMember.Get(ctx, op.ID); err != nil {
			return nil, nil, err
		}
		if n == 1 {
			return key, op, nil
		}
	}
	raw, err := m.cipher.Decrypt(op.TotpSecretEncrypted)
	if err != nil {
		return nil, nil, err
	}
	key, err := otpcode.FromSecret(Issuer, op.Email, string(raw))
	return key, op, err
}

// binding is the value a login challenge is keyed to: it changes on every accepted
// code, session epoch bump, password change and TOTP enrolment or reset, which makes
// the challenge single-use with no store of spent ones.
func binding(op *ent.StaffMember) string {
	return fmt.Sprintf("%d|%d|%t|%s|%s", op.SessionEpoch, op.TotpLastStep, Enrolled(op), op.PasswordHash, op.TotpSecretEncrypted)
}

// parseChallenge is the login module's Parse: the Staff member a still-valid challenge
// was minted for and the binding it verifies under.
func (m *Module) parseChallenge(ctx context.Context, challenge string) (login.Pending[*ent.StaffMember], error) {
	var (
		op      *ent.StaffMember
		held    string
		loadErr error
	)
	_, _, err := m.challenges.Parse(challenge, authtoken.PurposeStaffLoginChallenge, func(id int64) (string, error) {
		if op, loadErr = m.ent.StaffMember.Get(ctx, id); loadErr != nil {
			return "", loadErr
		}
		held = binding(op)
		return held, nil
	})
	if loadErr != nil && !ent.IsNotFound(loadErr) {
		return login.Pending[*ent.StaffMember]{}, loadErr
	}
	if err != nil {
		return login.Pending[*ent.StaffMember]{}, login.ErrChallengeInvalid
	}
	return login.Pending[*ent.StaffMember]{Principal: op, Address: op.Email, Method: loginMethod, Binding: held}, nil
}

// verifyCode is the login module's Verify: the code check in a transaction with the
// Staff member row locked and the binding re-checked, so two steps racing on one
// challenge cannot both succeed. At first login the code also confirms the enrolment
// (and bumps the session epoch). A wrong code changes nothing, so rolling back loses
// nothing.
func (m *Module) verifyCode(ctx context.Context, p login.Pending[*ent.StaffMember], code string) error {
	tx, err := m.ent.Tx(ctx)
	if err != nil {
		return err
	}
	if err := m.verify(ctx, tx, p.Principal.ID, p.Binding, code); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (m *Module) verify(ctx context.Context, tx *ent.Tx, id int64, held, code string) error {
	q := tx.StaffMember.Query().Where(entstaffmember.ID(id))
	q.Modify(func(s *sql.Selector) { s.ForUpdate() })
	op, err := q.Only(ctx)
	if err != nil {
		return err
	}
	if op.TotpSecretEncrypted == "" || binding(op) != held {
		return login.ErrChallengeInvalid
	}
	raw, err := m.cipher.Decrypt(op.TotpSecretEncrypted)
	if err != nil {
		return err
	}
	step, ok, err := otpcode.Match(string(raw), code, m.now(), op.TotpLastStep)
	if err != nil {
		return err
	}
	if !ok {
		return login.ErrInvalidCode
	}
	upd := tx.StaffMember.UpdateOneID(id).SetTotpLastStep(step)
	if !Enrolled(op) {
		upd = upd.SetTotpConfirmedAt(m.now()).AddSessionEpoch(1)
	}
	return upd.Exec(ctx)
}
