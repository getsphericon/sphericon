package doctor

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/ee/licensekey"
	"github.com/getsphericon/sphericon/ent"
	"github.com/getsphericon/sphericon/ent/outboundmessage"
	"github.com/getsphericon/sphericon/ent/sendingdomain"
	appdb "github.com/getsphericon/sphericon/internal/db"
	"github.com/getsphericon/sphericon/internal/secrets"
	"github.com/getsphericon/sphericon/internal/sending"
)

const upgradeGuide = "docs/operations/upgrading.md"

// checkDatabase covers the connection, the schema Major (what `migrate --status` reads) and
// the Postgres major.
func checkDatabase(ctx context.Context, r *Report, in Input) {
	if in.DB == nil {
		detail := "no connection"
		if in.DBErr != nil {
			detail = in.DBErr.Error()
		}
		r.add("database", Fail, "cannot reach the database: "+detail,
			"Check DATABASE_URL (host, port, user, password, database name) and that Postgres accepts connections from this machine.")
		r.add("postgres version", Fail, "not checked: the database is unreachable", "Fix the database check above first.")
		return
	}

	st, err := appdb.Status(ctx, in.DB)
	if err != nil {
		r.add("database", Fail, "cannot read the schema: "+err.Error(),
			"Check DATABASE_URL and that the user may read the schema; on a new database run `sphericon migrate`.")
	} else {
		checkSchema(ctx, r, in, st)
	}

	var num int
	if err := in.DB.QueryRowContext(ctx, "SHOW server_version_num").Scan(&num); err != nil {
		r.add("postgres version", Fail, "cannot read the server version: "+err.Error(), "Check DATABASE_URL.")
		return
	}
	major := num / 10000
	switch {
	case major < MinPostgresMajor:
		r.add("postgres version", Fail, fmt.Sprintf("PostgreSQL %d is older than the oldest supported, %d", major, MinPostgresMajor),
			fmt.Sprintf("Upgrade PostgreSQL to a major between %d and %d (see %s).", MinPostgresMajor, MaxTestedPostgresMajor, upgradeGuide))
	case major > MaxTestedPostgresMajor:
		r.add("postgres version", Warn, fmt.Sprintf("PostgreSQL %d is newer than the newest tested, %d", major, MaxTestedPostgresMajor),
			fmt.Sprintf("It will probably work; run on a major between %d and %d to stay on tested ground.", MinPostgresMajor, MaxTestedPostgresMajor))
	default:
		r.add("postgres version", Pass, fmt.Sprintf("PostgreSQL %d (supported: %d to %d)", major, MinPostgresMajor, MaxTestedPostgresMajor), "")
	}
}

func checkSchema(ctx context.Context, r *Report, in Input, st appdb.SchemaStatus) {
	detail := fmt.Sprintf("schema Major %d, this binary Major %d", st.State.Major, st.BinaryMajor)
	guard := appdb.Guard{Build: in.Build, AutoMigrate: in.Config.AutoMigrate}
	if err := guard.Check(ctx, in.DB); err != nil {
		r.add("database", Fail, detail+": "+err.Error(), "Follow the remedy in the message; the guide is "+upgradeGuide+".")
		return
	}
	if len(st.Pending) > 0 && !in.Config.AutoMigrate {
		r.add("database", Warn, fmt.Sprintf("%s; %d migrations pending", detail, len(st.Pending)),
			"Run `sphericon migrate`, or set SPHERICON_DB_AUTO_MIGRATE=true.")
		return
	}
	r.add("database", Pass, detail, "")
}

// checkSMTP covers the Instance's own mail (verification, password reset), not a
// Workspace's Integrations.
func checkSMTP(ctx context.Context, r *Report, in Input) {
	cfg := in.Config
	if cfg.SystemEmailProvider == "ses" {
		if cfg.SESRegion == "" || cfg.SESAccessKeyID == "" || cfg.SESSecretAccessKey == "" {
			r.add("system mail", Fail, "SPHERICON_SYSTEM_EMAIL_PROVIDER is ses but the SES region or credentials are missing",
				"Set SPHERICON_SES_REGION, SPHERICON_SES_ACCESS_KEY_ID and SPHERICON_SES_SECRET_ACCESS_KEY.")
			return
		}
		r.add("system mail", Pass, "SES region and credentials are set (not contacted)", "")
		return
	}
	if cfg.SMTPHost == "" {
		if cfg.RequireEmailVerification {
			r.add("system mail", Fail, "SPHERICON_SMTP_HOST is not set, and email verification needs mail",
				"Set SPHERICON_SMTP_HOST (and port, user, password), or set SPHERICON_REQUIRE_EMAIL_VERIFICATION=false for an Instance without outbound mail.")
			return
		}
		r.add("system mail", Skip, "no SMTP host is set and email verification is off", "")
		return
	}
	target := net.JoinHostPort(cfg.SMTPHost, strconv.Itoa(cfg.SMTPPort))
	if err := in.DialSMTP(ctx, cfg); err != nil {
		r.add("system mail", Fail, fmt.Sprintf("cannot send through SMTP %s: %v", target, err),
			"Check SPHERICON_SMTP_HOST, SPHERICON_SMTP_PORT, SPHERICON_SMTP_USER and SPHERICON_SMTP_PASS, and that this machine may reach the relay.")
		return
	}
	detail := "connected to SMTP " + target
	if cfg.SMTPUser != "" {
		detail += " and authenticated"
	}
	r.add("system mail", Pass, detail, "")
}

// checkAppURL covers SPHERICON_APP_URL: well formed, https, and answering.
func checkAppURL(ctx context.Context, r *Report, in Input) {
	cfg := in.Config
	u, err := parseOrigin(cfg.AppURL)
	if err != nil {
		r.add("app url", Fail, fmt.Sprintf("SPHERICON_APP_URL %q is not an origin: %v", cfg.AppURL, err),
			"Set it to the public origin of the Instance, for example https://sphericon.example.com (no path).")
		r.add("app url https", Skip, "needs a well formed SPHERICON_APP_URL", "")
		r.add("app url reachable", Skip, "needs a well formed SPHERICON_APP_URL", "")
		return
	}
	r.add("app url", Pass, u.String(), "")

	loopback := isLoopback(u.Hostname())
	switch {
	case u.Scheme == "https":
		r.add("app url https", Pass, "links and cookies use https", "")
	case loopback || cfg.IsDev:
		r.add("app url https", Warn, "SPHERICON_APP_URL is http: fine for local use only",
			"Serve the Instance over https and set SPHERICON_APP_URL to the https origin before real use.")
	default:
		r.add("app url https", Fail, "SPHERICON_APP_URL is http: session cookies are not Secure and every link in mail is unencrypted",
			"Put the Instance behind an https reverse proxy and set SPHERICON_APP_URL to the https origin.")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String()+"/healthz", nil)
	if err == nil {
		var resp *http.Response
		if resp, err = in.HTTP.Do(req); err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				err = fmt.Errorf("GET /healthz answered %s", resp.Status)
			}
		}
	}
	if err != nil {
		r.add("app url reachable", Fail, fmt.Sprintf("%s does not answer: %v", u, err),
			"Check the DNS name, the reverse proxy and the TLS certificate, and that the Instance is running.")
		return
	}
	r.add("app url reachable", Pass, u.String()+"/healthz answers 200", "")
}

// checkSecrets covers the encryption keyset and the JWT secret.
func checkSecrets(r *Report, in Input) {
	cfg := in.Config
	switch _, err := secrets.NewCipher(cfg.EncryptionKey); {
	case strings.TrimSpace(cfg.EncryptionKey) == "":
		r.add("encryption key", Fail, "SPHERICON_ENCRYPTION_KEY is not set",
			"Run `sphericon genkey`, set the output as SPHERICON_ENCRYPTION_KEY and back it up: without it stored credentials cannot be read.")
	case err != nil:
		r.add("encryption key", Fail, "SPHERICON_ENCRYPTION_KEY is not a valid keyset: "+err.Error(),
			"Restore the key you generated with `sphericon genkey`; do not edit it by hand.")
	default:
		r.add("encryption key", Pass, "the encryption keyset is present and valid", "")
	}

	if err := config.ValidateJWTSecret(cfg.JWTSecret); err != nil {
		// Development boots without it (Load only enforces it elsewhere): a warning there.
		status := Fail
		if cfg.IsDev {
			status = Warn
		}
		r.add("jwt secret", status, err.Error(), "Generate one with `openssl rand -hex 32` and set it as SPHERICON_JWT_SECRET.")
		return
	}
	r.add("jwt secret", Pass, "SPHERICON_JWT_SECRET is present and strong enough", "")
}

// checkLicense reports the License key's state the way the server judges it (ADR 0035).
func checkLicense(r *Report, in Input) {
	cfg := in.Config
	if cfg.LicenseKey == "" {
		r.add("license key", Pass, "none: running the open-source core", "")
		return
	}
	l, err := licensekey.Parse(cfg.LicenseKey, in.LicensePublicKey, in.Now())
	if errors.Is(err, licensekey.ErrInvalid) {
		r.add("license key", Fail, "SPHERICON_LICENSE_KEY is not valid, and the server refuses to start with it: "+err.Error(),
			"Copy the key again from the license email, or unset SPHERICON_LICENSE_KEY to run the core.")
		return
	}
	if err != nil {
		r.add("license key", Fail, err.Error(), "")
		return
	}
	if at, lapsed := l.LapsedAt(); lapsed {
		r.add("license key", Warn, "the license key expired on "+at.UTC().Format("2006-01-02")+": running the open-source core",
			"Set a renewed key as SPHERICON_LICENSE_KEY.")
		return
	}
	var features []string
	for _, f := range []licensekey.Feature{licensekey.FeatureAudit, licensekey.FeatureRetention, licensekey.FeatureStaff} {
		if l.Has(f) {
			features = append(features, string(f))
		}
	}
	r.add("license key", Pass, "valid: "+strings.Join(features, ", "), "")
}

// checkDKIM resolves the DKIM record of every Sending domain of every Workspace.
func checkDKIM(ctx context.Context, r *Report, in Input) {
	if in.Ent == nil {
		r.add("dkim", Fail, "not checked: the database is unreachable", "Fix the database check above first.")
		return
	}
	domains, err := in.Ent.SendingDomain.Query().Order(ent.Asc(sendingdomain.FieldID)).All(ctx)
	if err != nil {
		r.add("dkim", Fail, "cannot list the Sending domains: "+err.Error(), "Fix the database check above first.")
		return
	}
	if len(domains) == 0 {
		r.add("dkim", Skip, "no Sending domains yet", "")
		return
	}
	for _, d := range domains {
		name := "dkim " + d.Domain
		host, _ := sending.DKIMRecord(d.DkimSelector, d.Domain, d.DkimPublicKey)
		ok, err := sending.VerifyDKIM(ctx, in.LookupTXT, d.DkimSelector, d.Domain, d.DkimPublicKey)
		switch {
		case err != nil:
			r.add(name, Fail, "cannot resolve "+host+": "+err.Error(), "Check that this machine can resolve DNS, then run again.")
		case !ok:
			r.add(name, Fail, "no TXT record at "+host+" carries this domain's key",
				"Publish the TXT record shown for the domain under Sending domains in the app, wait for DNS to propagate, then run again.")
		default:
			r.add(name, Pass, "the record at "+host+" matches", "")
		}
	}
}

// checkMailOrigin compares SPHERICON_APP_URL with the origin the newest sent messages carry
// links on (ADR 0036): links already in inboxes keep pointing at the old one.
func checkMailOrigin(ctx context.Context, r *Report, in Input) {
	const name = "app url in sent mail"
	if in.Ent == nil {
		r.add(name, Fail, "not checked: the database is unreachable", "Fix the database check above first.")
		return
	}
	current, err := parseOrigin(in.Config.AppURL)
	if err != nil {
		r.add(name, Skip, "needs a well formed SPHERICON_APP_URL", "")
		return
	}
	origins, err := in.Ent.OutboundMessage.Query().
		Where(outboundmessage.AppOriginNotNil(), outboundmessage.StatusEQ(outboundmessage.StatusSent)).
		Order(ent.Desc(outboundmessage.FieldID)).
		Limit(recentMail).
		Select(outboundmessage.FieldAppOrigin).
		Strings(ctx)
	if err != nil {
		r.add(name, Fail, "cannot read recent mail: "+err.Error(), "Fix the database check above first.")
		return
	}
	if len(origins) == 0 {
		r.add(name, Skip, "no sent mail records its origin yet", "")
		return
	}
	var other []string
	stale := 0
	for _, o := range origins {
		if parsed, err := parseOrigin(o); err != nil || parsed.String() != current.String() {
			stale++
			if !slices.Contains(other, o) {
				other = append(other, o)
			}
		}
	}
	if stale == 0 {
		r.add(name, Pass, fmt.Sprintf("the last %d sent messages used %s", len(origins), current), "")
		return
	}
	r.add(name, Warn, fmt.Sprintf("%d of the last %d sent messages carry links on %s, but SPHERICON_APP_URL is %s",
		stale, len(origins), strings.Join(other, ", "), current),
		"Unsubscribe, confirm and tracking links in mail already delivered still point at the old origin: keep a redirect from it to the new one at the reverse proxy, or restore SPHERICON_APP_URL (ADR 0036).")
}

// parseOrigin reads a public origin: http or https, a host, and nothing else but an optional
// trailing slash. The result is normalized (lowercase scheme and host, no trailing slash).
func parseOrigin(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	switch {
	case err != nil:
		return nil, err
	case u.Scheme != "http" && u.Scheme != "https":
		return nil, errors.New("the scheme must be http or https")
	case u.Hostname() == "":
		return nil, errors.New("the host is missing")
	case u.User != nil:
		return nil, errors.New("it must not carry a user name or password")
	case (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "":
		return nil, errors.New("it must have no path, query or fragment")
	}
	return &url.URL{Scheme: strings.ToLower(u.Scheme), Host: strings.ToLower(u.Host)}, nil
}

func isLoopback(host string) bool {
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
