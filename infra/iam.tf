data "aws_iam_policy_document" "ecs_tasks_assume" {
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["ecs-tasks.amazonaws.com"]
    }
  }
}

# Execution role: pulls nothing from ECR (the image is public on GHCR), writes logs and resolves
# the two secrets at task start. The tasks have no task role: the app calls only SES, with the
# static key below.
resource "aws_iam_role" "execution" {
  name               = "${var.name}-ecs-execution"
  assume_role_policy = data.aws_iam_policy_document.ecs_tasks_assume.json
}

resource "aws_iam_role_policy_attachment" "execution" {
  role       = aws_iam_role.execution.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

data "aws_iam_policy_document" "execution_secrets" {
  statement {
    actions = ["secretsmanager:GetSecretValue"]
    resources = [
      data.aws_secretsmanager_secret.app.arn,
      aws_secretsmanager_secret.runtime.arn,
    ]
  }
}

resource "aws_iam_role_policy" "execution_secrets" {
  name   = "secrets"
  role   = aws_iam_role.execution.id
  policy = data.aws_iam_policy_document.execution_secrets.json
}

# The app's SES provider signs with static keys (internal/messaging/ses): a send-only IAM user.
resource "aws_iam_user" "ses" {
  name = "${var.name}-ses-send"
}

data "aws_iam_policy_document" "ses_send" {
  statement {
    actions   = ["ses:SendRawEmail"]
    resources = [aws_sesv2_email_identity.this.arn]
  }
  # The provider reads the send quota (it has no resource-level permission).
  statement {
    actions   = ["ses:GetSendQuota"]
    resources = ["*"]
  }
}

resource "aws_iam_user_policy" "ses_send" {
  name   = "send"
  user   = aws_iam_user.ses.name
  policy = data.aws_iam_policy_document.ses_send.json
}

resource "aws_iam_access_key" "ses" {
  user = aws_iam_user.ses.name
}
