// Package erasures owns the Erasures of the Export streams (ADR 0045, GLOSSARY "Export stream"):
// the record an erased Contact leaves so a consumer of the streams learns of the removal, written
// by the consumer of `contact.erased` and removed by its prune job and by a Workspace's erase.
//
// Governed by ee/LICENSE, not the AGPL.
package erasures

import (
	"context"
	"fmt"
	"time"

	"github.com/getsphericon/sphericon/internal/kernel/secrets"
	"github.com/samber/lo"

	"github.com/getsphericon/sphericon/ee/licensekey"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/erasure"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/infra/events"
)

// The unique constraint of the Erasures table (ADR 0106): one row per erasure.Requested Event.
var _ = db.Declare(
	db.Fault("erasures_workspace_id_erasure_key", "a row is inserted ON CONFLICT (workspace_id, erasure_key), the Event's envelope id"),
)

// Record adds the Erasure a contact.erased event carries to the Erasures stream, once:
// deduplicated by the envelope id, so a redelivered event writes one row. It is a raw-client entry
// like the Audit log's (an envelope carries only a Workspace id, ADR 0017), run by the stream's
// consumer of the domain events (ADR 0101). Unlicensed, it consumes and discards: an instance
// without the `export` feature stores nothing (ADR 0045).
func Record(ctx context.Context, client *ent.Client, lic *licensekey.License, cipher *secrets.Cipher, env events.Envelope) error {
	if !lic.Has(licensekey.FeatureExport) {
		return nil
	}
	ev, err := events.Decode(env)
	if err != nil {
		return err
	}
	erased, ok := ev.(*events.ContactErased)
	if !ok {
		return nil
	}
	return record(ctx, client, cipher, env, erased)
}

func record(ctx context.Context, client *ent.Client, cipher *secrets.Cipher, env events.Envelope, e *events.ContactErased) error {
	create := client.Scoped(env.WorkspaceID).Erasure().Create().
		SetErasureKey(env.ID).
		SetSubjectID(e.SubjectID).
		SetVisitorIds(append([]string{}, e.VisitorIDs...)).
		SetErasedAt(env.OccurredAt)
	if e.ContactID != 0 {
		create.SetContactID(e.ContactID)
	}
	email, err := seal(cipher, e.EmailHash)
	if err != nil {
		return err
	}
	phone, err := seal(cipher, e.PhoneHash)
	if err != nil {
		return err
	}
	create.SetNillableEmailHashEncrypted(email).SetNillablePhoneHashEncrypted(phone)
	if err := create.
		OnConflictColumns(erasure.FieldWorkspaceID, erasure.FieldErasureKey).
		Ignore().
		Exec(ctx); err != nil {
		return fmt.Errorf("record erasure %q: %w", env.ID, err)
	}
	return nil
}

// seal is a hash as the table stores it, sealed with the Instance's cipher; nil for none.
func seal(cipher *secrets.Cipher, hash string) (*string, error) {
	if hash == "" {
		return nil, nil
	}
	sealed, err := cipher.Encrypt([]byte(hash))
	if err != nil {
		return nil, fmt.Errorf("seal an erasure hash: %w", err)
	}
	return &sealed, nil
}

// SecretSweeps are the sealed hashes of every Erasure, as the Secret re-encryption sweeps them
// (secrets.Cipher.Reencrypt), on the raw client.
func SecretSweeps(c *ent.Client) []secrets.Sweep {
	sweep := func(column string, value func(*ent.Erasure) *string, swap func(*ent.ErasureUpdate, string, string) *ent.ErasureUpdate) secrets.Sweep {
		return secrets.Sweep{
			Column: erasure.Table + "." + column,
			Page: func(ctx context.Context, after int64, n int) ([]secrets.Stored, error) {
				rs, err := c.Erasure.Query().Where(erasure.IDGT(after)).Order(ent.Asc(erasure.FieldID)).Limit(n).All(ctx)
				return lo.FilterMap(rs, func(e *ent.Erasure, _ int) (secrets.Stored, bool) {
					v := value(e)
					return secrets.Stored{ID: e.ID, Value: lo.FromPtr(v)}, v != nil
				}), err
			},
			Swap: func(ctx context.Context, id int64, prev, next string) (bool, error) {
				n, err := swap(c.Erasure.Update().Where(erasure.ID(id)), prev, next).Save(ctx)
				return n > 0, err
			},
		}
	}
	return []secrets.Sweep{
		sweep(erasure.FieldEmailHashEncrypted, func(e *ent.Erasure) *string { return e.EmailHashEncrypted },
			func(u *ent.ErasureUpdate, prev, next string) *ent.ErasureUpdate {
				return u.Where(erasure.EmailHashEncrypted(prev)).SetEmailHashEncrypted(next)
			}),
		sweep(erasure.FieldPhoneHashEncrypted, func(e *ent.Erasure) *string { return e.PhoneHashEncrypted },
			func(u *ent.ErasureUpdate, prev, next string) *ent.ErasureUpdate {
				return u.Where(erasure.PhoneHashEncrypted(prev)).SetPhoneHashEncrypted(next)
			}),
	}
}

// Interval is how often the Edition prunes the stream; the window is in days, so hourly is ample.
const Interval = time.Hour

// Prune deletes the Erasures rows of every Workspace that are older than retention before now
// and returns how many it removed. Rows live for the Event retention window: past it the Events
// they announce are gone from a fresh read anyway, and a consumer that was away longer re-seeds
// from an empty cursor. A retention of zero or less keeps every row for good. Without the
// `export` license it removes nothing.
func Prune(ctx context.Context, client *ent.Client, lic *licensekey.License, retention time.Duration, now time.Time) (int, error) {
	if retention <= 0 || !lic.Has(licensekey.FeatureExport) {
		return 0, nil
	}
	n, err := client.Erasure.Delete().Where(erasure.ErasedAtLT(now.Add(-retention))).Exec(ctx)
	if err != nil {
		return 0, fmt.Errorf("prune erasures: %w", err)
	}
	return n, nil
}
