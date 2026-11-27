// Package downloadlinks is the one-time link to a file the app writes as it is read (ADR 0138): a
// CSV export of Contacts, the refused rows of a CSV import. A Member's request names what to
// download and is answered with a link; the browser follows it, so the file streams to disk rather
// than through the page. A link is bound to the session that asked for it, lives a minute and works
// once. It is the owner of the download_links table.
package downloadlinks

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/downloadlink"
	"github.com/getsphericon/sphericon/internal/infra/db"
	"github.com/getsphericon/sphericon/internal/kernel/credentials"
	"github.com/getsphericon/sphericon/internal/kernel/i18n"
	"github.com/getsphericon/sphericon/internal/kernel/validation"
)

// TTL is how long a link stands after it is issued.
const TTL = time.Minute

// Purpose is what a link downloads.
type Purpose = downloadlink.Purpose

const (
	// PurposeContactsExport is the CSV export of a Segment's Contacts, or of all of them.
	PurposeContactsExport = downloadlink.PurposeContactsExport
	// PurposeImportReport is the refused rows of a CSV import.
	PurposeImportReport = downloadlink.PurposeImportReport
)

// ErrLinkInvalid means a link that does not stand: unknown, followed already, lapsed, issued to
// another session or for another download. The answer is the same for each, so a link tells
// nothing about why.
var ErrLinkInvalid = validation.LinkInvalid("downloadlinks: download link invalid, used or expired", i18n.ErrorsDownloadLinkInvalid)

// The unique constraint of the links (ADR 0106): no client can break it.
var _ = db.Declare(
	db.Fault("download_links_token_hash", "a link's token is random (credentials.GenerateTokenSecret), stored by its hash"),
)

// Module issues and follows the links.
type Module struct {
	now func() time.Time
}

// New builds the module on the Instance clock, which dates a link's expiry.
func New(now func() time.Time) *Module { return &Module{now: now} }

// IssueInput is a link to issue: what it downloads, for which session, with what it reads.
type IssueInput struct {
	Purpose Purpose
	// Session is the session token of the request that asks for the link (auth.SiteAuth's), which
	// alone may follow it.
	Session string
	// Payload is what the download reads, encoded as a JSON object.
	Payload any
}

// Link is an issued link: the token it carries and when it lapses.
type Link struct {
	Token     string
	ExpiresAt time.Time
}

// Issue stores a link in the Workspace of s and returns its token, which is shown once: only its
// hash is kept. The Workspace's links that lapsed go with it, so a link never followed lasts until
// the next one is issued.
func (m *Module) Issue(ctx context.Context, s *ent.Scoped, in IssueInput) (Link, error) {
	if in.Session == "" {
		return Link{}, fmt.Errorf("issue download link: no session")
	}
	payload, err := json.Marshal(in.Payload)
	if err != nil {
		return Link{}, fmt.Errorf("encode download link payload: %w", err)
	}
	now := m.now()
	if _, err := s.DownloadLink().Delete().Where(downloadlink.ExpiresAtLTE(now)).Exec(ctx); err != nil {
		return Link{}, fmt.Errorf("delete lapsed download links: %w", err)
	}
	token := credentials.GenerateTokenSecret()
	expires := now.Add(TTL)
	err = s.DownloadLink().Create().
		SetPurpose(in.Purpose).
		SetTokenHash(credentials.HashSecret(token)).
		SetSessionHash(credentials.HashSecret(in.Session)).
		SetPayload(payload).
		SetExpiresAt(expires).
		Exec(ctx)
	if err != nil {
		return Link{}, fmt.Errorf("store download link: %w", err)
	}
	return Link{Token: token, ExpiresAt: expires}, nil
}

// FollowInput is a link followed: its token, the session that follows it and what it is expected
// to download.
type FollowInput struct {
	Purpose Purpose
	Token   string
	Session string
}

// Follow spends the link the token names in the Workspace of s, when it was issued to the session
// for the purpose and has not lapsed, and decodes what it reads into payload. The link is deleted
// by one conditional write, so of two follows only one finds it; any other link is ErrLinkInvalid.
// Run inside the transaction that records what the download does, so a failure there leaves the
// link standing.
func (m *Module) Follow(ctx context.Context, s *ent.Scoped, in FollowInput, payload any) error {
	if in.Token == "" || in.Session == "" {
		return ErrLinkInvalid
	}
	link, err := s.DownloadLink().Query().
		Where(
			downloadlink.TokenHash(credentials.HashSecret(in.Token)),
			downloadlink.SessionHash(credentials.HashSecret(in.Session)),
			downloadlink.PurposeEQ(in.Purpose),
			downloadlink.ExpiresAtGT(m.now()),
		).
		Only(ctx)
	if ent.IsNotFound(err) {
		return ErrLinkInvalid
	}
	if err != nil {
		return fmt.Errorf("find download link: %w", err)
	}
	spent, err := s.DownloadLink().Delete().Where(downloadlink.ID(link.ID)).Exec(ctx)
	if err != nil {
		return fmt.Errorf("spend download link: %w", err)
	}
	if spent == 0 {
		return ErrLinkInvalid
	}
	if err := json.Unmarshal(link.Payload, payload); err != nil {
		return fmt.Errorf("decode download link payload: %w", err)
	}
	return nil
}
