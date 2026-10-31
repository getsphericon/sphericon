package external

import (
	"context"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/emaildoc"
)

// EmailSchemaGet describes the editor's document to an agent (ADR 0040): the structure the editor
// derives, with the attribute rules the server checks. It holds no Workspace data and needs only the
// right to read Templates.
func (h *Handlers) EmailSchemaGet(_ context.Context) (externalapi.EmailSchemaGetRes, error) {
	return new(mapper.EmailSchemaToResource(emaildoc.AgentSchema())), nil
}
