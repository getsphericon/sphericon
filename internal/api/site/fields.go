package site

import "github.com/getsphericon/sphericon/internal/validation"

// The json names of the fields a form sends and an error is reported against.
const (
	fieldName        = validation.FieldName
	fieldEmail       = validation.FieldEmail
	fieldNewEmail    = validation.FieldNewEmail
	fieldPassword    = validation.FieldPassword
	fieldNewPassword = validation.FieldNewPassword
)
