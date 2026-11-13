// Package publicid is a primary key as the APIs show it (ADR 0142): the key, sealed under the
// Instance's id key with AES as one block (the key beside a fixed tag), written in base62. A
// client sees a string that says nothing about how many rows a table holds or when one was made,
// and that maps back to the key without a lookup, so an id of a deleted row (an Erasure's Contact)
// is as readable as any other. The id key never rotates: a new key would change every id a client
// has stored.
package publicid

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"math/big"
	"strings"
	"sync/atomic"
)

// ID is a primary key whose text is its public form: the APIs carry it as an EntityId.
type ID int64

// Length is the length of every public id: 16 bytes in base62.
const Length = 22

const alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// tag fills the block's second half, so a string that is no sealed key fails to open.
var tag = [8]byte{'s', 'p', 'h', 'e', 'r', 'i', 'd', '1'}

// ErrMalformed refuses a string that is no public id: of the wrong form, or not sealed under the
// Instance's id key.
var ErrMalformed = errors.New("publicid: not an id of this Instance")

var block atomic.Pointer[cipher.Block]

// Use sets the Instance's id key, 16 bytes. It is set once at start, before anything writes an id.
func Use(key []byte) error {
	b, err := aes.NewCipher(key)
	if err != nil {
		return fmt.Errorf("publicid: %w", err)
	}
	block.Store(&b)
	return nil
}

func current() cipher.Block {
	b := block.Load()
	if b == nil {
		panic("publicid: the Instance's id key is not set")
	}
	return *b
}

// Encode is the public form of the key, which is positive, as every primary key is.
func Encode(key int64) string {
	if key < 0 {
		panic(fmt.Sprintf("publicid: a key is positive, not %d", key))
	}
	var plain, sealed [16]byte
	binary.BigEndian.PutUint64(plain[:8], uint64(key))
	copy(plain[8:], tag[:])
	current().Encrypt(sealed[:], plain[:])
	s := new(big.Int).SetBytes(sealed[:]).Text(62)
	return strings.Repeat("0", Length-len(s)) + s
}

// Decode is the key of a public id; ErrMalformed for a string that is not one.
func Decode(s string) (int64, error) {
	if len(s) != Length || strings.Trim(s, alphabet) != "" {
		return 0, ErrMalformed
	}
	n, ok := new(big.Int).SetString(s, 62)
	if !ok || n.BitLen() > 128 {
		return 0, ErrMalformed
	}
	var sealed, plain [16]byte
	n.FillBytes(sealed[:])
	current().Decrypt(plain[:], sealed[:])
	key := binary.BigEndian.Uint64(plain[:8])
	if [8]byte(plain[8:]) != tag || key > math.MaxInt64 {
		return 0, ErrMalformed
	}
	return int64(key), nil
}

// MarshalText is the public form.
func (id ID) MarshalText() ([]byte, error) { return []byte(Encode(int64(id))), nil }

// UnmarshalText reads a public form; ErrMalformed for a string that is not one.
func (id *ID) UnmarshalText(text []byte) error {
	key, err := Decode(string(text))
	if err != nil {
		return err
	}
	*id = ID(key)
	return nil
}

// String is the public form, so a formatted ID never shows the key.
func (id ID) String() string { return Encode(int64(id)) }

// Ptr is an optional key as an optional ID.
func Ptr(key *int64) *ID {
	if key == nil {
		return nil
	}
	return new(ID(*key))
}
