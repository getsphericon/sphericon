# The Route 53 zone is the root of every hostname and mail record (ADR 0028). Until the registrar's
# nameservers point at it (a one-time manual step, see README.md) nothing here resolves and the
# certificate is not validated.
locals {
  dns_ttl = 3600

  app_host     = var.domain
  api_host     = "${var.api_host_label}.${var.domain}"
  tracker_host = "${var.tracker_host_label}.${var.domain}"
}

resource "aws_route53_zone" "this" {
  name = var.domain
}

# The app (the apex itself), API and tracker hosts all point at the load balancer, which routes by
# Host (alb.tf). An ALIAS at the apex is fine: nothing else lives there but the zone's NS and SOA.
# The load balancer is IPv4-only (no IPv6 in the VPC), so there are no AAAA records.
resource "aws_route53_record" "hosts" {
  for_each = toset([local.app_host, local.api_host, local.tracker_host])

  zone_id = aws_route53_zone.this.zone_id
  name    = each.key
  type    = "A"

  alias {
    name                   = aws_lb.this.dns_name
    zone_id                = aws_lb.this.zone_id
    evaluate_target_health = true
  }
}
