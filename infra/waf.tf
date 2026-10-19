# A web ACL on the load balancer (ADR 0028): AWS managed rule groups, plus one rate-based rule over
# the two hosts that are public and unauthenticated at the edge, the tracker's /collect and the MCP
# host. It sits in front of the app-level limits of ADR 0025 and does not replace them: those are per
# Workspace, this one is per source address and only keeps a flood off the tasks.
#
# var.waf_mode "count" only counts what a rule would block (CloudWatch metrics and sampled requests
# of the web ACL, in the console), "block" enforces. Ship in count, read the sampled requests, then
# switch (docs/operations/runbook.md, "Web application firewall").

locals {
  waf_block = var.waf_mode == "block"
}

resource "aws_wafv2_web_acl" "this" {
  name  = var.name
  scope = "REGIONAL"

  default_action {
    allow {}
  }

  dynamic "rule" {
    for_each = { for i, name in var.waf_managed_rule_groups : name => i }

    content {
      name     = "aws-${rule.key}"
      priority = rule.value + 10 # after the rate rule (priority 1)

      # A managed group keeps its own actions in block mode and counts them otherwise.
      override_action {
        dynamic "none" {
          for_each = local.waf_block ? [1] : []
          content {}
        }
        dynamic "count" {
          for_each = local.waf_block ? [] : [1]
          content {}
        }
      }

      statement {
        managed_rule_group_statement {
          vendor_name = "AWS"
          name        = rule.key

          # Rules that match legitimate traffic of this app stay in count mode whatever var.waf_mode.
          dynamic "rule_action_override" {
            for_each = rule.key == "AWSManagedRulesCommonRuleSet" ? var.waf_common_count_rules : []
            content {
              name = rule_action_override.value
              action_to_use {
                count {}
              }
            }
          }
        }
      }

      visibility_config {
        cloudwatch_metrics_enabled = true
        metric_name                = "${var.name}-aws-${rule.key}"
        sampled_requests_enabled   = true
      }
    }
  }

  # Requests per source address over the window, for the MCP host and the collect endpoint only.
  rule {
    name     = "rate-mcp-collect"
    priority = 1

    action {
      dynamic "block" {
        for_each = local.waf_block ? [1] : []
        content {}
      }
      dynamic "count" {
        for_each = local.waf_block ? [] : [1]
        content {}
      }
    }

    statement {
      rate_based_statement {
        limit                 = var.waf_rate_limit
        evaluation_window_sec = var.waf_rate_window_seconds
        aggregate_key_type    = "IP"

        scope_down_statement {
          or_statement {
            statement {
              byte_match_statement {
                search_string         = local.mcp_host
                positional_constraint = "EXACTLY"

                field_to_match {
                  single_header {
                    name = "host"
                  }
                }

                text_transformation {
                  priority = 0
                  type     = "LOWERCASE"
                }
              }
            }

            statement {
              byte_match_statement {
                search_string         = "/collect"
                positional_constraint = "STARTS_WITH"

                field_to_match {
                  uri_path {}
                }

                text_transformation {
                  priority = 0
                  type     = "NONE"
                }
              }
            }
          }
        }
      }
    }

    visibility_config {
      cloudwatch_metrics_enabled = true
      metric_name                = "${var.name}-rate-mcp-collect"
      sampled_requests_enabled   = true
    }
  }

  visibility_config {
    cloudwatch_metrics_enabled = true
    metric_name                = var.name
    sampled_requests_enabled   = true
  }
}

# The console keeps sampled requests for about three hours, too short to review a week of count mode,
# so every counted or blocked request is logged. Allowed requests are dropped (volume and cost). The
# name must start with aws-waf-logs-.
resource "aws_cloudwatch_log_group" "waf" {
  name              = "aws-waf-logs-${var.name}"
  retention_in_days = var.waf_log_retention_days
}

resource "aws_wafv2_web_acl_logging_configuration" "this" {
  resource_arn            = aws_wafv2_web_acl.this.arn
  log_destination_configs = [aws_cloudwatch_log_group.waf.arn]

  redacted_fields {
    single_header {
      name = "authorization"
    }
  }

  redacted_fields {
    single_header {
      name = "cookie"
    }
  }

  logging_filter {
    default_behavior = "DROP"

    filter {
      behavior    = "KEEP"
      requirement = "MEETS_ANY"

      condition {
        action_condition {
          action = "COUNT"
        }
      }

      condition {
        action_condition {
          action = "BLOCK"
        }
      }
    }
  }
}

resource "aws_wafv2_web_acl_association" "alb" {
  resource_arn = aws_lb.this.arn
  web_acl_arn  = aws_wafv2_web_acl.this.arn
}
