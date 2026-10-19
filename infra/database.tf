# RDS PostgreSQL: Multi-AZ, gp3, encrypted, not publicly accessible, with deletion protection, a
# final snapshot and a 14-day backup window (ADR 0028). Used directly, without a transaction-mode pooler, because river relies on LISTEN/NOTIFY.

resource "aws_db_subnet_group" "this" {
  name       = var.name
  subnet_ids = aws_subnet.db[*].id
}

# The master password is generated here and lives in the state (private, encrypted bucket) and in
# the runtime secret. It is never typed, never in tfvars and never printed.
resource "random_password" "db" {
  length  = 32
  special = false
}

resource "aws_db_instance" "pg" {
  identifier     = var.name
  engine         = "postgres"
  engine_version = "18" # the repo's tests and dev stack run Postgres 18
  instance_class = var.db_instance_class

  allocated_storage = var.db_allocated_storage
  storage_type      = "gp3"
  storage_encrypted = true

  db_name  = "sphericon"
  username = "sphericon"
  password = random_password.db.result

  db_subnet_group_name   = aws_db_subnet_group.this.name
  vpc_security_group_ids = [aws_security_group.db.id]
  publicly_accessible    = false
  multi_az               = var.db_multi_az

  # Automated backups give point-in-time recovery inside the window (restore drill:
  # docs/operations/runbook.md).
  backup_retention_period    = var.db_backup_retention_days
  copy_tags_to_snapshot      = true
  auto_minor_version_upgrade = true
  apply_immediately          = true

  # db_protected = false makes the environment disposable again (scratch): no protection, no final
  # snapshot. While protected, destroy fails until it is applied as false (infra/README.md section 7),
  # and the automated backups outlive the instance so it can still be restored to a point in time.
  deletion_protection       = var.db_protected
  skip_final_snapshot       = !var.db_protected
  final_snapshot_identifier = var.db_protected ? "${var.name}-final" : null
  delete_automated_backups  = !var.db_protected
}
