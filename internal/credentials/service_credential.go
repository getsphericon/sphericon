package credentials

const serviceCredentialPrefix = "smsc"

var serviceCredentialRe = prefixedTokenRe(serviceCredentialPrefix)

// GenerateServiceCredential returns a new Service credential's visible prefix, its
// secret and the value a service presents (`smsc_<prefix>_<secret>`). The prefix of an
// API token is `omtk`, so one kind never parses as the other.
func GenerateServiceCredential() (prefix, secret, value string, err error) {
	if prefix, err = GenerateTokenPrefix(); err != nil {
		return "", "", "", err
	}
	if secret, err = GenerateTokenSecret(); err != nil {
		return "", "", "", err
	}
	return prefix, secret, ServiceCredentialValue(prefix, secret), nil
}

// ServiceCredentialValue is the value a service presents for a credential.
func ServiceCredentialValue(prefix, secret string) string {
	return serviceCredentialPrefix + "_" + prefix + "_" + secret
}

// ParseServiceCredential splits a presented value into its prefix and secret; nil when
// the value is not shaped like a Service credential.
func ParseServiceCredential(value string) *ParsedToken {
	return parseWith(serviceCredentialRe, value)
}
