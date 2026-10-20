package config

import (
	"fmt"
	"strings"
)

// SigninCredentials is the OAuth client of one Sign-in provider (ADR 0048): the Instance
// operator registers an app with the provider and sets its id and secret.
type SigninCredentials struct {
	ClientID     string
	ClientSecret string
}

// signinVars is one provider's pair of variables as read from the environment, with the
// names they were read from.
type signinVars struct {
	name               string
	id, secret         string
	idName, secretName string
}

// signinCredentials builds the enabled providers from the two variables each one reads.
// A provider is enabled by setting both and left off by setting neither; one without the
// other stops the start, naming the missing variable, so a typo is found at boot and not
// on the first click of a visitor.
func signinCredentials(vars []signinVars) (map[string]SigninCredentials, error) {
	out := map[string]SigninCredentials{}
	for _, v := range vars {
		id, secret := strings.TrimSpace(v.id), strings.TrimSpace(v.secret)
		switch {
		case id == "" && secret == "":
		case id == "":
			return nil, fmt.Errorf("%s is set but %s is not: set both or neither", v.secretName, v.idName)
		case secret == "":
			return nil, fmt.Errorf("%s is set but %s is not: set both or neither", v.idName, v.secretName)
		default:
			out[v.name] = SigninCredentials{ClientID: id, ClientSecret: secret}
		}
	}
	return out, nil
}
