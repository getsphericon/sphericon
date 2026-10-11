variable "region" {
  description = "AWS region of everything, SES included (SES_REGION follows it)."
  type        = string
  default     = "us-east-2"
}

variable "name" {
  description = "Base name of the resources."
  type        = string
  default     = "sphericon"
}

# Image (pulled anonymously from GHCR: the package is public).

variable "image_repository" {
  description = "Image repository on GHCR, as published by the release workflow (.goreleaser.yaml)."
  type        = string
  default     = "ghcr.io/getsphericon/sphericon"
}

variable "image_tag" {
  description = "Image tag to deploy (a release version, without the v). Changing it runs the migration and rolls the service."
  type        = string
}

# Application settings.

variable "otel_service_name" {
  description = "OTEL_SERVICE_NAME reported to OpenTelemetry."
  type        = string
  default     = "sphericon"
}

variable "domain" {
  description = "Apex domain. The Route 53 zone is created for it; point the registrar's nameservers at it once (infra/README.md). The apex serves the web app and the site API; api.<domain> and t.<domain> serve the external API and the tracker."
  type        = string
  default     = "getsphericon.app"
}

variable "api_host_label" {
  description = "Label of the external API host: <label>.<domain>, rewritten to the /api prefix of the service."
  type        = string
  default     = "api"
}

variable "tracker_host_label" {
  description = "Label of the tracker host: <label>.<domain> serves /t.js and /collect/* only."
  type        = string
  default     = "t"
}

variable "app_port" {
  description = "PORT the server listens on; also the target group and container port."
  type        = number
  default     = 3000
}

variable "task_cpu" {
  description = "Fargate task CPU units (512 = 0.5 vCPU)."
  type        = number
  default     = 512
}

variable "task_memory" {
  description = "Fargate task memory in MiB."
  type        = number
  default     = 1024
}

variable "db_instance_class" {
  description = "RDS instance class. The connection budget in ecs.tf assumes the 1 GiB db.t4g.micro."
  type        = string
  default     = "db.t4g.micro"
}

variable "db_allocated_storage" {
  description = "RDS storage in GB (gp3, encrypted)."
  type        = number
  default     = 20
}

variable "db_max_open_conns" {
  description = "DB_MAX_OPEN_CONNS: the database/sql pool of one process. See the connection budget in ecs.tf."
  type        = number
  default     = 10
}

variable "pgx_max_conns" {
  description = "PGX_MAX_CONNS: the river pool of one process. See the connection budget in ecs.tf."
  type        = number
  default     = 10
}

variable "app_secret_name" {
  description = "Name of the Secrets Manager secret that `mise run infra:bootstrap` creates once (JWT_SECRET, ENCRYPTION_KEY, BOOTSTRAP_TOKEN, LICENSE_KEY). Terraform reads only its ARN; the values never pass through Terraform."
  type        = string
  default     = "sphericon/app"
}

# System email through SES.

variable "mail_from_label" {
  description = "Label of the SES MAIL FROM subdomain: <label>.<domain>. SPF for SES lives here."
  type        = string
  default     = "mail"
}

variable "system_email_from" {
  description = "SYSTEM_EMAIL_FROM: sender address of platform mail (password reset, invitations). Must be on the apex domain, which is the verified SES identity. Null means noreply@<domain>."
  type        = string
  default     = null
}

variable "dmarc_rua" {
  description = "Mailbox that receives DMARC aggregate reports (rua). Empty omits the tag."
  type        = string
  default     = ""
}
