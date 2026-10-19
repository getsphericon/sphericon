# CloudWatch alarms and EventBridge rules for the hosted deployment, all notifying one SNS topic that
# emails var.alarm_email. Slack: subscribe an AWS Chatbot channel configuration to the topic
# (infra/README.md); Chatbot is configured once in the console, so it is not Terraform here.
# Every alarm also notifies on recovery (ok_actions), so the channel shows when it ends.

data "aws_caller_identity" "current" {}

resource "aws_sns_topic" "alarms" {
  name = "${var.name}-alarms"
}

resource "aws_sns_topic_subscription" "alarm_email" {
  topic_arn = aws_sns_topic.alarms.arn
  protocol  = "email"
  endpoint  = var.alarm_email
}

# CloudWatch alarms of this account and EventBridge rules of this stack may publish, over TLS only.
# The topic is not encrypted with the AWS managed key: CloudWatch alarms cannot publish to it.
data "aws_iam_policy_document" "alarms_topic" {
  statement {
    sid       = "CloudWatchAlarms"
    actions   = ["sns:Publish"]
    resources = [aws_sns_topic.alarms.arn]

    principals {
      type        = "Service"
      identifiers = ["cloudwatch.amazonaws.com"]
    }

    condition {
      test     = "StringEquals"
      variable = "aws:SourceAccount"
      values   = [data.aws_caller_identity.current.account_id]
    }
  }

  statement {
    sid       = "EventBridgeRules"
    actions   = ["sns:Publish"]
    resources = [aws_sns_topic.alarms.arn]

    principals {
      type        = "Service"
      identifiers = ["events.amazonaws.com"]
    }

    condition {
      test     = "ArnLike"
      variable = "aws:SourceArn"
      values = [
        aws_cloudwatch_event_rule.deployment_failed.arn,
        aws_cloudwatch_event_rule.task_crashed.arn,
      ]
    }
  }

  statement {
    sid       = "TLSOnly"
    effect    = "Deny"
    actions   = ["sns:Publish"]
    resources = [aws_sns_topic.alarms.arn]

    principals {
      type        = "*"
      identifiers = ["*"]
    }

    condition {
      test     = "Bool"
      variable = "aws:SecureTransport"
      values   = ["false"]
    }
  }
}

resource "aws_sns_topic_policy" "alarms" {
  arn    = aws_sns_topic.alarms.arn
  policy = data.aws_iam_policy_document.alarms_topic.json
}

locals {
  alarm_actions = [aws_sns_topic.alarms.arn]
}

# --- Load balancer -----------------------------------------------------------------------------

# Share of requests answered with a 5xx (by the app or by the load balancer itself), judged only
# when there are at least 20 requests in the window: a single failure of a quiet period is not a rate.
resource "aws_cloudwatch_metric_alarm" "alb_5xx_rate" {
  alarm_name          = "${var.name}-alb-5xx-rate"
  alarm_description   = "More than 5% of requests failed with a 5xx over 5 minutes (twice in a row). Runbook: Hosted deployment alarms."
  comparison_operator = "GreaterThanThreshold"
  threshold           = 5
  evaluation_periods  = 2
  treat_missing_data  = "notBreaching"
  alarm_actions       = local.alarm_actions
  ok_actions          = local.alarm_actions

  metric_query {
    id          = "rate"
    expression  = "IF(requests >= 20, 100 * (target_5xx + elb_5xx) / requests, 0)"
    label       = "5xx rate (%)"
    return_data = true
  }

  metric_query {
    id = "requests"
    metric {
      namespace   = "AWS/ApplicationELB"
      metric_name = "RequestCount"
      period      = 300
      stat        = "Sum"
      dimensions  = { LoadBalancer = aws_lb.this.arn_suffix }
    }
  }

  metric_query {
    id = "target_5xx"
    metric {
      namespace   = "AWS/ApplicationELB"
      metric_name = "HTTPCode_Target_5XX_Count"
      period      = 300
      stat        = "Sum"
      dimensions  = { LoadBalancer = aws_lb.this.arn_suffix }
    }
  }

  metric_query {
    id = "elb_5xx"
    metric {
      namespace   = "AWS/ApplicationELB"
      metric_name = "HTTPCode_ELB_5XX_Count"
      period      = 300
      stat        = "Sum"
      dimensions  = { LoadBalancer = aws_lb.this.arn_suffix }
    }
  }
}

# No healthy target means the load balancer answers 503 to everyone. Missing data counts as
# breaching: a task that has vanished stops reporting the metric.
resource "aws_cloudwatch_metric_alarm" "alb_no_healthy_targets" {
  alarm_name          = "${var.name}-alb-no-healthy-targets"
  alarm_description   = "The target group has no healthy task (/readyz failing or no task running). Runbook: Hosted deployment alarms."
  namespace           = "AWS/ApplicationELB"
  metric_name         = "HealthyHostCount"
  statistic           = "Minimum"
  period              = 60
  evaluation_periods  = 3
  comparison_operator = "LessThanThreshold"
  threshold           = 1
  treat_missing_data  = "breaching"
  alarm_actions       = local.alarm_actions
  ok_actions          = local.alarm_actions

  dimensions = {
    LoadBalancer = aws_lb.this.arn_suffix
    TargetGroup  = aws_lb_target_group.app.arn_suffix
  }
}

resource "aws_cloudwatch_metric_alarm" "alb_unhealthy_targets" {
  alarm_name          = "${var.name}-alb-unhealthy-targets"
  alarm_description   = "A task fails the /readyz health check for 5 minutes. Runbook: Hosted deployment alarms."
  namespace           = "AWS/ApplicationELB"
  metric_name         = "UnHealthyHostCount"
  statistic           = "Maximum"
  period              = 60
  evaluation_periods  = 5
  comparison_operator = "GreaterThanThreshold"
  threshold           = 0
  treat_missing_data  = "notBreaching"
  alarm_actions       = local.alarm_actions
  ok_actions          = local.alarm_actions

  dimensions = {
    LoadBalancer = aws_lb.this.arn_suffix
    TargetGroup  = aws_lb_target_group.app.arn_suffix
  }
}

# --- ECS ----------------------------------------------------------------------------------------

# RunningTaskCount comes from Container Insights (enabled on the cluster in ecs.tf).
resource "aws_cloudwatch_metric_alarm" "ecs_running_tasks" {
  alarm_name          = "${var.name}-ecs-no-running-task"
  alarm_description   = "The service has no running task. Runbook: Hosted deployment alarms."
  namespace           = "ECS/ContainerInsights"
  metric_name         = "RunningTaskCount"
  statistic           = "Minimum"
  period              = 60
  evaluation_periods  = 3
  comparison_operator = "LessThanThreshold"
  threshold           = 1
  treat_missing_data  = "breaching"
  alarm_actions       = local.alarm_actions
  ok_actions          = local.alarm_actions

  dimensions = {
    ClusterName = aws_ecs_cluster.this.name
    ServiceName = aws_ecs_service.web.name
  }
}

resource "aws_cloudwatch_metric_alarm" "ecs_memory" {
  alarm_name          = "${var.name}-ecs-memory-high"
  alarm_description   = "Service memory is above 85% for 15 minutes: the task is close to an out-of-memory kill. Runbook: Hosted deployment alarms."
  namespace           = "AWS/ECS"
  metric_name         = "MemoryUtilization"
  statistic           = "Average"
  period              = 300
  evaluation_periods  = 3
  comparison_operator = "GreaterThanThreshold"
  threshold           = 85
  treat_missing_data  = "notBreaching"
  alarm_actions       = local.alarm_actions
  ok_actions          = local.alarm_actions

  dimensions = {
    ClusterName = aws_ecs_cluster.this.name
    ServiceName = aws_ecs_service.web.name
  }
}

resource "aws_cloudwatch_metric_alarm" "ecs_cpu" {
  alarm_name          = "${var.name}-ecs-cpu-high"
  alarm_description   = "Service CPU is above 85% for 15 minutes. Runbook: Hosted deployment alarms."
  namespace           = "AWS/ECS"
  metric_name         = "CPUUtilization"
  statistic           = "Average"
  period              = 300
  evaluation_periods  = 3
  comparison_operator = "GreaterThanThreshold"
  threshold           = 85
  treat_missing_data  = "notBreaching"
  alarm_actions       = local.alarm_actions
  ok_actions          = local.alarm_actions

  dimensions = {
    ClusterName = aws_ecs_cluster.this.name
    ServiceName = aws_ecs_service.web.name
  }
}

# The deploy circuit breaker gave up on a new revision and rolled back to the previous one.
resource "aws_cloudwatch_event_rule" "deployment_failed" {
  name        = "${var.name}-ecs-deployment-failed"
  description = "ECS deployment of ${var.name} failed (circuit breaker rollback)."

  event_pattern = jsonencode({
    source        = ["aws.ecs"]
    "detail-type" = ["ECS Deployment State Change"]
    resources     = [aws_ecs_service.web.id]
    detail        = { eventName = ["SERVICE_DEPLOYMENT_FAILED"] }
  })
}

resource "aws_cloudwatch_event_target" "deployment_failed" {
  rule = aws_cloudwatch_event_rule.deployment_failed.name
  arn  = aws_sns_topic.alarms.arn
}

# A task of the service stopped with a container exit code other than 0 (a crash or an OOM kill):
# a crash loop sends one message per restart. A normal stop (deploy, scale in) exits 0 or has no code.
resource "aws_cloudwatch_event_rule" "task_crashed" {
  name        = "${var.name}-ecs-task-crashed"
  description = "A ${var.name} task stopped with a non-zero container exit code."

  event_pattern = jsonencode({
    source        = ["aws.ecs"]
    "detail-type" = ["ECS Task State Change"]
    detail = {
      clusterArn = [aws_ecs_cluster.this.arn]
      group      = ["service:${aws_ecs_service.web.name}"]
      lastStatus = ["STOPPED"]
      containers = { exitCode = [{ "anything-but" = 0 }] }
    }
  })
}

resource "aws_cloudwatch_event_target" "task_crashed" {
  rule = aws_cloudwatch_event_rule.task_crashed.name
  arn  = aws_sns_topic.alarms.arn
}

# --- RDS ----------------------------------------------------------------------------------------

resource "aws_cloudwatch_metric_alarm" "rds_cpu" {
  alarm_name          = "${var.name}-rds-cpu-high"
  alarm_description   = "Database CPU is above 80% for 15 minutes. Runbook: Hosted deployment alarms."
  namespace           = "AWS/RDS"
  metric_name         = "CPUUtilization"
  statistic           = "Average"
  period              = 300
  evaluation_periods  = 3
  comparison_operator = "GreaterThanThreshold"
  threshold           = 80
  treat_missing_data  = "missing"
  alarm_actions       = local.alarm_actions
  ok_actions          = local.alarm_actions

  dimensions = { DBInstanceIdentifier = aws_db_instance.pg.identifier }
}

# 20% of the allocated storage, in bytes. The volume does not autoscale (no max_allocated_storage).
resource "aws_cloudwatch_metric_alarm" "rds_free_storage" {
  alarm_name          = "${var.name}-rds-free-storage-low"
  alarm_description   = "Less than 20% of the database storage is free: grow db_allocated_storage. Runbook: Hosted deployment alarms."
  namespace           = "AWS/RDS"
  metric_name         = "FreeStorageSpace"
  statistic           = "Minimum"
  period              = 300
  evaluation_periods  = 1
  comparison_operator = "LessThanThreshold"
  threshold           = var.db_allocated_storage * 1024 * 1024 * 1024 * 0.2
  treat_missing_data  = "missing"
  alarm_actions       = local.alarm_actions
  ok_actions          = local.alarm_actions

  dimensions = { DBInstanceIdentifier = aws_db_instance.pg.identifier }
}

resource "aws_cloudwatch_metric_alarm" "rds_connections" {
  alarm_name          = "${var.name}-rds-connections-high"
  alarm_description   = "Database connections are above ${var.db_connections_alarm_threshold} for 10 minutes (the budget is in ecs.tf). Runbook: Hosted deployment alarms."
  namespace           = "AWS/RDS"
  metric_name         = "DatabaseConnections"
  statistic           = "Maximum"
  period              = 300
  evaluation_periods  = 2
  comparison_operator = "GreaterThanThreshold"
  threshold           = var.db_connections_alarm_threshold
  treat_missing_data  = "missing"
  alarm_actions       = local.alarm_actions
  ok_actions          = local.alarm_actions

  dimensions = { DBInstanceIdentifier = aws_db_instance.pg.identifier }
}

# --- SES ----------------------------------------------------------------------------------------

# Account-level reputation metrics of var.region: AWS puts the account under review at a 5% bounce
# or a 0.1% complaint rate, and pauses sending at 10% and 0.5%. The metrics are fractions (0.05 = 5%).
resource "aws_cloudwatch_metric_alarm" "ses_bounce_rate" {
  alarm_name          = "${var.name}-ses-bounce-rate"
  alarm_description   = "SES account bounce rate is at 5%, where AWS puts the account under review. Runbook: Hosted deployment alarms."
  namespace           = "AWS/SES"
  metric_name         = "Reputation.BounceRate"
  statistic           = "Maximum"
  period              = 900
  evaluation_periods  = 1
  comparison_operator = "GreaterThanOrEqualToThreshold"
  threshold           = 0.05
  treat_missing_data  = "notBreaching"
  alarm_actions       = local.alarm_actions
  ok_actions          = local.alarm_actions
}

resource "aws_cloudwatch_metric_alarm" "ses_complaint_rate" {
  alarm_name          = "${var.name}-ses-complaint-rate"
  alarm_description   = "SES account complaint rate is at 0.1%, where AWS puts the account under review. Runbook: Hosted deployment alarms."
  namespace           = "AWS/SES"
  metric_name         = "Reputation.ComplaintRate"
  statistic           = "Maximum"
  period              = 900
  evaluation_periods  = 1
  comparison_operator = "GreaterThanOrEqualToThreshold"
  threshold           = 0.001
  treat_missing_data  = "notBreaching"
  alarm_actions       = local.alarm_actions
  ok_actions          = local.alarm_actions
}
