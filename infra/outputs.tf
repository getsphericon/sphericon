output "app_url" {
  description = "Public URL of the web app (APP_URL)."
  value       = "https://${local.app_host}"
}

output "alb_dns_name" {
  description = "DNS name of the load balancer: smoke-test it with a Host header before DNS delegation."
  value       = aws_lb.this.dns_name
}

output "name_servers" {
  description = "Set these at the registrar of the domain (once per zone)."
  value       = aws_route53_zone.this.name_servers
}

output "database_endpoint" {
  description = "RDS endpoint (reachable only from the app tasks)."
  value       = aws_db_instance.pg.address
}
