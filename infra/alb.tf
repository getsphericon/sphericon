# One Application Load Balancer routes the three hosts to the one service by Host header (ECS
# Express Mode could not express this: no listener rules, no URL rewrite). One ACM certificate
# covers the three hosts (the apex and two subdomains), validated through DNS in the zone of dns.tf.

resource "aws_acm_certificate" "this" {
  domain_name               = local.app_host
  subject_alternative_names = [local.api_host, local.tracker_host]
  validation_method         = "DNS"

  lifecycle {
    create_before_destroy = true
  }
}

resource "aws_route53_record" "cert_validation" {
  for_each = toset([local.app_host, local.api_host, local.tracker_host])

  zone_id         = aws_route53_zone.this.zone_id
  allow_overwrite = true
  ttl             = 60
  name            = [for o in aws_acm_certificate.this.domain_validation_options : o.resource_record_name if o.domain_name == each.key][0]
  type            = [for o in aws_acm_certificate.this.domain_validation_options : o.resource_record_type if o.domain_name == each.key][0]
  records         = [[for o in aws_acm_certificate.this.domain_validation_options : o.resource_record_value if o.domain_name == each.key][0]]
}

# Waits until the registrar's nameservers point at the zone and the certificate is issued.
resource "aws_acm_certificate_validation" "this" {
  certificate_arn         = aws_acm_certificate.this.arn
  validation_record_fqdns = [for r in aws_route53_record.cert_validation : r.fqdn]
}

resource "aws_lb" "this" {
  name               = var.name
  load_balancer_type = "application"
  security_groups    = [aws_security_group.alb.id]
  subnets            = aws_subnet.public[*].id
}

resource "aws_lb_target_group" "app" {
  name                 = var.name
  port                 = var.app_port
  protocol             = "HTTP"
  target_type          = "ip"
  vpc_id               = aws_vpc.this.id
  deregistration_delay = 30

  # /readyz pings the database, so a task that cannot reach Postgres is not routed to.
  health_check {
    path                = "/readyz"
    matcher             = "200"
    interval            = 15
    timeout             = 5
    healthy_threshold   = 2
    unhealthy_threshold = 3
  }
}

resource "aws_lb_listener" "http" {
  load_balancer_arn = aws_lb.this.arn
  port              = 80
  protocol          = "HTTP"

  default_action {
    type = "redirect"
    redirect {
      port        = "443"
      protocol    = "HTTPS"
      status_code = "HTTP_301"
    }
  }
}

# Anything not matched below (an unknown host, other paths on the tracker host) is a 404.
resource "aws_lb_listener" "https" {
  load_balancer_arn = aws_lb.this.arn
  port              = 443
  protocol          = "HTTPS"
  ssl_policy        = "ELBSecurityPolicy-TLS13-1-2-2021-06"
  certificate_arn   = aws_acm_certificate_validation.this.certificate_arn

  default_action {
    type = "fixed-response"
    fixed_response {
      content_type = "text/plain"
      message_body = "not found"
      status_code  = "404"
    }
  }
}

# api.<domain>/x -> service /api/x (the binary stays path-based). The query string is kept.
resource "aws_lb_listener_rule" "api" {
  listener_arn = aws_lb_listener.https.arn
  priority     = 10

  action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.app.arn
  }

  condition {
    host_header {
      values = [local.api_host]
    }
  }

  transform {
    type = "url-rewrite"
    url_rewrite_config {
      rewrite {
        regex   = "^/(.*)$"
        replace = "/api/$1"
      }
    }
  }
}

# The tracker host exposes only the script and the collect endpoint.
resource "aws_lb_listener_rule" "tracker" {
  listener_arn = aws_lb_listener.https.arn
  priority     = 20

  action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.app.arn
  }

  condition {
    host_header {
      values = [local.tracker_host]
    }
  }

  condition {
    path_pattern {
      values = ["/t.js", "/collect", "/collect/*"]
    }
  }
}

# The app host serves the SPA and /site/* (no rewrite).
resource "aws_lb_listener_rule" "app" {
  listener_arn = aws_lb_listener.https.arn
  priority     = 30

  action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.app.arn
  }

  condition {
    host_header {
      values = [local.app_host]
    }
  }
}
