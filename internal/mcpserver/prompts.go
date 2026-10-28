package mcpserver

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// promptFiles are the shipped prompts (ADR 0016, phase 3): Markdown files
// with a one-line description in front matter, served as MCP prompts. They are
// content, not code: edit the files. A prompt names a tool as `tool_name()`,
// and a test checks every such name against the generated tool surface.
//
//go:embed prompts/*.md
var promptFiles embed.FS

type shippedPrompt struct {
	name        string
	description string
	body        string
}

// loadPrompts parses the embedded prompts; the file name is the prompt name.
func loadPrompts() ([]shippedPrompt, error) {
	entries, err := fs.ReadDir(promptFiles, "prompts")
	if err != nil {
		return nil, err
	}
	out := make([]shippedPrompt, 0, len(entries))
	for _, e := range entries {
		raw, err := promptFiles.ReadFile("prompts/" + e.Name())
		if err != nil {
			return nil, err
		}
		head, body, ok := strings.Cut(strings.TrimPrefix(string(raw), "---\n"), "\n---\n")
		description, found := strings.CutPrefix(head, "description: ")
		if !ok || !found {
			return nil, fmt.Errorf("prompt %s: want front matter with a description", e.Name())
		}
		out = append(out, shippedPrompt{
			name:        strings.TrimSuffix(e.Name(), ".md"),
			description: strings.TrimSpace(description),
			body:        strings.TrimSpace(body),
		})
	}
	return out, nil
}

func (p shippedPrompt) prompt() *mcp.Prompt {
	return &mcp.Prompt{Name: p.name, Title: p.name, Description: p.description}
}

func (p shippedPrompt) handler(context.Context, *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
	return &mcp.GetPromptResult{
		Description: p.description,
		Messages:    []*mcp.PromptMessage{{Role: "user", Content: &mcp.TextContent{Text: p.body}}},
	}, nil
}
