variable "region" {
  description = "AWS region of everything, SES included (SPHERICON_SES_REGION follows it)."
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
  description = "Apex domain. The Route 53 zone is created for it; point the registrar's nameservers at it once (infra/README.md). The apex serves the web app and the site API; api.<domain>, t.<domain> and mcp.<domain> serve the external API, the tracker and the MCP endpoint."
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

variable "mcp_host_label" {
  description = "Label of the MCP host: <label>.<domain> serves /mcp and /.well-known/* only (the agent connector URL is https://<label>.<domain>/mcp)."
  type        = string
  default     = "mcp"
}

variable "app_port" {
  description = "PORT the server listens on; also the target group and container port."
  type        = number
  default     = 3000
}

variable "internal_listener_source_security_group_id" {
  description = "Security group (same VPC) allowed to reach the app's internal listener. Empty (the default) exposes nothing: no SPHERICON_INTERNAL_ADDR, no port, no rule."
  type        = string
  default     = ""

  validation {
    condition     = var.internal_listener_source_security_group_id == "" || can(regex("^sg-[0-9a-f]+$", var.internal_listener_source_security_group_id))
    error_message = "Must be empty or a security group id (sg-...)."
  }
}

variable "internal_port" {
  description = "Port of the internal listener (SPHERICON_INTERNAL_ADDR), used only when internal_listener_source_security_group_id is set."
  type        = number
  default     = 3400

  validation {
    condition     = var.internal_port >= 1 && var.internal_port <= 65535 && var.internal_port != var.app_port
    error_message = "Must be a valid port and differ from app_port."
  }
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
  description = "RDS instance class. The connection budget in ecs.tf assumes the 2 GiB db.t4g.small."
  type        = string
  default     = "db.t4g.small"
}

variable "db_multi_az" {
  description = "Run a synchronous standby in a second availability zone (automatic failover)."
  type        = bool
  default     = true
}

variable "db_protected" {
  description = "Production safety: deletion protection on, and a final snapshot (<name>-final) and the automated backups kept on destroy. Set false (and apply) before destroying a scratch environment."
  type        = bool
  default     = true
}

variable "db_backup_retention_days" {
  description = "Days of automated backups, which is also the point-in-time recovery window (the AWS free plan caps it at 1)."
  type        = number
  default     = 14

  validation {
    condition     = var.db_backup_retention_days >= 1 && var.db_backup_retention_days <= 35
    error_message = "Must be between 1 and 35 days."
  }
}

variable "db_allocated_storage" {
  description = "RDS storage in GB (gp3, encrypted)."
  type        = number
  default     = 20
}

variable "db_max_open_conns" {
  description = "SPHERICON_DB_MAX_OPEN_CONNS: the database/sql pool of one process. See the connection budget in ecs.tf."
  type        = number
  default     = 10
}

variable "pgx_max_conns" {
  description = "SPHERICON_PGX_MAX_CONNS: the river pool of one process. See the connection budget in ecs.tf."
  type        = number
  default     = 10
}

variable "app_secret_name" {
  description = "Name of the Secrets Manager secret that `mise run infra:bootstrap` creates once (SPHERICON_JWT_SECRET, SPHERICON_ENCRYPTION_KEY, SPHERICON_BOOTSTRAP_TOKEN, SPHERICON_LICENSE_KEY). Terraform reads only its ARN; the values never pass through Terraform."
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
  description = "SPHERICON_SYSTEM_EMAIL_FROM: sender address of platform mail (password reset, invitations). Must be on the apex domain, which is the verified SES identity. Null means noreply@<domain>."
  type        = string
  default     = null
}

variable "dmarc_rua" {
  description = "Mailbox that receives DMARC aggregate reports (rua). Empty omits the tag."
  type        = string
  default     = ""
}

variable "docs_host_label" {
  description = "Label of the documentation host: <label>.<domain> is a CNAME to GitHub Pages. Keep it in step with docs/public/CNAME."
  type        = string
  default     = "docs"
}

variable "docs_github_owner" {
  description = "GitHub organization that owns the repository publishing the documentation site; the docs host is a CNAME to <owner>.github.io."
  type        = string
  default     = "getsphericon"
}

variable "ses_test_recipients" {
  description = "Email addresses SES verifies as recipients while the account is in the SES sandbox (it delivers only to verified addresses). Each gets a verification email with a link to click. Empty once SES production access is granted."
  type        = set(string)
  default     = []
}

# Alarms (alarms.tf).

variable "alarm_email" {
  description = "Email address subscribed to the alarm topic. AWS sends it one confirmation link: until it is clicked, no alarm reaches it."
  type        = string

  validation {
    condition     = can(regex("^[^@\\s]+@[^@\\s]+\\.[^@\\s]+$", var.alarm_email))
    error_message = "Must be an email address."
  }
}

variable "db_connections_alarm_threshold" {
  description = "DatabaseConnections above which the database alarm fires. The default is about 80% of the 190 connections of the db.t4g.small (see the connection budget in ecs.tf); raise it with the instance class."
  type        = number
  default     = 150
}

# Web application firewall (waf.tf).

variable "waf_mode" {
  description = "\"count\" counts what the web ACL would block, \"block\" enforces it. Ship in count, read the sampled requests (runbook, \"Web application firewall\"), then switch."
  type        = string
  default     = "count"

  validation {
    condition     = contains(["count", "block"], var.waf_mode)
    error_message = "Must be \"count\" or \"block\"."
  }
}

variable "waf_managed_rule_groups" {
  description = "AWS managed rule groups on the web ACL, in priority order."
  type        = list(string)
  default = [
    "AWSManagedRulesCommonRuleSet",
    "AWSManagedRulesKnownBadInputsRuleSet",
    "AWSManagedRulesAmazonIpReputationList",
  ]
}

variable "waf_common_count_rules" {
  description = "Rules of AWSManagedRulesCommonRuleSet kept in count mode even when waf_mode is \"block\": the body size limit (8 KB) and the XSS body check match the email HTML and MJML the app posts and large /collect batches (ADR 0025 caps those at 500 KB)."
  type        = list(string)
  default     = ["SizeRestrictions_BODY", "CrossSiteScripting_BODY"]
}

variable "waf_log_retention_days" {
  description = "Retention of the log group that holds the requests the web ACL counted or blocked (waf.tf). Must cover the count-mode review."
  type        = number
  default     = 30
}

variable "waf_rate_limit" {
  description = "Requests per source address over waf_rate_window_seconds before the rate rule for the MCP host and /collect fires. Agent connectors share egress addresses, so keep it well above one client's rate; the per-Workspace limits of ADR 0025 stay the precise ones."
  type        = number
  default     = 2000

  validation {
    condition     = var.waf_rate_limit >= 10
    error_message = "A WAF rate-based rule needs a limit of at least 10."
  }
}

variable "waf_rate_window_seconds" {
  description = "Evaluation window of the rate rule: 60, 120, 300 or 600."
  type        = number
  default     = 300

  validation {
    condition     = contains([60, 120, 300, 600], var.waf_rate_window_seconds)
    error_message = "Must be 60, 120, 300 or 600."
  }
}
