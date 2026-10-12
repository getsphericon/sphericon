package mcpserver_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	apiauth "github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/mcpserver"
)

// specDoc is a small OpenAPI document exercising every argument shape the
// projection supports: path, query (scalar and list), header, object body,
// whole (array) body, untrusted response fields.
const specDoc = `{
 "openapi":"3.0.0",
 "components":{
  "parameters":{"Trace":{"name":"X-Trace","in":"header","schema":{"type":"string"}}},
  "schemas":{
   "Thing":{"type":"object","properties":{"name":{"type":"string","x-untrusted":true},"n":{"type":"integer"}},"required":["name"]},
   "Bag":{"type":"object","properties":{
      "fields":{"type":"object","additionalProperties":{"type":"string","x-untrusted":true}},
      "items":{"type":"array","items":{"$ref":"#/components/schemas/Thing"}},
      "maybe":{"type":"string","nullable":true}}}
  }
 },
 "paths":{
  "/things/{id}":{
   "get":{"operationId":"Things_get","parameters":[
      {"name":"id","in":"path","required":true,"schema":{"type":"string"}},
      {"name":"tag","in":"query","schema":{"type":"array","items":{"type":"string"}}},
      {"name":"limit","in":"query","schema":{"type":"number"}},
      {"$ref":"#/components/parameters/Trace"}],
     "responses":{"200":{"description":"ok","content":{"application/json":{"schema":{"$ref":"#/components/schemas/Bag"}}}}}},
   "patch":{"operationId":"Things_patch","parameters":[{"name":"id","in":"path","schema":{"type":"string"}}],
     "requestBody":{"content":{"application/json":{"schema":{"$ref":"#/components/schemas/Thing"}}}},
     "responses":{"200":{"description":"ok"}}},
   "put":{"operationId":"Things_put","parameters":[{"name":"id","in":"path","schema":{"type":"string"}}],
     "responses":{"204":{"description":"ok"}}},
   "delete":{"operationId":"Things_delete","parameters":[{"name":"id","in":"path","schema":{"type":"string"}}],
     "responses":{"204":{"description":"ok"}}}
  },
  "/bulk":{"post":{"operationId":"Bulk_create","summary":"Bulk",
     "requestBody":{"required":true,"content":{"application/json":{"schema":{"type":"array","items":{"type":"string"}}}}},
     "responses":{"200":{"description":"ok","content":{"application/json":{"schema":{"$ref":"#/components/schemas/Thing"}}}}}}},
  "/hidden":{"get":{"operationId":"Hidden_get","x-mcp":{"hidden":true},"responses":{"200":{"description":"ok"}}}},
  "/named":{"get":{"x-mcp":{"name":"custom_name"},"responses":{"200":{"description":"ok"}}}}
 }
}`

type recorded struct {
	method, uri, body string
	header            http.Header
}

// stubAPI records the dispatched /api request and answers with reply.
type stubAPI struct {
	mu    sync.Mutex
	last  recorded
	reply func(r *http.Request) (int, string)
}

func (s *stubAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var b []byte
	if r.Body != nil {
		b, _ = io.ReadAll(r.Body)
	}
	s.mu.Lock()
	s.last = recorded{method: r.Method, uri: r.URL.RequestURI(), body: string(b), header: r.Header.Clone()}
	s.mu.Unlock()
	code, body := s.reply(r)
	w.WriteHeader(code)
	_, _ = io.WriteString(w, body)
}

func (s *stubAPI) seen() recorded {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.last
}

// fakeAuth accepts the token "good"; any other token is unauthorized, "boom" is an outage.
type fakeAuth struct{}

func (fakeAuth) HandleBearerAuth(ctx context.Context, _ externalapi.OperationName, t externalapi.BearerAuth) (context.Context, error) {
	switch t.Token {
	case "good":
		return apiauth.WithTokenAuth(ctx, &apiauth.TokenAuth{Scopes: []string{"mcp:send"}}), nil
	case "boom":
		return ctx, errors.New("database down")
	}
	return ctx, apiauth.ErrUnauthorized
}

type headerTripper struct {
	h     http.Handler
	token string
}

func (t headerTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+t.token)
	rec := httptest.NewRecorder()
	t.h.ServeHTTP(rec, r)
	return rec.Result(), nil
}

func specSession(t *testing.T, api *stubAPI) *mcp.ClientSession {
	t.Helper()
	h, err := mcpserver.New([]byte(specDoc), api, fakeAuth{})
	require.NoError(t, err)
	transport := &mcp.StreamableClientTransport{
		Endpoint:             "http://local/mcp",
		HTTPClient:           &http.Client{Transport: headerTripper{h: h, token: "good"}},
		DisableStandaloneSSE: true,
		MaxRetries:           -1,
	}
	s, err := mcp.NewClient(&mcp.Implementation{Name: "t", Version: "0"}, nil).Connect(t.Context(), transport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func okJSON(body string) func(*http.Request) (int, string) {
	return func(*http.Request) (int, string) { return http.StatusOK, body }
}

func TestSpecProjectionListsToolsWithAnnotationsAndSchemas(t *testing.T) {
	s := specSession(t, &stubAPI{reply: okJSON(`{}`)})
	list, err := s.ListTools(t.Context(), nil)
	require.NoError(t, err)

	byName := map[string]*mcp.Tool{}
	for _, tool := range list.Tools {
		byName[tool.Name] = tool
	}
	assert.ElementsMatch(t, []string{"things_get", "things_patch", "things_put", "things_delete", "bulk_create", "custom_name"},
		func() []string {
			var n []string
			for k := range byName {
				n = append(n, k)
			}
			return n
		}(), "hidden operations are not tools; names come from x-mcp or the snake-cased operation id")

	assert.True(t, byName["things_get"].Annotations.ReadOnlyHint)
	assert.True(t, byName["things_put"].Annotations.IdempotentHint)
	assert.True(t, *byName["things_delete"].Annotations.DestructiveHint)
	assert.False(t, byName["things_patch"].Annotations.IdempotentHint, "a patch is destructive but not idempotent")
	assert.True(t, *byName["things_patch"].Annotations.DestructiveHint)
	assert.False(t, *byName["bulk_create"].Annotations.DestructiveHint, "a post only adds")
	assert.Equal(t, "Bulk", byName["bulk_create"].Description)

	raw, err := json.Marshal(byName["bulk_create"].InputSchema)
	require.NoError(t, err)
	var in map[string]any
	require.NoError(t, json.Unmarshal(raw, &in))
	assert.Equal(t, []any{"body"}, in["required"], "a required non-object body is the required `body` argument")
}

func TestSpecToolCallBuildsTheAPIRequestFromArguments(t *testing.T) {
	api := &stubAPI{reply: okJSON(`{"fields":{"a":"x"},"items":[{"name":"n1","n":1}],"maybe":null}`)}
	s := specSession(t, api)

	res := call(t, s, "things_get", map[string]any{"id": "a b", "tag": []any{"x", "y"}, "limit": 2.5, "X-Trace": "t-1"})
	require.False(t, res.IsError, text(t, res))
	got := api.seen()
	assert.Equal(t, http.MethodGet, got.method)
	assert.Equal(t, "/api/things/a%20b?limit=2.5&tag=x&tag=y", got.uri)
	assert.Equal(t, "t-1", got.header.Get("X-Trace"))
	assert.Equal(t, "Bearer good", got.header.Get("Authorization"), "the caller's own token is forwarded")
	assert.JSONEq(t, `{"fields":{"a":{"untrusted_data":"x"}},"items":[{"name":{"untrusted_data":"n1"},"n":1}],"maybe":null}`, text(t, res),
		"untrusted values are wrapped through additionalProperties, arrays and refs; null stays null")

	call(t, s, "things_get", map[string]any{"id": "1", "tag": []any{}})
	assert.Equal(t, "/api/things/1?tag=", api.seen().uri, "an empty list is one empty value")

	res = call(t, s, "things_patch", map[string]any{"id": "7", "name": "renamed", "n": 3})
	require.False(t, res.IsError, text(t, res))
	got = api.seen()
	assert.Equal(t, http.MethodPatch, got.method)
	assert.JSONEq(t, `{"name":"renamed","n":3}`, got.body)
	assert.Equal(t, "application/json", got.header.Get("Content-Type"))
}

func TestSpecToolCallWholeBody(t *testing.T) {
	api := &stubAPI{reply: okJSON(`{"name":"made"}`)}
	s := specSession(t, api)

	res := call(t, s, "bulk_create", map[string]any{"body": []any{"a", "b"}})
	require.False(t, res.IsError, text(t, res))
	assert.JSONEq(t, `["a","b"]`, api.seen().body)
	assert.JSONEq(t, `{"name":{"untrusted_data":"made"}}`, text(t, res))

	call(t, s, "bulk_create", map[string]any{})
	assert.Empty(t, api.seen().body, "no body argument sends no body")
}

func TestSpecToolCallRefusesBadArguments(t *testing.T) {
	api := &stubAPI{reply: okJSON(`{}`)}
	s := specSession(t, api)

	res := call(t, s, "things_patch", map[string]any{"name": "x"})
	require.True(t, res.IsError)
	assert.Contains(t, text(t, res), "id", "the schema or the builder names the missing path argument")

	res = call(t, s, "things_put", map[string]any{"id": "1", "bogus": true})
	require.True(t, res.IsError)
	assert.Empty(t, api.seen().method, "nothing is dispatched for refused arguments")
}

func TestSpecResultsPassThroughWhenNotMarkable(t *testing.T) {
	api := &stubAPI{}
	s := specSession(t, api)

	api.reply = func(*http.Request) (int, string) { return http.StatusOK, "plain text, not json" }
	res := call(t, s, "things_get", map[string]any{"id": "1"})
	require.False(t, res.IsError)
	assert.Equal(t, "plain text, not json", text(t, res))

	api.reply = func(*http.Request) (int, string) { return http.StatusOK, "" }
	res = call(t, s, "things_get", map[string]any{"id": "1"})
	assert.Equal(t, "HTTP 200 OK", text(t, res), "an empty body is reported by status")

	api.reply = func(*http.Request) (int, string) { return http.StatusNotFound, `{"detail":"nope"}` }
	res = call(t, s, "things_get", map[string]any{"id": "1"})
	require.True(t, res.IsError)
	assert.Contains(t, text(t, res), "HTTP 404 Not Found")
	assert.Contains(t, text(t, res), `"nope"`, "the problem is not rewritten as untrusted data")
}

func TestNewRefusesInvalidSpecs(t *testing.T) {
	api := &stubAPI{reply: okJSON(`{}`)}
	op := func(paths string) []byte {
		return []byte(`{"components":{"parameters":{}},"paths":` + paths + `}`)
	}
	for name, spec := range map[string][]byte{
		"not json":                 []byte(`{`),
		"no operation id or name":  op(`{"/a":{"get":{"responses":{}}}}`),
		"duplicate tool names":     op(`{"/a":{"get":{"operationId":"X_y"}},"/b":{"get":{"operationId":"X_y"}}}`),
		"unresolved parameter ref": op(`{"/a":{"get":{"operationId":"A_b","parameters":[{"$ref":"#/components/parameters/Nope"}]}}}`),
		"non-json body":            op(`{"/a":{"post":{"operationId":"A_b","requestBody":{"content":{"text/plain":{"schema":{"type":"string"}}}}}}}`),
		"parameter and body clash": op(`{"/a":{"post":{"operationId":"A_b","parameters":[{"name":"name","in":"query","schema":{"type":"string"}}],"requestBody":{"content":{"application/json":{"schema":{"type":"object","properties":{"name":{"type":"string"}}}}}}}}}`),
		"duplicate parameter":      op(`{"/a":{"get":{"operationId":"A_b","parameters":[{"name":"p","in":"query"},{"name":"p","in":"query"}]}}}`),
	} {
		_, err := mcpserver.New(spec, api, fakeAuth{})
		assert.Error(t, err, name)
	}
}

func TestGatewayAnswersAuthFailuresAndOutages(t *testing.T) {
	h, err := mcpserver.New([]byte(specDoc), &stubAPI{reply: okJSON(`{}`)}, fakeAuth{}, mcpserver.WithResourceMetadataURL("https://app.test/.well-known/oauth-protected-resource/mcp"))
	require.NoError(t, err)
	do := func(auth string) *httptest.ResponseRecorder {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/mcp", strings.NewReader(`{}`))
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	rec := do("")
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Contains(t, rec.Header().Get("WWW-Authenticate"), "resource_metadata=")

	rec = do("Bearer bad")
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Contains(t, rec.Header().Get("WWW-Authenticate"), `error="invalid_token"`)

	rec = do("Bearer boom")
	assert.Equal(t, http.StatusInternalServerError, rec.Code, "an outage is a 500, not a 401")
	assert.NotContains(t, rec.Header().Get("WWW-Authenticate"), "invalid_token")
}
