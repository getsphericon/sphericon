terraform {
  required_version = ">= 1.16.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.0"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.7"
    }
  }

  # State lives in a private, versioned, encrypted S3 bucket with native locking (a lock file next
  # to the state object: no DynamoDB table, ADR 0028). The bucket is created once by
  # `mise run infra:bootstrap`; its name (sphericon-tfstate-<account id>) is passed by
  # `mise run infra` as a non-secret -backend-config, so nothing account specific is committed.
  # Credentials come from the environment (the wrapper exports them from the `aws login` session).
  backend "s3" {
    key          = "production/terraform.tfstate"
    region       = "us-east-2"
    encrypt      = true
    use_lockfile = true
  }
}
