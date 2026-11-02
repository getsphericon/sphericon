package testhelper

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	ht "github.com/ogen-go/ogen/http"
)

// EditBody wraps a client's transport so that each request's JSON body is edited before it is
// sent: a test sends what the typed client cannot build (a key left out, a value the contract
// does not list) and sees how the server answers it.
func EditBody(edit func(body map[string]any)) func(ht.Client) ht.Client {
	return func(inner ht.Client) ht.Client { return bodyEditor{inner: inner, edit: edit} }
}

type bodyEditor struct {
	inner ht.Client
	edit  func(map[string]any)
}

func (c bodyEditor) Do(r *http.Request) (*http.Response, error) {
	if r.Body == nil {
		return c.inner.Do(r)
	}
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("read request body: %w", err)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, fmt.Errorf("decode request body: %w", err)
	}
	c.edit(body)
	edited, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encode request body: %w", err)
	}
	r.Body = io.NopCloser(bytes.NewReader(edited))
	r.ContentLength = int64(len(edited))
	return c.inner.Do(r)
}
