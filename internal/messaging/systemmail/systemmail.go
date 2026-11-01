// Package systemmail builds sphericon's own (platform) email sender: the one that mails Users
// and Workspace owners on the Instance's behalf (verification, resets, notices), as opposed to a
// Workspace's sender. It is configured by the self-hoster, not by a tenant.
package systemmail

import (
	"cmp"
	"encoding/json"

	"github.com/getsphericon/sphericon/config"
	"github.com/getsphericon/sphericon/internal/messaging"
)

// defaultFromName is the display name of system mail when SPHERICON_SYSTEM_EMAIL_FROM_NAME is unset.
const defaultFromName = "sphericon"

// Sender builds the system sender from config through the catalog: "ses" is SES, any other
// provider name the SMTP relay (mailpit in development). The From address is
// SPHERICON_SYSTEM_EMAIL_FROM. Same abstraction as a
// Workspace's sends, just the Instance's own provider; it is never signed with a Workspace's
// Sending domain.
func Sender(cfg *config.Config, catalog *messaging.Catalog) (messaging.EmailSender, error) {
	from := cfg.SystemEmailFrom
	fromName := cmp.Or(cfg.SystemEmailFromName, defaultFromName)
	if cfg.SystemEmailProvider == string(messaging.ProviderSES) {
		blob, err := json.Marshal(map[string]any{
			"region":          cfg.SESRegion,
			"accessKeyId":     cfg.SESAccessKeyID,
			"secretAccessKey": cfg.SESSecretAccessKey,
			"from":            from,
			"fromName":        fromName,
			"replyTo":         cfg.SystemEmailReplyTo,
		})
		if err != nil {
			return nil, err
		}
		return catalog.BuildEmail(messaging.ProviderSES, blob, messaging.Deps{})
	}
	blob, err := json.Marshal(map[string]any{
		"host":     cfg.SMTPHost,
		"port":     cfg.SMTPPort,
		"username": cfg.SMTPUser,
		"password": cfg.SMTPPass,
		"from":     from,
		"fromName": fromName,
		"replyTo":  cfg.SystemEmailReplyTo,
	})
	if err != nil {
		return nil, err
	}
	return catalog.BuildEmail(messaging.ProviderSMTP, blob, messaging.Deps{})
}
