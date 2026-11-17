package jsonvalue_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/getsphericon/sphericon/internal/kernel/jsonvalue"
)

type doc struct {
	Name  string            `json:"name"`
	Parts []json.RawMessage `json:"parts"`
}

func TestDocumentsSpelledOtherwiseAreTheSame(t *testing.T) {
	a := doc{Name: "x", Parts: []json.RawMessage{json.RawMessage(`{"a":1,"b":[true]}`)}}
	b := doc{Name: "x", Parts: []json.RawMessage{json.RawMessage(`{ "b": [ true ], "a": 1 }`)}}

	assert.True(t, jsonvalue.Same(a, b))
}

func TestDocumentsOfOtherValuesDiffer(t *testing.T) {
	a := doc{Name: "x", Parts: []json.RawMessage{json.RawMessage(`{"a":1}`)}}

	assert.False(t, jsonvalue.Same(a, doc{Name: "x", Parts: []json.RawMessage{json.RawMessage(`{"a":2}`)}}))
	assert.False(t, jsonvalue.Same(a, doc{Name: "y", Parts: a.Parts}))
	assert.False(t, jsonvalue.Same(a, doc{Name: "x", Parts: []json.RawMessage{json.RawMessage(`{"a":`)}}), "a document that does not encode is no match")
}
