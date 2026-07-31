# TLS Certificate Recovery Plan

## Goal

Restore trusted HTTPS for `api.aandiclub.com` and prevent certificate renewal from depending on application releases.

The v2 request headers remain unchanged. Public endpoints do not require JWT authentication, but clients still send `deviceOS` and an ISO-8601 `timestamp`.

## Confirmed root cause

- The served Let's Encrypt certificate expired on `2026-07-30 11:25:33 UTC`.
- Certificate renewal ran only at the end of a tag or manual CD deployment.
- The CD workflow ignored renewal failures and reloaded Nginx before renewal.
- No deployment ran during the certificate renewal window.

## Recovery

1. Run the `TLS Certificate Renewal` workflow manually from the default branch.
2. The workflow renews only the `api.aandiclub.com` certificate on EC2.
3. It verifies that the renewed certificate is valid for at least 14 days.
4. It validates the Nginx configuration and performs a graceful reload.
5. A GitHub-hosted runner verifies the certificate served publicly without bypassing TLS validation.
6. Verify `GET /v2/blogs?page=0&size=20` with the required `deviceOS` and ISO-8601 `timestamp` headers.

## Prevention

- Run certificate renewal at `01:17` and `13:17` UTC every day.
- Serialize certificate maintenance and CD with the `production-ec2-maintenance` concurrency group.
- Keep certificate renewal independent from gateway and monitor-bot image deployment.
- Fail the workflow when renewal, certificate validity, Nginx validation, reload, or public TLS verification fails.
- Retain manual dispatch for recovery and operational verification.

## Rollback

The change does not replace or delete existing certificate files manually. Certbot retains the lineage under `/opt/aandi/gateway/certbot/conf`.

If Nginx validation fails, the script exits before reload and the running Nginx process remains unchanged. If a reload must be reversed, restore the previous Certbot lineage on EC2, run `nginx -t`, and reload Nginx again.
