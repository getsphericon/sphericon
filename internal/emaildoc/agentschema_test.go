package emaildoc_test

import (
	"context"
	"testing"

	"github.com/getsphericon/sphericon/internal/emaildoc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// What an agent is told must not be refused: the example renders, and every value the schema
// allows for an alignment is accepted, while one it does not list is refused.
func TestAgentSchemaExampleAndAllowedValuesRender(t *testing.T) {
	schema := emaildoc.AgentSchema()

	_, err := shared().Render(context.Background(), schema.Example)
	require.NoError(t, err, "the example document is valid")

	allowed := schema.Nodes["paragraph"].Attrs["alignment"].Allowed
	require.NotEmpty(t, allowed)
	for _, value := range allowed {
		_, err := shared().Render(context.Background(),
			doc(node{"type": "paragraph", "attrs": node{"alignment": value}, "content": []any{text("x")}}))
		require.NoErrorf(t, err, "alignment %q is allowed", value)
	}
	_, err = shared().Render(context.Background(),
		doc(node{"type": "paragraph", "attrs": node{"alignment": "justify"}, "content": []any{text("x")}}))
	require.ErrorIs(t, err, emaildoc.ErrInvalid)
}

// The schema lists the editor's structure and tells an agent the rule of the attributes the server
// checks, and which ones are required.
func TestAgentSchemaNamesTheRulesTheServerChecks(t *testing.T) {
	schema := emaildoc.AgentSchema()

	assert.Equal(t, emaildoc.EditorVersion(), schema.EditorVersion)
	assert.Contains(t, schema.Nodes, "heading")
	assert.Contains(t, schema.Marks, "link")
	assert.Equal(t, "integer", schema.Nodes["heading"].Attrs["level"].Type)
	assert.Contains(t, schema.Nodes["button"].Attrs["href"].Rule, "required")
	assert.Contains(t, schema.Marks["link"].Attrs["href"].Rule, "required")
	assert.Equal(t, []string{"1", "a", "A", "i", "I"}, schema.Nodes["orderedList"].Attrs["type"].Allowed)
	for name, node := range schema.Nodes {
		for attr, rule := range node.Attrs {
			assert.NotEmptyf(t, rule.Type, "%s.%s has a type", name, attr)
			assert.NotEmptyf(t, rule.Rule, "%s.%s has a rule", name, attr)
		}
	}
}
