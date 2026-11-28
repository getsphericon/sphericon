-- +goose NO TRANSACTION
-- +goose Up
-- Validates the check the previous migration added NOT VALID: a Contact's source is one of the
-- Contact sources, `import` included (ADR 0138).
ALTER TABLE "contacts" VALIDATE CONSTRAINT "contacts_source_check";

-- +goose Down
