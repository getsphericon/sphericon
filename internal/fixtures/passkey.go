package fixtures

import (
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"errors"

	"github.com/go-webauthn/webauthn/protocol/webauthncbor"
	"github.com/go-webauthn/webauthn/protocol/webauthncose"
)

// passkeyCredentialID is the stored credential id of a fixture Passkey: the fixture states it
// in base64url (the form a browser sends it in, lifted into the catalog as
// <Name>CredentialID) and the column holds the raw bytes, as a bytea literal.
func passkeyCredentialID(b64 string) (string, error) {
	id, err := base64.RawURLEncoding.DecodeString(b64)
	if err != nil {
		return "", err
	}
	return byteaLiteral(id), nil
}

// passkeyPublicKey is the COSE public key (ES256) of a fixture Passkey, derived from its
// private key so the two cannot drift apart. The fixture states the private key as base64 PKCS8
// (lifted into the catalog as <Name>PrivateKey, which the test authenticator signs with) and
// the column holds the public half, as a bytea literal.
func passkeyPublicKey(pkcs8 string) (string, error) {
	der, err := base64.StdEncoding.DecodeString(pkcs8)
	if err != nil {
		return "", err
	}
	parsed, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		return "", err
	}
	key, ok := parsed.(*ecdsa.PrivateKey)
	if !ok {
		return "", errors.New("fixtures: a fixture Passkey takes an ECDSA private key")
	}
	cose, err := webauthncbor.Marshal(webauthncose.EC2PublicKeyData{
		KeyType: int64(webauthncose.EllipticKey), Algorithm: int64(webauthncose.AlgES256),
		Curve:  int64(webauthncose.P256),
		XCoord: key.X.FillBytes(make([]byte, 32)),
		YCoord: key.Y.FillBytes(make([]byte, 32)),
	})
	if err != nil {
		return "", err
	}
	return byteaLiteral(cose), nil
}

// byteaLiteral is the text form Postgres reads a bytea from.
func byteaLiteral(b []byte) string { return `\x` + hex.EncodeToString(b) }
