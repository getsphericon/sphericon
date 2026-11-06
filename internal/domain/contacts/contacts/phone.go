package contacts

import (
	"context"
	"errors"

	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/internal/kernel/patch"
	"github.com/getsphericon/sphericon/internal/kernel/phone"
)

// e164 is a trimmed phone in E.164 (ADR 0075), a number without a country code read with country;
// nil stays nil. A phone that cannot be is refused with its error on the phone field.
func e164(p, country *string) (*string, error) {
	if p == nil {
		return nil, nil
	}
	v, err := phone.Normalize(*p, lo.FromPtr(country))
	switch {
	case errors.Is(err, phone.ErrCountryRequired):
		return nil, ErrPhoneCountryRequired
	case errors.Is(err, phone.ErrExtension):
		return nil, ErrPhoneExtension
	case err != nil:
		return nil, ErrPhoneInvalid
	}
	return &v, nil
}

// patchedPhone is the phone an update sets, in E.164, read with the country the same update sets
// or, when it leaves it, the Contact's stored one.
func patchedPhone(in UpdateInput, stored *string) (patch.Field[string], error) {
	given, ok := in.Phone.Get()
	if !ok {
		return in.Phone, nil
	}
	p, err := e164(&given, in.Country.Over(stored))
	if err != nil {
		return patch.Field[string]{}, err
	}
	return patch.Set(*p), nil
}

// upsertedPhone is the phone an upsert asserts, in E.164. A number without a country code is read
// with the country the upsert sets, or else with the country of the Contact its other alias keys
// resolve, so a local number from a door that knows the person another way still lands on them.
func upsertedPhone(ctx context.Context, s *ent.Scoped, in UpsertInput) (*string, error) {
	if in.Country.Changes() {
		return e164(in.Phone, in.Country.Ptr())
	}
	p, err := e164(in.Phone, nil)
	if !errors.Is(err, ErrPhoneCountryRequired) {
		return p, err
	}
	known, rerr := Resolve(ctx, s, in.SubjectID, in.Email, nil)
	if rerr != nil {
		return nil, rerr
	}
	if known == nil || known.Country == nil {
		return nil, err
	}
	return e164(in.Phone, known.Country)
}
