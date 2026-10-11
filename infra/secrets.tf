# Two secrets reach the tasks through the task definition's `secrets` (valueFrom), so the values
# never appear in a container definition, a plan or an environment listing:
#
#  - var.app_secret_name (sphericon/app): created once, outside Terraform, by `mise run
#    infra:bootstrap` (JWT_SECRET, ENCRYPTION_KEY, BOOTSTRAP_TOKEN, LICENSE_KEY). Only its ARN is
#    read here. ENCRYPTION_KEY is never rotated.
#  - <name>/runtime: composed here from resources Terraform creates (DATABASE_URL, the SES send
#    key). These values are in the state, which is why the state bucket is private and encrypted.

data "aws_secretsmanager_secret" "app" {
  name = var.app_secret_name
}

resource "aws_secretsmanager_secret" "runtime" {
  name = "${var.name}/runtime"

  # Destroy and recreate freely: no recovery window, so the name is free again at once.
  recovery_window_in_days = 0
}

resource "aws_secretsmanager_secret_version" "runtime" {
  secret_id = aws_secretsmanager_secret.runtime.id
  secret_string = jsonencode({
    DATABASE_URL          = "postgres://${aws_db_instance.pg.username}:${random_password.db.result}@${aws_db_instance.pg.address}:${aws_db_instance.pg.port}/${aws_db_instance.pg.db_name}?sslmode=require"
    SES_ACCESS_KEY_ID     = aws_iam_access_key.ses.id
    SES_SECRET_ACCESS_KEY = aws_iam_access_key.ses.secret
  })
}
