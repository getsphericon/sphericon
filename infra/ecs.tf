locals {
  image = "${var.image_repository}:${var.image_tag}"

  # Connection budget. db.t4g.micro (1 GiB) allows LEAST(DBInstanceClassMemory/9531392, 5000)
  # connections, in practice about 85-110 (RDS reserves 3 for superusers; read the real value with
  # `SHOW max_connections`). Every process opens two pools: database/sql (DB_MAX_OPEN_CONNS: ent,
  # the event bus and the job workers' own queries) and river's pgx pool (PGX_MAX_CONNS: fetch,
  # completion, LISTEN and leader election). One process peaks at 10 + 10 = 20.
  #
  # river runs 20 workers, but they do their database work through the database/sql pool and hold
  # no pgx connection, so the pgx pool does not need one connection per worker.
  #
  #   steady state              1 task                          20
  #   migration (before deploy) old task + migrate task        20 + ~4 (goose: 1-2; river migrator: 1-2)
  #   rolling deploy            old + new task                  20 + 20 = 40
  #
  # 40 stays under the lowest plausible limit (about 85). A third process, or a second task kept
  # permanently (3 x 20 = 60), still fits; beyond that lower the pools or move to a larger class.
  # No transaction-mode pooler: river relies on LISTEN/NOTIFY.
  plain_env = {
    APP_URL           = "https://${local.app_host}"
    PORT              = tostring(var.app_port)
    OTEL_SERVICE_NAME = var.otel_service_name
    DB_MAX_OPEN_CONNS = tostring(var.db_max_open_conns)
    PGX_MAX_CONNS     = tostring(var.pgx_max_conns)

    SYSTEM_EMAIL_PROVIDER = "ses"
    SYSTEM_EMAIL_FROM     = local.system_email_from
    SES_REGION            = var.region
  }

  # ECS resolves "<secret arn>:<json key>::" to one key of a JSON secret at task start.
  secret_env = merge(
    { for k in ["JWT_SECRET", "ENCRYPTION_KEY", "BOOTSTRAP_TOKEN", "LICENSE_KEY"] :
    k => "${data.aws_secretsmanager_secret.app.arn}:${k}::" },
    { for k in ["DATABASE_URL", "SES_ACCESS_KEY_ID", "SES_SECRET_ACCESS_KEY"] :
    k => "${aws_secretsmanager_secret.runtime.arn}:${k}::" },
  )

  # The image has ENTRYPOINT ["sphericon"] and no CMD: the service runs the server, the migrate
  # task adds the CMD `migrate`. Both read the same full production config.
  container = {
    name      = "web"
    image     = local.image
    essential = true
    portMappings = [{
      containerPort = var.app_port
      protocol      = "tcp"
    }]
    environment = [for k, v in local.plain_env : { name = k, value = v }]
    secrets     = [for k, v in local.secret_env : { name = k, valueFrom = v }]
    logConfiguration = {
      logDriver = "awslogs"
      options = {
        awslogs-group         = aws_cloudwatch_log_group.app.name
        awslogs-region        = var.region
        awslogs-stream-prefix = "app"
      }
    }
  }
}

resource "aws_cloudwatch_log_group" "app" {
  name              = "/ecs/${var.name}"
  retention_in_days = 30
}

resource "aws_ecs_cluster" "this" {
  name = var.name
}

resource "aws_ecs_task_definition" "web" {
  family                   = var.name
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = var.task_cpu
  memory                   = var.task_memory
  execution_role_arn       = aws_iam_role.execution.arn

  runtime_platform {
    operating_system_family = "LINUX"
    cpu_architecture        = "ARM64"
  }

  container_definitions = jsonencode([local.container])
}

resource "aws_ecs_task_definition" "migrate" {
  family                   = "${var.name}-migrate"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = 256
  memory                   = 512
  execution_role_arn       = aws_iam_role.execution.arn

  runtime_platform {
    operating_system_family = "LINUX"
    cpu_architecture        = "ARM64"
  }

  container_definitions = jsonencode([merge(local.container, {
    name         = "migrate"
    command      = ["migrate"]
    portMappings = []
    logConfiguration = merge(local.container.logConfiguration, {
      options = merge(local.container.logConfiguration.options, { awslogs-stream-prefix = "migrate" })
    })
  })])
}

# Applies the goose and river migrations once, before the new service revision is created or
# changed: ECS has no pre-deploy hook, so the service depends on this step. It re-runs whenever the
# migrate task definition changes (a new image tag, new settings). The script (mise/scripts/
# ecs-migrate.sh) runs `aws ecs run-task`, waits for it and fails the apply when the task fails,
# so the service is left on the old revision. It needs the aws CLI and the AWS_* credentials in
# Terraform's environment, which `mise run infra` provides. It is idempotent: a re-run with
# nothing pending is a no-op.
resource "terraform_data" "migrate" {
  triggers_replace = [aws_ecs_task_definition.migrate.arn]

  provisioner "local-exec" {
    command = "${path.module}/../mise/scripts/ecs-migrate.sh"
    environment = {
      AWS_REGION      = var.region
      CLUSTER         = aws_ecs_cluster.this.name
      TASK_DEFINITION = aws_ecs_task_definition.migrate.arn
      SUBNETS         = join(",", aws_subnet.public[*].id)
      SECURITY_GROUP  = aws_security_group.app.id
      LOG_GROUP       = aws_cloudwatch_log_group.app.name
    }
  }

  depends_on = [
    aws_db_instance.pg,
    aws_secretsmanager_secret_version.runtime,
    aws_vpc_security_group_ingress_rule.db_from_app,
    aws_vpc_security_group_egress_rule.app_all,
    aws_route_table_association.public,
  ]
}

# One service: HTTP, river and watermill all run in this process, so there is no worker.
# Rolling deploy: the new task starts first (200%), the old one drains after it is healthy (100%).
# AUTO_MIGRATE stays unset: terraform_data.migrate migrates once, before this resource.
resource "aws_ecs_service" "web" {
  name            = var.name
  cluster         = aws_ecs_cluster.this.id
  task_definition = aws_ecs_task_definition.web.arn
  desired_count   = 1
  launch_type     = "FARGATE"

  deployment_minimum_healthy_percent = 100
  deployment_maximum_percent         = 200
  health_check_grace_period_seconds  = 60

  deployment_circuit_breaker {
    enable   = true
    rollback = true
  }

  network_configuration {
    subnets          = aws_subnet.public[*].id
    security_groups  = [aws_security_group.app.id]
    assign_public_ip = true
  }

  load_balancer {
    target_group_arn = aws_lb_target_group.app.arn
    container_name   = "web"
    container_port   = var.app_port
  }

  depends_on = [
    terraform_data.migrate,
    aws_lb_listener.https,
  ]
}
