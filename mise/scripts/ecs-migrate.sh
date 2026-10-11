#!/usr/bin/env bash
# Runs the migrate task definition once on Fargate and waits for it (called by the
# terraform_data.migrate provisioner in infra/ecs.tf, before the service is updated).
# Inputs, all non-secret, from the environment: CLUSTER, TASK_DEFINITION, SUBNETS (comma
# separated), SECURITY_GROUP, LOG_GROUP, AWS_REGION. AWS credentials come from Terraform's
# environment. The task reads its secrets itself from Secrets Manager.
set -euo pipefail
set +x
: "${CLUSTER:?}" "${TASK_DEFINITION:?}" "${SUBNETS:?}" "${SECURITY_GROUP:?}" "${LOG_GROUP:?}"
export AWS_PAGER=""

network="awsvpcConfiguration={subnets=[$SUBNETS],securityGroups=[$SECURITY_GROUP],assignPublicIp=ENABLED}"
task="$(aws ecs run-task --cluster "$CLUSTER" --task-definition "$TASK_DEFINITION" \
  --launch-type FARGATE --network-configuration "$network" \
  --query 'tasks[0].taskArn' --output text)"
if [[ -z "$task" || "$task" == None ]]; then
  echo "error: ECS did not start the migrate task" >&2
  exit 1
fi
echo "migrate task: ${task##*/}"

aws ecs wait tasks-stopped --cluster "$CLUSTER" --tasks "$task"

code="$(aws ecs describe-tasks --cluster "$CLUSTER" --tasks "$task" \
  --query 'tasks[0].containers[0].exitCode' --output text)"
if [[ "$code" != 0 ]]; then
  reason="$(aws ecs describe-tasks --cluster "$CLUSTER" --tasks "$task" \
    --query 'tasks[0].[stoppedReason,containers[0].reason]' --output text)"
  echo "error: migrate task failed (exit code $code): $reason" >&2
  stream="migrate/migrate/${task##*/}"
  aws logs get-log-events --log-group-name "$LOG_GROUP" --log-stream-name "$stream" \
    --limit 50 --query 'events[].message' --output text >&2 || true
  exit 1
fi
echo "migrations applied"
