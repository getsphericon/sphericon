# RDS PostgreSQL: one db.t4g.micro, single AZ, gp3, encrypted, not publicly accessible. Used
# directly, without a transaction-mode pooler, because river relies on LISTEN/NOTIFY.

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
  multi_az               = false

  backup_retention_period    = 7
  auto_minor_version_upgrade = true
  apply_immediately          = true

  # A test deployment destroyed and recreated freely (ADR 0028): no protection, no final snapshot.
  deletion_protection = false
  skip_final_snapshot = true
}
