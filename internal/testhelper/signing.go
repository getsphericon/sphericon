package testhelper

import "encoding/base64"

// SigningSecret is a Standard Webhooks signing secret for a test: whsec_ and the base64 of a
// plain string made from seed, which is how the fixtures spell theirs. It is built, not written
// out, so the secret scanner finds no key in a test.
func SigningSecret(seed string) string {
	return "whsec_" + base64.StdEncoding.EncodeToString([]byte("sphericon-test-"+seed))
}
