package external

import (
	"context"
	"net/http"

	externalapi "github.com/getsphericon/sphericon/gen/external"
	"github.com/getsphericon/sphericon/internal/api/auth"
	"github.com/getsphericon/sphericon/internal/convert"
	"github.com/getsphericon/sphericon/internal/emaildoc"
	"github.com/go-faster/jx"
	"github.com/samber/lo"
)

// EmailSchemaGet describes the editor's document to an agent (ADR 0040): the structure the editor
// derives, with the attribute rules the server checks. It holds no Workspace data and needs only the
// right to read Templates.
func (h *Handlers) EmailSchemaGet(ctx context.Context) (externalapi.EmailSchemaGetRes, error) {
	if !auth.HasScope(auth.GetTokenAuth(ctx), "templates:read") {
		res := externalapi.ProblemDetails(problem(http.StatusUnauthorized, "insufficient scope"))
		return &res, nil
	}
	schema := emaildoc.AgentSchema()
	return &externalapi.EmailSchemaResult{
		EditorVersion: schema.EditorVersion,
		Nodes:         externalapi.EmailSchemaResultNodes(lo.MapValues(schema.Nodes, schemaType)),
		Marks:         externalapi.EmailSchemaResultMarks(lo.MapValues(schema.Marks, schemaType)),
		Rules:         schema.Rules,
		Example:       convert.DocObject[externalapi.EmailSchemaResultExample](schema.Example),
	}, nil
}

func schemaType(t emaildoc.AgentType, _ string) externalapi.EmailSchemaType {
	return externalapi.EmailSchemaType{
		Content: optString(t.Content),
		Group:   optString(t.Group),
		Attrs:   externalapi.EmailSchemaTypeAttrs(lo.MapValues(t.Attrs, schemaAttribute)),
	}
}

func schemaAttribute(a emaildoc.AgentAttr, _ string) externalapi.EmailSchemaAttribute {
	return externalapi.EmailSchemaAttribute{Default: jx.Raw(a.Default), Type: a.Type, Allowed: a.Allowed, Rule: a.Rule}
}
