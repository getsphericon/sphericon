# System email through Amazon SES in var.region. The identity is the apex domain, so mail is sent
# from an address such as noreply@<domain>; its records live in the zone of dns.tf. The domain is
# verified by its DKIM records (no separate _amazonses TXT). A new SES account starts in the
# sandbox: production access is a manual request (README.md).

locals {
  mail_from_domain  = "${var.mail_from_label}.${var.domain}"
  system_email_from = coalesce(var.system_email_from, "noreply@${var.domain}")
}

resource "aws_sesv2_email_identity" "this" {
  email_identity = var.domain

  dkim_signing_attributes {
    next_signing_key_length = "RSA_2048_BIT"
  }
}

# Bounces and SPF alignment use the MAIL FROM subdomain (envelope sender), not the apex.
resource "aws_sesv2_email_identity_mail_from_attributes" "this" {
  email_identity         = aws_sesv2_email_identity.this.email_identity
  mail_from_domain       = local.mail_from_domain
  behavior_on_mx_failure = "USE_DEFAULT_VALUE"
}

# Easy DKIM: three CNAMEs to <token>.dkim.amazonses.com.
resource "aws_route53_record" "ses_dkim" {
  count = 3

  zone_id = aws_route53_zone.this.zone_id
  name    = "${aws_sesv2_email_identity.this.dkim_signing_attributes[0].tokens[count.index]}._domainkey.${var.domain}"
  type    = "CNAME"
  ttl     = local.dns_ttl
  records = ["${aws_sesv2_email_identity.this.dkim_signing_attributes[0].tokens[count.index]}.dkim.amazonses.com"]
}

resource "aws_route53_record" "ses_mail_from_mx" {
  zone_id = aws_route53_zone.this.zone_id
  name    = local.mail_from_domain
  type    = "MX"
  ttl     = local.dns_ttl
  records = ["10 feedback-smtp.${var.region}.amazonses.com"]
}

# SPF of the MAIL FROM subdomain only. SES needs no apex SPF: DMARC passes through the aligned DKIM
# of the apex identity, and SPF is checked on the envelope sender (the MAIL FROM subdomain).
resource "aws_route53_record" "ses_mail_from_spf" {
  zone_id = aws_route53_zone.this.zone_id
  name    = local.mail_from_domain
  type    = "TXT"
  ttl     = local.dns_ttl
  records = ["v=spf1 include:amazonses.com ~all"]
}

# DMARC passes through aligned DKIM (the apex identity signs the From domain) or aligned SPF
# (relaxed: the MAIL FROM subdomain shares the organizational domain).
resource "aws_route53_record" "dmarc" {
  zone_id = aws_route53_zone.this.zone_id
  name    = "_dmarc.${var.domain}"
  type    = "TXT"
  ttl     = local.dns_ttl
  records = [var.dmarc_rua != "" ? "v=DMARC1; p=none; rua=mailto:${var.dmarc_rua}" : "v=DMARC1; p=none"]
}
