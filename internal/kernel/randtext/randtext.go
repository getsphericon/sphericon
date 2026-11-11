// Package randtext draws random text from an alphabet: the one generator behind the API token
// prefix and the Workspace id.
package randtext

import (
	"crypto/rand"
	"math/big"
)

// FromAlphabet returns n characters drawn uniformly and independently from alphabet (bytes, not
// runes) with crypto/rand.Int. Like crypto/rand.Text it returns no error: crypto/rand.Reader
// never fails (the runtime aborts if the system source does), and that is rand.Int's only error
// for a positive size.
func FromAlphabet(alphabet string, n int) string {
	size := big.NewInt(int64(len(alphabet)))
	out := make([]byte, n)
	for i := range out {
		k, err := rand.Int(rand.Reader, size)
		if err != nil {
			panic(err)
		}
		out[i] = alphabet[k.Int64()]
	}
	return string(out)
}
