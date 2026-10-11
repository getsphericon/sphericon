# Credentials: the AWS_* environment variables that `mise run infra` exports from the `aws login`
# session of the `sphericon` profile. Nothing is committed.
provider "aws" {
  region = var.region

  default_tags {
    tags = {
      Project   = var.name
      ManagedBy = "terraform"
    }
  }
}
