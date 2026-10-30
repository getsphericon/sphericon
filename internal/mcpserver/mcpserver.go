// Package mcpserver mounts the MCP surface at /mcp (ADR 0016). It is a projection
// of the external /api contract, not a second implementation: tools are generated
// from the embedded OpenAPI document and each call is dispatched in-process through
// the same ogen server with the caller's own Bearer token, so workspace scoping,
// token scopes, validation and RFC 7807 errors are the /api ones by construction.
package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	apiauth "github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/apitokens"
	"github.com/getsphericon/sphericon/internal/server"
)

// apiPrefix is where the external ogen server is mounted.
const apiPrefix = "/api"

const instructions = "Sphericon marketing automation. Tools mirror the Sphericon external API and act on " +
	"the workspace of the Bearer API token used to connect; results and errors are the API's." + untrustedInstructions

// Authenticator validates a Bearer API token whatever it is used for (the external API's security
// handler). A tool call meets its operation's scopes when it is dispatched to /api.
type Authenticator interface {
	Authenticate(ctx context.Context, t externalapi.BearerAuth) (context.Context, error)
}

var _ Authenticator = (*apiauth.ExternalSecurityHandler)(nil)

// New builds the /mcp http.Handler: Streamable HTTP, stateless, one tool per
// non-hidden operation of the OpenAPI spec. api is the external API handler
// serving under /api; auth validates the connecting Bearer token.
//
// With [WithResourceMetadataURL], 401 responses carry a WWW-Authenticate challenge
// pointing OAuth clients (claude.ai connectors) at the protected resource metadata.
func New(spec []byte, api http.Handler, auth Authenticator, opts ...Option) (http.Handler, error) {
	var cfg options
	for _, o := range opts {
		o(&cfg)
	}
	ops, err := project(spec)
	if err != nil {
		return nil, fmt.Errorf("project MCP tools: %w", err)
	}
	srv := mcp.NewServer(&mcp.Implementation{Name: "sphericon", Version: "1"}, &mcp.ServerOptions{Instructions: instructions})
	sendTools := map[string]bool{}
	for _, op := range ops {
		srv.AddTool(op.tool, op.handler(api))
		if op.send {
			sendTools[op.tool.Name] = true
		}
	}
	books, err := loadPrompts()
	if err != nil {
		return nil, fmt.Errorf("load prompts: %w", err)
	}
	for _, book := range books {
		srv.AddPrompt(book.prompt(), book.handler)
	}
	srv.AddReceivingMiddleware(sendLock(sendTools, auth))
	streamable := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return srv },
		&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true, PropagateRequestCancellation: true},
	)
	return requireToken(auth, cfg.resourceMetadataURL, streamable), nil
}

// sendLock hides send-class tools from tools/list and refuses a direct call to
// them unless the connecting token carries mcp:send.
func sendLock(sendTools map[string]bool, auth Authenticator) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			switch method {
			case "tools/list":
				res, err := next(ctx, method, req)
				list, ok := res.(*mcp.ListToolsResult)
				if err != nil || !ok || canSend(ctx, auth, req.GetExtra()) {
					return res, err
				}
				kept := *list
				kept.Tools = make([]*mcp.Tool, 0, len(list.Tools))
				for _, tool := range list.Tools {
					if !sendTools[tool.Name] {
						kept.Tools = append(kept.Tools, tool)
					}
				}
				return &kept, nil
			case "tools/call":
				if params, ok := req.GetParams().(*mcp.CallToolParamsRaw); ok && sendTools[params.Name] &&
					!canSend(ctx, auth, req.GetExtra()) {
					return toolError("refused: " + params.Name + " is a send-class tool and needs the " + string(apitokens.ScopeMcpSend) + " scope on the API token"), nil
				}
			}
			return next(ctx, method, req)
		}
	}
}

// canSend reports whether the connecting token carries mcp:send.
func canSend(ctx context.Context, auth Authenticator, extra *mcp.RequestExtra) bool {
	if extra == nil || extra.Header == nil {
		return false
	}
	token, ok := strings.CutPrefix(extra.Header.Get("Authorization"), "Bearer ")
	if !ok {
		return false
	}
	ctx, err := auth.Authenticate(ctx, externalapi.BearerAuth{Token: token})
	return err == nil && apiauth.HasScope(apiauth.GetTokenAuth(ctx), apitokens.ScopeMcpSend)
}

// Option configures [New].
type Option func(*options)

type options struct{ resourceMetadataURL string }

// WithResourceMetadataURL advertises the RFC 9728 protected resource metadata URL
// in the WWW-Authenticate header of 401 responses (MCP authorization spec).
func WithResourceMetadataURL(u string) Option {
	return func(o *options) { o.resourceMetadataURL = u }
}

// requireToken rejects requests without a valid Bearer API token (401).
func requireToken(auth Authenticator, metadataURL string, next http.Handler) http.Handler {
	challenge := func(w http.ResponseWriter, params string) {
		parts := []string{}
		if params != "" {
			parts = append(parts, params)
		}
		if metadataURL != "" {
			parts = append(parts, fmt.Sprintf("resource_metadata=%q", metadataURL))
		}
		w.Header().Set("WWW-Authenticate", strings.Join(append([]string{"Bearer"}, strings.Join(parts, ", ")), " "))
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || token == "" {
			challenge(w, "")
			writeProblem(w, http.StatusUnauthorized, "missing bearer token")
			return
		}
		if _, err := auth.Authenticate(r.Context(), externalapi.BearerAuth{Token: token}); err != nil {
			if errors.Is(err, apiauth.ErrUnauthorized) {
				challenge(w, `error="invalid_token"`)
				writeProblem(w, http.StatusUnauthorized, "invalid bearer token")
				return
			}
			// A rate limit, or a valid credential refused for a reason of its own (a Connection
			// whose User is not admitted), answers as on /api.
			server.ProblemErrorHandler(r.Context(), w, r, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), mcpRequestKey{}, r.Context())))
	})
}

// mcpRequestKey carries the /mcp request's own context to the tool call it runs.
type mcpRequestKey struct{}

// boundToRequest ends ctx when the /mcp request that runs the call ends (a request timeout, a
// client that hung up). The SDK runs a tool on a context that keeps the request's values but
// not its end, so without this the /api call would run on after nobody can read its answer.
func boundToRequest(ctx context.Context) (context.Context, context.CancelFunc) {
	request, ok := ctx.Value(mcpRequestKey{}).(context.Context)
	if !ok {
		return context.WithCancel(ctx)
	}
	ctx, cancel := context.WithCancelCause(ctx)
	stop := context.AfterFunc(request, func() { cancel(context.Cause(request)) })
	return ctx, func() {
		stop()
		cancel(nil)
	}
}

// writeProblem renders an RFC 7807 problem.
func writeProblem(w http.ResponseWriter, code int, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(code)
	problem := map[string]any{
		"status": code,
		"title":  http.StatusText(code),
		"detail": detail,
	}
	if err := json.NewEncoder(w).Encode(problem); err != nil {
		slog.Warn("write problem response", "error", err)
	}
}

// handler dispatches a tool call to the /api handler as the caller.
func (op *operation) handler(api http.Handler) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ctx, cancel := boundToRequest(ctx)
		defer cancel()
		args := map[string]any{}
		if len(req.Params.Arguments) > 0 {
			if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
				return toolError("arguments must be a JSON object: " + err.Error()), nil
			}
		}
		httpReq, err := op.request(ctx, args, req.Extra)
		if err != nil {
			return toolError(err.Error()), nil
		}
		rec := httptest.NewRecorder()
		api.ServeHTTP(rec, httpReq)
		return op.markUntrusted(result(rec.Result())), nil
	}
}

// request builds the in-process /api request for the given tool arguments.
func (op *operation) request(ctx context.Context, args map[string]any, extra *mcp.RequestExtra) (*http.Request, error) {
	rest := make(map[string]any, len(args))
	maps.Copy(rest, args)

	path := op.path
	query := url.Values{}
	headers := http.Header{}
	for _, p := range op.params {
		v, ok := rest[p.name]
		if !ok || v == nil {
			continue
		}
		delete(rest, p.name)
		values := scalars(v)
		switch p.in {
		case "path":
			path = strings.ReplaceAll(path, "{"+p.name+"}", url.PathEscape(values[0]))
		case "query":
			query[p.name] = values
		case "header":
			headers.Set(p.name, values[0])
		}
	}
	if strings.Contains(path, "{") {
		return nil, errors.New("missing required path argument for " + op.path)
	}

	var body io.Reader
	if op.hasBody {
		payload := map[string]any{}
		switch {
		case op.bodyWhole:
			if v, ok := rest[bodyArgument]; ok {
				delete(rest, bodyArgument)
				raw, err := json.Marshal(v)
				if err != nil {
					return nil, err
				}
				body = bytes.NewReader(raw)
			}
		default:
			for field := range op.bodyFields {
				if v, ok := rest[field]; ok {
					payload[field] = v
					delete(rest, field)
				}
			}
			raw, err := json.Marshal(payload)
			if err != nil {
				return nil, err
			}
			body = bytes.NewReader(raw)
		}
	}
	if len(rest) > 0 {
		unknown := make([]string, 0, len(rest))
		for k := range rest {
			unknown = append(unknown, k)
		}
		slices.Sort(unknown)
		return nil, errors.New("unknown arguments: " + strings.Join(unknown, ", "))
	}

	target := apiPrefix + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	httpReq, err := http.NewRequestWithContext(ctx, op.method, target, body)
	if err != nil {
		return nil, err
	}
	maps.Copy(httpReq.Header, headers)
	if body != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	httpReq.Header.Set("Accept", "application/json")
	// The caller's own token: the /api security handler authenticates and scopes it.
	if extra != nil && extra.Header != nil {
		httpReq.Header.Set("Authorization", extra.Header.Get("Authorization"))
	}
	return httpReq, nil
}

// scalars renders an argument as one or more query/path/header strings.
func scalars(v any) []string {
	if list, ok := v.([]any); ok {
		out := make([]string, 0, len(list))
		for _, item := range list {
			out = append(out, scalar(item))
		}
		if len(out) == 0 {
			return []string{""}
		}
		return out
	}
	return []string{scalar(v)}
}

func scalar(v any) string {
	if f, ok := v.(float64); ok {
		return strconv.FormatFloat(f, 'f', -1, 64)
	}
	return fmt.Sprint(v)
}

// result maps the /api response to a tool result: the body on success, the body
// (the RFC 7807 problem) as a tool error otherwise.
func result(resp *http.Response) *mcp.CallToolResult {
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return toolError(fmt.Sprintf("HTTP %d %s: the response body could not be read: %v", resp.StatusCode, http.StatusText(resp.StatusCode), err))
	}
	body := strings.TrimSpace(string(raw))
	if resp.StatusCode >= http.StatusBadRequest {
		return toolError(fmt.Sprintf("HTTP %d %s: %s", resp.StatusCode, http.StatusText(resp.StatusCode), body))
	}
	if body == "" {
		body = fmt.Sprintf("HTTP %d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: body}}}
}

func toolError(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: msg}}}
}
