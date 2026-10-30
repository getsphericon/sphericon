package oauthserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/oauthclient"
	"github.com/getsphericon/sphericon/internal/webhook"
)

const (
	// maxMetadataBytes caps a Client ID Metadata Document (the MCP spec suggests 5 KB).
	maxMetadataBytes = 10 << 10

	// metadataTTL is how long a document without usable cache headers is kept; metadataMaxTTL
	// caps what the client's own headers may ask for.
	metadataTTL    = time.Hour
	metadataMaxTTL = 24 * time.Hour

	// metadataFetchTimeout bounds the fetch of one document.
	metadataFetchTimeout = 5 * time.Second
)

// NewFetcher is the client that downloads metadata documents: SSRF-hardened, honouring the
// Instance's internal allowlist (ADR 0039), never following redirects.
func NewFetcher(allow webhook.Allowlist) webhook.Doer {
	return webhook.NewClientAllowing(metadataFetchTimeout, allow)
}

// metadataDocument is the part of a Client ID Metadata Document that sphericon reads.
type metadataDocument struct {
	ClientID                string   `json:"client_id"`
	ClientName              string   `json:"client_name"`
	RedirectURIs            []string `json:"redirect_uris"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
}

// IsMetadataClientID reports whether a client_id is a URL (a Client ID Metadata Document)
// rather than an identifier handed out by dynamic client registration.
func IsMetadataClientID(clientID string) bool {
	return strings.HasPrefix(clientID, "https://") || strings.HasPrefix(clientID, "http://")
}

// ClientHost is the host of a metadata client's client_id, shown on the consent screen so
// the user sees who is asking; empty for a registered client.
func ClientHost(clientID string) string {
	if !IsMetadataClientID(clientID) {
		return ""
	}
	u, err := url.Parse(clientID)
	if err != nil {
		return ""
	}
	return u.Host
}

// metadataClient returns the client a URL client_id names: the cached row while fresh,
// otherwise the document fetched again and stored.
func (s *Service) metadataClient(ctx context.Context, clientID string) (*ent.OAuthClient, error) {
	if err := validMetadataURL(clientID); err != nil {
		return nil, err
	}
	row, err := s.ent.OAuthClient.Query().Where(oauthclient.ClientID(clientID)).Only(ctx)
	if err != nil && !ent.IsNotFound(err) {
		return nil, err
	}
	if row != nil && row.MetadataExpiresAt != nil && row.MetadataExpiresAt.After(time.Now()) {
		return row, nil
	}

	doc, ttl, err := s.fetchMetadata(ctx, clientID)
	if err != nil {
		return nil, err
	}
	name := truncate(strings.TrimSpace(doc.ClientName), maxClientName)
	if name == "" {
		name = defaultClient
	}
	err = s.ent.OAuthClient.Create().
		SetClientID(clientID).
		SetName(name).
		SetRedirectUris(doc.RedirectURIs).
		SetMetadataExpiresAt(time.Now().Add(ttl)).
		OnConflictColumns(oauthclient.FieldClientID).
		UpdateName().
		UpdateRedirectUris().
		UpdateMetadataExpiresAt().
		Exec(ctx)
	if err != nil {
		return nil, err
	}
	return s.ent.OAuthClient.Query().Where(oauthclient.ClientID(clientID)).Only(ctx)
}

// validMetadataURL checks the shape the MCP spec requires of a metadata client_id: an
// https URL with a path and no fragment or credentials.
func validMetadataURL(raw string) error {
	u, err := url.Parse(raw)
	switch {
	case err != nil || u.Host == "":
		return ErrInvalidRequest.With("client_id is not a valid URL")
	case u.Scheme != "https":
		return ErrInvalidRequest.With("a client_id URL must use https")
	case u.Fragment != "" || u.User != nil || u.Path == "" || u.Path == "/":
		return ErrInvalidRequest.With("client_id must be an https URL with a path, and no fragment or credentials")
	}
	return nil
}

// fetchMetadata downloads and validates the document at clientID through the SSRF-hardened
// client, and returns it with how long it may be cached.
func (s *Service) fetchMetadata(ctx context.Context, clientID string) (*metadataDocument, time.Duration, error) {
	ctx, cancel := context.WithTimeout(ctx, metadataFetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, clientID, http.NoBody)
	if err != nil {
		return nil, 0, ErrInvalidRequest.With("client_id is not a valid URL")
	}
	req.Header.Set("Accept", "application/json")
	resp, err := s.fetcher.Do(req)
	if err != nil {
		return nil, 0, ErrInvalidRequest.With("the client metadata document could not be fetched")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, 0, ErrInvalidRequest.With("the client metadata document could not be fetched")
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxMetadataBytes+1))
	if err != nil || len(body) > maxMetadataBytes {
		return nil, 0, ErrInvalidRequest.With(fmt.Sprintf("the client metadata document is unreadable or larger than %d bytes", maxMetadataBytes))
	}
	var doc metadataDocument
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, 0, ErrInvalidRequest.With("the client metadata document is not a JSON object")
	}
	if err := doc.validate(clientID); err != nil {
		return nil, 0, err
	}
	return &doc, cacheTTL(resp.Header.Get("Cache-Control")), nil
}

func (d *metadataDocument) validate(clientID string) error {
	switch {
	case d.ClientID != clientID:
		return ErrInvalidRequest.With("client_id in the metadata document does not match its URL")
	case len(d.RedirectURIs) == 0 || len(d.RedirectURIs) > maxRedirectURIs:
		return ErrInvalidRequest.With(fmt.Sprintf("the metadata document needs between 1 and %d redirect_uris", maxRedirectURIs))
	case d.TokenEndpointAuthMethod != "" && d.TokenEndpointAuthMethod != "none":
		return ErrInvalidRequest.With("only token_endpoint_auth_method none is supported")
	}
	for _, u := range d.RedirectURIs {
		if !validRedirectURI(u) {
			return ErrInvalidRequest.With("redirect_uris must be https URLs (or http on a loopback host) without a fragment")
		}
	}
	return nil
}

// cacheTTL reads max-age from a Cache-Control header, capped at metadataMaxTTL; no-store
// and no-cache mean fetch again every time, and a missing header means metadataTTL.
func cacheTTL(header string) time.Duration {
	ttl := metadataTTL
	for directive := range strings.SplitSeq(strings.ToLower(header), ",") {
		directive = strings.TrimSpace(directive)
		switch {
		case directive == "no-store" || directive == "no-cache":
			return 0
		case strings.HasPrefix(directive, "max-age="):
			if n, err := strconv.Atoi(strings.TrimPrefix(directive, "max-age=")); err == nil && n >= 0 {
				ttl = time.Duration(n) * time.Second
			}
		}
	}
	return min(ttl, metadataMaxTTL)
}
